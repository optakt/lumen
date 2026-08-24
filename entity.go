package lumen

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Entity is a named thing that can be mentioned by beliefs or records.
// Entities are identified by normalized name; aliases collapse to the same entity.
type Entity struct {
	ID      string   // canonical normalized name
	Aliases []string // alternate spellings/references
	Kind    string   // "person", "theory", "event", "concept", "organization" — optional
}

// MentionEdge records that a content node (belief or record) mentions an entity.
type MentionEdge struct {
	NodeID   string // belief or record ID
	EntityID string // entity canonical ID
	// Span is the matched text that triggered this edge.
	Span string
}

// EntityGraph maintains the bipartite graph between content nodes and entities.
// It supports: registering entities with aliases, extracting mentions from text,
// querying which nodes mention a given entity, and finding nodes that co-mention entities.
type EntityGraph struct {
	mu       sync.RWMutex
	entities map[string]*Entity // canonical ID → entity
	// aliases maps normalized alias → canonical ID
	aliases map[string]string
	// nodeToEntities[nodeID] = set of entity IDs mentioned
	nodeToEntities map[string]map[string]bool
	// entityToNodes[entityID] = set of node IDs that mention it
	entityToNodes map[string]map[string]bool
	// all mention edges, in insertion order
	mentions []MentionEdge
}

func NewEntityGraph() *EntityGraph {
	return &EntityGraph{
		entities:       make(map[string]*Entity),
		aliases:        make(map[string]string),
		nodeToEntities: make(map[string]map[string]bool),
		entityToNodes:  make(map[string]map[string]bool),
	}
}

// normalize converts a name to a canonical lowercase key.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Remove possessives
	s = strings.TrimSuffix(s, "'s")
	// Collapse whitespace
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// RegisterEntity adds an entity to the graph. If the canonical ID already exists,
// new aliases are merged in. Returns the canonical ID.
func (g *EntityGraph) RegisterEntity(e *Entity) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	canon := normalize(e.ID)
	if existing, ok := g.entities[canon]; ok {
		// Merge aliases
		for _, a := range e.Aliases {
			na := normalize(a)
			g.aliases[na] = canon
			existing.Aliases = append(existing.Aliases, a)
		}
		return canon
	}
	e.ID = canon
	g.entities[canon] = e
	g.aliases[canon] = canon
	for _, a := range e.Aliases {
		g.aliases[normalize(a)] = canon
	}
	return canon
}

// Resolve returns the canonical entity ID for a name, or empty string if unknown.
func (g *EntityGraph) Resolve(name string) string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.aliases[normalize(name)]
}

// AddMention records that a content node mentions an entity (by canonical ID or alias).
// Returns the canonical entity ID, or error if entity is unknown.
func (g *EntityGraph) AddMention(nodeID, entityNameOrID, span string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	canon, ok := g.aliases[normalize(entityNameOrID)]
	if !ok {
		// Auto-register as an unknown entity
		canon = normalize(entityNameOrID)
		e := &Entity{ID: canon}
		g.entities[canon] = e
		g.aliases[canon] = canon
	}
	if g.nodeToEntities[nodeID] == nil {
		g.nodeToEntities[nodeID] = make(map[string]bool)
	}
	if g.entityToNodes[canon] == nil {
		g.entityToNodes[canon] = make(map[string]bool)
	}
	if !g.nodeToEntities[nodeID][canon] {
		g.nodeToEntities[nodeID][canon] = true
		g.entityToNodes[canon][nodeID] = true
		g.mentions = append(g.mentions, MentionEdge{
			NodeID: nodeID, EntityID: canon, Span: span,
		})
	}
	return canon, nil
}

// NodesForEntity returns all content node IDs that mention the given entity.
func (g *EntityGraph) NodesForEntity(entityID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	canon := g.aliases[normalize(entityID)]
	nodes := make([]string, 0, len(g.entityToNodes[canon]))
	for id := range g.entityToNodes[canon] {
		nodes = append(nodes, id)
	}
	sort.Strings(nodes)
	return nodes
}

// EntitiesForNode returns all entity IDs mentioned by the given content node.
func (g *EntityGraph) EntitiesForNode(nodeID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	result := make([]string, 0, len(g.nodeToEntities[nodeID]))
	for id := range g.nodeToEntities[nodeID] {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// EntitySnapshot returns a snapshot of the entity→nodes mapping for use in
// bulk co-mention analysis. The caller owns the returned map; the EntityGraph
// is not locked during the caller's analysis.
func (g *EntityGraph) EntitySnapshot() map[string][]string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make(map[string][]string, len(g.entityToNodes))
	for entityID, nodes := range g.entityToNodes {
		ids := make([]string, 0, len(nodes))
		for nodeID := range nodes {
			ids = append(ids, nodeID)
		}
		out[entityID] = ids
	}
	return out
}

// CoMentioned returns node IDs that share at least `minShared` entities with the given node.
// Excludes the node itself. Results are sorted by shared count (descending).
func (g *EntityGraph) CoMentioned(nodeID string, minShared int) []struct {
	NodeID      string
	SharedCount int
} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	myEntities := g.nodeToEntities[nodeID]
	counts := make(map[string]int)
	for entityID := range myEntities {
		for otherNode := range g.entityToNodes[entityID] {
			if otherNode != nodeID {
				counts[otherNode]++
			}
		}
	}
	type result struct {
		NodeID      string
		SharedCount int
	}
	var out []result
	for nid, cnt := range counts {
		if cnt >= minShared {
			out = append(out, result{nid, cnt})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SharedCount != out[j].SharedCount {
			return out[i].SharedCount > out[j].SharedCount
		}
		return out[i].NodeID < out[j].NodeID
	})
	// Convert to anonymous struct slice
	anon := make([]struct {
		NodeID      string
		SharedCount int
	}, len(out))
	for i, r := range out {
		anon[i].NodeID = r.NodeID
		anon[i].SharedCount = r.SharedCount
	}
	return anon
}

// ExtractAndIndex parses text for known entity names and auto-indexes mentions.
// It matches entity canonical IDs and aliases using word-boundary matching.
// Returns the list of entities found.
func (g *EntityGraph) ExtractAndIndex(nodeID, text string) []string {
	g.mu.Lock()
	// Build patterns for all known aliases (longest first to prefer specific matches)
	type aliasEntry struct {
		alias string
		canon string
		re    *regexp.Regexp
	}
	var entries []aliasEntry
	for alias, canon := range g.aliases {
		if len(alias) < 3 {
			continue // skip very short aliases to avoid noise
		}
		// Build case-insensitive word-boundary pattern
		pattern := `(?i)\b` + regexp.QuoteMeta(alias) + `\b`
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		entries = append(entries, aliasEntry{alias, canon, re})
	}
	// Sort by alias length descending (prefer longer, more specific matches)
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].alias) > len(entries[j].alias)
	})
	g.mu.Unlock()

	// Match against text
	matched := make(map[string]string) // canon → span
	for _, e := range entries {
		spans := e.re.FindAllString(text, -1)
		if len(spans) > 0 && matched[e.canon] == "" {
			matched[e.canon] = spans[0]
		}
	}

	var found []string
	for canon, span := range matched {
		g.AddMention(nodeID, canon, span) //nolint
		found = append(found, canon)
	}
	sort.Strings(found)
	return found
}

// SimpleNER extracts candidate named entities from text using heuristics:
// sequences of capitalized words (2-4 words), filtered against a stoplist.
// This is used to bootstrap entity registration without an external NER API.
func SimpleNER(text string) []string {
	stoplist := map[string]bool{
		"the": true, "a": true, "an": true, "in": true, "on": true,
		"at": true, "to": true, "of": true, "and": true, "or": true,
		"but": true, "for": true, "with": true, "this": true, "that": true,
		"it": true, "is": true, "are": true, "was": true, "were": true,
		"has": true, "have": true, "had": true, "be": true, "been": true,
		"as": true, "by": true, "from": true, "not": true, "no": true,
		"its": true, "their": true, "they": true, "we": true, "he": true,
		"she": true, "i": true, "if": true, "so": true, "than": true,
		"when": true, "which": true, "who": true, "what": true, "how": true,
	}

	// Tokenize into words
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '\''
	})

	var candidates []string
	seen := make(map[string]bool)
	i := 0
	for i < len(words) {
		w := words[i]
		if len(w) > 0 && unicode.IsUpper(rune(w[0])) && !stoplist[strings.ToLower(w)] {
			// Start of a candidate — extend up to 4 words
			end := i + 1
			for end < i+4 && end < len(words) {
				nw := words[end]
				if len(nw) > 0 && unicode.IsUpper(rune(nw[0])) && !stoplist[strings.ToLower(nw)] {
					end++
				} else {
					break
				}
			}
			phrase := strings.Join(words[i:end], " ")
			if !seen[strings.ToLower(phrase)] {
				seen[strings.ToLower(phrase)] = true
				candidates = append(candidates, phrase)
			}
			i = end
		} else {
			i++
		}
	}
	return candidates
}

// EntityStats returns counts of entities and mentions in the graph.
func (g *EntityGraph) EntityStats() (entities, mentions int) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.entities), len(g.mentions)
}


// Remove removes all entity mentions for a given content node ID.
// Used by ApplyContraction to prevent entity queries returning dead IDs.
func (g *EntityGraph) Remove(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Remove all mention edges from this node.
	newMentions := g.mentions[:0]
	for _, m := range g.mentions {
		if m.NodeID != nodeID {
			newMentions = append(newMentions, m)
		}
	}
	g.mentions = newMentions
	// Remove any entities that are now completely unreferenced.
	for entityID := range g.entities {
		referenced := false
		for _, m := range g.mentions {
			if m.EntityID == entityID {
				referenced = true
				break
			}
		}
		if !referenced {
			delete(g.entities, entityID)
		}
	}
}
