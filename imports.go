package lumen

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ResolveImports loads a .lm file and all its imports transitively,
// returning a merged ParseResult. Cycles are detected and returned as errors.
// basePath is the directory from which relative import paths are resolved.
func ResolveImports(src, basePath string, visited map[string]bool) (*ParseResult, error) {
	stack := make(map[string]bool, len(visited))
	for path, active := range visited {
		stack[path] = active
	}
	return resolveImports(src, basePath, stack, make(map[string]bool))
}

func resolveImports(src, basePath string, stack, loaded map[string]bool) (*ParseResult, error) {
	result, err := ParseFull(src)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	for _, imp := range result.Imports {
		absPath := imp.Path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(basePath, absPath)
		}
		absPath = filepath.Clean(absPath)
		if stack[absPath] {
			return nil, fmt.Errorf("import cycle detected: %s", absPath)
		}
		if loaded[absPath] {
			continue
		}

		impSrc, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", imp.Path, err)
		}
		stack[absPath] = true
		impResult, err := resolveImports(string(impSrc), filepath.Dir(absPath), stack, loaded)
		delete(stack, absPath)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", imp.Path, err)
		}
		loaded[absPath] = true

		// Imported declarations precede local declarations. Merge every
		// executable declaration type; dropping queries or retractions changes
		// the meaning of the imported file.
		result.Frames = append(impResult.Frames, result.Frames...)
		result.Records = append(impResult.Records, result.Records...)
		result.Beliefs = append(impResult.Beliefs, result.Beliefs...)
		result.Correlations = append(impResult.Correlations, result.Correlations...)
		result.Retracts = append(impResult.Retracts, result.Retracts...)
		result.Bridges = append(impResult.Bridges, result.Bridges...)
		result.Queries = append(impResult.Queries, result.Queries...)
	}
	return result, nil
}

// LoadFileWithImports loads a .lm file and resolves all imports, populating
// the store with declarations from the file and all transitively imported files.
func LoadFileWithImports(path string, store *Store, now time.Time) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	absPath := filepath.Clean(path)
	visited := map[string]bool{absPath: true}
	result, err := ResolveImports(string(src), filepath.Dir(absPath), visited)
	if err != nil {
		return err
	}
	return loadParsed(result, store, now)
}

// loadParsed populates a store from a fully resolved ParseResult.
// Extracted from LoadFile so both code paths share the same logic.

func loadParsed(result *ParseResult, store *Store, now time.Time) error {
	declaredFrames := make(map[string]Frame)
	for _, pf := range result.Frames {
		composition, err := ParseCompositionMode(pf.Composition)
		if err != nil {
			return fmt.Errorf("frame %s: %w", pf.Name, err)
		}
		staleAction, err := ParseStaleAction(pf.OnStaleDerivation)
		if err != nil {
			return fmt.Errorf("frame %s: %w", pf.Name, err)
		}
		frame := Frame{
			Name:                pf.Name,
			Composition:         composition,
			Decay:               pf.Decay,
			ProvenanceDepth:     pf.ProvenanceDepth,
			ImportedDecayPolicy: pf.ImportedDecayPolicy,
			Opaque:              pf.Opaque,
			OpaqueSource:        pf.OpaqueSource,
			OpaqueReason:        pf.OpaqueReason,
			Calibration:         pf.Calibration,
			OnStaleDerivation:   staleAction,
		}
		if existing, ok := declaredFrames[frame.Name]; ok {
			if existing != frame {
				return fmt.Errorf("frame %s declared with conflicting definitions", frame.Name)
			}
		} else {
			declaredFrames[frame.Name] = frame
		}
		store.RegisterFrame(frame)
	}
	for _, pb := range result.Bridges {
		bridge := &Bridge{
			Name: pb.Name, FromFrame: pb.FromFrame, ToFrame: pb.ToFrame,
			Loss: pb.Loss, Method: pb.Method, Verified: pb.Verified,
			Assumptions: pb.Assumptions,
		}
		if existing, ok := store.Bridges.Lookup(bridge.Name); ok {
			if *existing != *bridge {
				return fmt.Errorf("bridge %s declared with conflicting definitions", bridge.Name)
			}
		} else if err := store.Bridges.Register(bridge); err != nil {
			return err
		}
	}
	for _, pr := range result.Records {
		ts := now
		if pr.At != nil {
			ts = *pr.At
		}
		rec := &Record{
			ID: pr.ID, Content: pr.Content, Timestamp: ts, Frame: pr.FrameName,
			Foundational: pr.Foundational,
		}
		if err := store.Assert(rec); err != nil {
			return fmt.Errorf("assert %s: %w", pr.ID, err)
		}
	}
	for _, pb := range result.Beliefs {
		conf := pb.Confidence
		if pb.HasCredalPrior {
			conf = (pb.CredalPriorLo + pb.CredalPriorHi) / 2
		}
		assertedAt := now
		if pb.At != nil {
			assertedAt = *pb.At
		}
		b := &Belief{
			ID: pb.ID, Content: pb.Content, Confidence: conf,
			AssertedAt: assertedAt, Frame: pb.FrameName, Derivation: pb.From,
		}
		if pb.DecayOverride != nil {
			b.DecayOverride = pb.DecayOverride
		}
		if err := store.Believe(b); err != nil {
			return fmt.Errorf("believe %s: %w", pb.ID, err)
		}
		// If inline evidence blocks are declared, run CredalBayesUpdate and
		// update the belief's confidence with the posterior midpoint.
		// Opaque frames do not support evidence decomposition.
		frameIsOpaque := func() bool {
			store.mu.RLock()
			f := store.frames[pb.FrameName]
			store.mu.RUnlock()
			return f.IsOpaque()
		}()
		if len(pb.Evidence) > 0 && !frameIsOpaque {
			var prior CredalPrior
			if pb.HasCredalPrior {
				prior = CredalPrior{Lo: pb.CredalPriorLo, Hi: pb.CredalPriorHi}
			} else {
				prior = CredalPrior{Lo: conf, Hi: conf}
			}
			// Guard prior endpoints away from 0 and 1.
			eps := 1e-6
			if prior.Lo <= 0 {
				prior.Lo = eps
			}
			if prior.Hi >= 1 {
				prior.Hi = 1 - eps
			}
			if prior.Lo > prior.Hi {
				prior.Lo = prior.Hi
			}

			var evidence []CredalEvidence
			for _, ev := range pb.Evidence {
				evConf := ev.Confidence
				if evConf <= 0 {
					evConf = 1.0
				}
				evidence = append(evidence, CredalEvidence{
					SourceID:   ev.ID,
					LRLo:       ev.LRLo,
					LRHi:       ev.LRHi,
					Confidence: evConf,
				})
			}

			posterior, err := CredalBayesUpdate(prior, evidence)
			if err != nil {
				// Do not silently drop declared evidence: a belief whose
				// evidence cannot be composed is a load error, not a shrug.
				return fmt.Errorf("belief %s: evidence composition failed: %w", pb.ID, err)
			}
			// Update the stored belief's confidence with the posterior midpoint,
			// and store the composition metadata so FragilityScan uses the exact
			// sensitivity path and ExportLM round-trips the evidence.
			store.mu.Lock()
			if stored, ok := store.beliefs[pb.ID]; ok {
				stored.Confidence = posterior.Midpoint()
				stored.CompositionPrior = (prior.Lo + prior.Hi) / 2
				for _, ev := range evidence {
					stored.CompositionEvidence = append(stored.CompositionEvidence, Evidence{
						SourceID:        ev.SourceID,
						Confidence:      ev.Confidence,
						LikelihoodRatio: (ev.LRLo + ev.LRHi) / 2,
					})
				}
			}
			store.mu.Unlock()
		}
	}
	for _, pr := range result.Retracts {
		if err := store.Retract(pr.ID, pr.Reason, now); err != nil {
			return fmt.Errorf("retract %s: %w", pr.ID, err)
		}
	}
	// Register named queries so they can be executed by ID.
	for _, q := range result.Queries {
		store.AddQuery(q)
	}
	return nil
}
