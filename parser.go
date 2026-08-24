package lumen

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ParsedFrame struct {
	Name                string
	Composition         string // "bayesian", "dempster-shafer", "opaque"
	Decay               DecayPolicy
	ProvenanceDepth     int
	ImportedDecayPolicy string
	// Opaque frame fields
	Opaque       bool
	OpaqueSource string // model identifier
	OpaqueReason string // why opacity is declared
	Calibration        string // calibration method ("isotonic", etc.)
	OnStaleDerivation  string // "mark_suspect", "fail", "retry"
}

type ParsedRecord struct {
	ID           string
	FrameName    string
	Content      string
	At           *time.Time
	Foundational bool // provenance: foundational — marks the chain terminus
}

// ParsedInlineEvidence is an inline credal evidence block within a believe declaration.
type ParsedInlineEvidence struct {
	ID           string    // evidence identifier (local to this belief)
	LRLo         float64   // lower bound on likelihood ratio
	LRHi         float64   // upper bound (= LRLo for point LR)
	Confidence   float64   // confidence that this evidence applies (0–1)
	Source       string    // ID of supporting record (optional)
	CorrelatesWith map[string]float64 // evidence-id → correlation coefficient
}

type ParsedBelief struct {
	ID            string
	FrameName     string
	Content       string
	Confidence    float64
	From          []string
	DecayOverride *DecayPolicy
	// CredalPrior: if set, Confidence is ignored and CredalBayesUpdate is used.
	HasCredalPrior bool
	CredalPriorLo  float64
	CredalPriorHi  float64
	// Evidence: inline credal evidence blocks.
	Evidence []ParsedInlineEvidence
}

// ParsedQuery is an epistemic archaeology query — first-class in .lm files.
type ParsedQuery struct {
	ID      string   // query identifier
	Target  string   // belief ID to query about
	Select  string   // "confidence-changes", "source-changes", "retraction-events"
	Where   string   // filter expression (raw, for future query engine)
	Since   string   // ISO timestamp lower bound
}

type ParsedFile struct {
	Frames  []ParsedFrame
	Records []ParsedRecord
	Beliefs []ParsedBelief
	Queries []ParsedQuery
}

type Parser struct {
	tokens []Token
	pos    int
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

func (p *Parser) cur() Token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return Token{Kind: TokEOF}
}

// advance past current token
func (p *Parser) advance() Token {
	t := p.cur()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

// skip newlines only
func (p *Parser) skipNewlines() {
	for p.cur().Kind == TokNewline {
		p.advance()
	}
}

// expect a token of the given kind (skipping newlines first)
func (p *Parser) expect(kind TokenKind) (Token, error) {
	p.skipNewlines()
	t := p.cur()
	if t.Kind != kind {
		return t, fmt.Errorf("expected %s, got %s %q at %d:%d", kind, t.Kind, t.Value, t.Line, t.Col)
	}
	p.advance()
	return t, nil
}

// expectIdent expects a specific identifier value
func (p *Parser) expectIdent(val string) error {
	p.skipNewlines()
	t := p.cur()
	if t.Kind != TokIdent || t.Value != val {
		return fmt.Errorf("expected %q, got %s %q at %d:%d", val, t.Kind, t.Value, t.Line, t.Col)
	}
	p.advance()
	return nil
}

func (p *Parser) Parse() (*ParsedFile, error) {
	result := &ParsedFile{}
	for {
		p.skipNewlines()
		t := p.cur()
		if t.Kind == TokEOF {
			break
		}
		if t.Kind != TokIdent {
			p.advance()
			continue
		}
		switch t.Value {
		case "frame":
			p.advance()
			f, err := p.parseFrame()
			if err != nil {
				return nil, err
			}
			result.Frames = append(result.Frames, f)
		case "record":
			p.advance()
			r, err := p.parseRecord()
			if err != nil {
				return nil, err
			}
			result.Records = append(result.Records, r)
		case "belief":
			p.advance()
			b, err := p.parseBelief()
			if err != nil {
				return nil, err
			}
			result.Beliefs = append(result.Beliefs, b)
		case "query":
			p.advance()
			q, err := p.parseQuery()
			if err != nil {
				return nil, err
			}
			result.Queries = append(result.Queries, q)
		default:
			p.advance() // skip unknown top-level token
		}
	}
	return result, nil
}

// parseQuery parses an epistemic archaeology query:
//
//	query <id>
//	    target: <belief-id>
//	    select: confidence-changes | source-changes | retraction-events
//	    where: <expression>
//	    since: "<timestamp>"
func (p *Parser) parseQuery() (ParsedQuery, error) {
	p.skipNewlines()
	idTok, err := p.expect(TokIdent)
	if err != nil {
		return ParsedQuery{}, fmt.Errorf("query id: %w", err)
	}
	q := ParsedQuery{ID: idTok.Value}

	p.skipNewlines()
	if p.cur().Kind != TokIndent {
		return q, nil
	}
	p.advance()

	for {
		p.skipNewlines()
		if p.cur().Kind == TokDedent || p.cur().Kind == TokEOF {
			p.advance()
			break
		}
		key := p.cur()
		if key.Kind != TokIdent {
			p.advance()
			continue
		}
		p.advance()
		if _, err := p.expect(TokColon); err != nil {
			return q, err
		}
		p.skipNewlines()
		switch key.Value {
		case "target":
			valTok := p.cur(); p.advance()
			q.Target = valTok.Value
		case "select":
			valTok := p.cur(); p.advance()
			q.Select = valTok.Value
		case "where":
			// Consume tokens until newline as raw filter expression
			var parts []string
			for p.cur().Kind != TokNewline && p.cur().Kind != TokEOF && p.cur().Kind != TokDedent {
				parts = append(parts, p.cur().Value)
				p.advance()
			}
			q.Where = strings.Join(parts, " ")
		case "since":
			valTok := p.cur(); p.advance()
			q.Since = valTok.Value
		}
	}
	return q, nil
}

func (p *Parser) parseFrame() (ParsedFrame, error) {
	p.skipNewlines()
	name, err := p.expect(TokIdent)
	if err != nil {
		return ParsedFrame{}, fmt.Errorf("frame name: %w", err)
	}
	f := ParsedFrame{
		Name: name.Value, Composition: "bayesian",
		ProvenanceDepth: 3, ImportedDecayPolicy: "most_conservative",
	}

	p.skipNewlines()
	if p.cur().Kind != TokIndent {
		return f, nil
	}
	p.advance() // consume INDENT

	for {
		p.skipNewlines()
		if p.cur().Kind == TokDedent || p.cur().Kind == TokEOF {
			p.advance()
			break
		}
		key := p.cur()
		if key.Kind != TokIdent {
			p.advance()
			continue
		}
		p.advance()
		if _, err := p.expect(TokColon); err != nil {
			return f, fmt.Errorf("after key %q: %w", key.Value, err)
		}
		p.skipNewlines()
		switch key.Value {
		case "composition":
			val, err := p.expect(TokIdent)
			if err != nil {
				return f, err
			}
			f.Composition = val.Value
		case "decay":
			kind, err := p.expect(TokIdent)
			if err != nil {
				return f, err
			}
			decay, err := p.parseDecayArgs(kind.Value)
			if err != nil {
				return f, fmt.Errorf("decay: %w", err)
			}
			f.Decay = decay
		case "provenance-depth":
			val, err := p.expect(TokInt)
			if err != nil {
				return f, err
			}
			n, _ := strconv.Atoi(val.Value)
			f.ProvenanceDepth = n
		case "imported-decay":
			val, err := p.expect(TokIdent)
			if err != nil {
				return f, err
			}
			f.ImportedDecayPolicy = val.Value
		case "source":
			// opaque frame: source: "model-identifier"
			valTok := p.cur(); p.advance()
			f.OpaqueSource = valTok.Value
			f.Opaque = true
		case "opacity-reason":
			valTok := p.cur(); p.advance()
			f.OpaqueReason = valTok.Value
			f.Opaque = true
		case "calibration":
			valTok := p.cur(); p.advance()
			f.Calibration = valTok.Value
		case "on_stale_derivation", "on-stale-derivation":
			valTok := p.cur(); p.advance()
			f.OnStaleDerivation = valTok.Value
		default:
			// Unknown frame attribute: consume the value token so the next
			// loop iteration doesn't treat it as a key name.
			p.advance()
		}
	}
	// If composition is "opaque", set the Opaque flag regardless of other fields.
	if f.Composition == "opaque" {
		f.Opaque = true
	}
	return f, nil
}

// parseDecayArgs parses the arguments for a decay kind.
// For "exponential": expects "halflife" ":" duration
// For "linear": expects "rate" ":" float
// For "none": no args
func (p *Parser) parseDecayArgs(kind string) (DecayPolicy, error) {
	switch kind {
	case "none":
		return DecayPolicy{Kind: "none"}, nil
	case "exponential":
		// "halflife" ":" duration
		if err := p.expectIdent("halflife"); err != nil {
			return DecayPolicy{}, err
		}
		if _, err := p.expect(TokColon); err != nil {
			return DecayPolicy{}, err
		}
		dur, err := p.parseDuration()
		if err != nil {
			return DecayPolicy{}, err
		}
		return DecayPolicy{Kind: "exponential", Halflife: dur}, nil
	case "linear":
		if err := p.expectIdent("rate"); err != nil {
			return DecayPolicy{}, err
		}
		if _, err := p.expect(TokColon); err != nil {
			return DecayPolicy{}, err
		}
		val, err := p.expect(TokFloat)
		if err != nil {
			return DecayPolicy{}, err
		}
		rate, _ := strconv.ParseFloat(val.Value, 64)
		return DecayPolicy{Kind: "linear", Rate: rate}, nil
	default:
		return DecayPolicy{}, fmt.Errorf("unknown decay kind %q", kind)
	}
}

func (p *Parser) parseDuration() (time.Duration, error) {
	p.skipNewlines()
	t := p.cur()
	if t.Kind != TokDuration && t.Kind != TokInt {
		return 0, fmt.Errorf("expected duration, got %s %q at %d:%d", t.Kind, t.Value, t.Line, t.Col)
	}
	p.advance()
	s := t.Value
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func (p *Parser) parseRecord() (ParsedRecord, error) {
	p.skipNewlines()
	name, err := p.expect(TokIdent)
	if err != nil {
		return ParsedRecord{}, fmt.Errorf("record name: %w", err)
	}
	if err := p.expectIdent("in"); err != nil {
		return ParsedRecord{}, fmt.Errorf("record %s: %w", name.Value, err)
	}
	frame, err := p.expect(TokIdent)
	if err != nil {
		return ParsedRecord{}, fmt.Errorf("record frame: %w", err)
	}
	r := ParsedRecord{ID: name.Value, FrameName: frame.Value}

	p.skipNewlines()
	if p.cur().Kind != TokIndent {
		return r, nil
	}
	p.advance()

	for {
		p.skipNewlines()
		if p.cur().Kind == TokDedent || p.cur().Kind == TokEOF {
			p.advance()
			break
		}
		t := p.cur()
		p.advance()
		switch t.Kind {
		case TokString:
			r.Content = t.Value
		case TokIdent:
			switch t.Value {
			case "at":
				if _, err := p.expect(TokColon); err != nil {
					return r, err
				}
				p.skipNewlines()
				ts := p.cur()
				if ts.Kind != TokString && ts.Kind != TokIdent {
					return r, fmt.Errorf("at timestamp: expected STRING, got %s %q at %d:%d", ts.Kind, ts.Value, ts.Line, ts.Col)
				}
				p.advance()
				// Accept both RFC3339 and date-only "2006-01-02" formats.
				var parsed time.Time
				var err error
				for _, layout := range []string{time.RFC3339, "2006-01-02"} {
					if parsed, err = time.Parse(layout, ts.Value); err == nil {
						break
					}
				}
				if err != nil {
					return r, fmt.Errorf("at timestamp %q: expected RFC3339 or YYYY-MM-DD: %w", ts.Value, err)
				}
				r.At = &parsed
			case "provenance":
				// provenance: foundational — marks the chain terminus (Gödelian limit)
				if _, err := p.expect(TokColon); err != nil { return r, err }
				p.skipNewlines()
				valTok := p.cur(); p.advance()
				if valTok.Value == "foundational" {
					r.Foundational = true
				}
			}
		}
	}
	return r, nil
}

func (p *Parser) parseBelief() (ParsedBelief, error) {
	p.skipNewlines()
	name, err := p.expect(TokIdent)
	if err != nil {
		return ParsedBelief{}, fmt.Errorf("belief name: %w", err)
	}
	if err := p.expectIdent("in"); err != nil {
		return ParsedBelief{}, fmt.Errorf("belief %s: %w", name.Value, err)
	}
	frame, err := p.expect(TokIdent)
	if err != nil {
		return ParsedBelief{}, fmt.Errorf("belief frame: %w", err)
	}
	b := ParsedBelief{ID: name.Value, FrameName: frame.Value}

	p.skipNewlines()
	if p.cur().Kind != TokIndent {
		return b, nil
	}
	p.advance()

	for {
		p.skipNewlines()
		if p.cur().Kind == TokDedent || p.cur().Kind == TokEOF {
			p.advance()
			break
		}
		t := p.cur()
		p.advance()
		switch t.Kind {
		case TokString:
			b.Content = t.Value
		case TokIdent:
			switch t.Value {
			case "confidence":
				if _, err := p.expect(TokColon); err != nil {
					return b, err
				}
				p.skipNewlines()
				val := p.cur()
				p.advance()
				conf, err := strconv.ParseFloat(val.Value, 64)
				if err != nil {
					return b, fmt.Errorf("confidence: %w", err)
				}
				b.Confidence = conf
			case "from", "sources":
				if _, err := p.expect(TokColon); err != nil {
					return b, err
				}
				for {
					p.skipNewlines()
					id := p.cur()
					if id.Kind != TokIdent {
						break
					}
					p.advance()
					b.From = append(b.From, id.Value)
					p.skipNewlines()
					if p.cur().Kind != TokComma {
						break
					}
					p.advance()
				}
			case "prior":
				// Credal prior: prior: [0.35, 0.65]
				if _, err := p.expect(TokColon); err != nil {
					return b, err
				}
				p.skipNewlines()
				// Expect '[' lo ',' hi ']'
				if _, err := p.expect(TokLBracket); err != nil {
					return b, fmt.Errorf("prior: expected '['")
				}
				loTok := p.cur()
				p.advance()
				lo, err := strconv.ParseFloat(loTok.Value, 64)
				if err != nil {
					return b, fmt.Errorf("prior lo: %w", err)
				}
				if _, err := p.expect(TokComma); err != nil {
					return b, fmt.Errorf("prior: expected ','")
				}
				hiTok := p.cur()
				p.advance()
				hi, err := strconv.ParseFloat(hiTok.Value, 64)
				if err != nil {
					return b, fmt.Errorf("prior hi: %w", err)
				}
				if _, err := p.expect(TokRBracket); err != nil {
					return b, fmt.Errorf("prior: expected ']'")
				}
				b.HasCredalPrior = true
				b.CredalPriorLo = lo
				b.CredalPriorHi = hi
			 case "decay":
				if _, err := p.expect(TokColon); err != nil {
					return b, err
				}
				kind, err := p.expect(TokIdent)
				if err != nil {
					return b, err
				}
				decay, err := p.parseDecayArgs(kind.Value)
				if err != nil {
					return b, fmt.Errorf("belief decay: %w", err)
				}
				b.DecayOverride = &decay
			case "evidence":
				// Inline credal evidence block: evidence <id> \n INDENT ... DEDENT
				ev, err := p.parseInlineEvidence()
				if err != nil {
					return b, fmt.Errorf("evidence block: %w", err)
				}
				b.Evidence = append(b.Evidence, ev)
			}
		}
	}
	return b, nil
}

// parseInlineEvidence parses an evidence block inside a believe declaration.
// Syntax:
//
//	evidence <id>
//	    lr: <float>             -- point LR
//	    lr: [<lo>, <hi>]        -- interval LR
//	    confidence: <float>
//	    source: <record-id>
//	    correlates-with: <evidence-id> <float>
func (p *Parser) parseInlineEvidence() (ParsedInlineEvidence, error) {
	ev := ParsedInlineEvidence{
		CorrelatesWith: make(map[string]float64),
	}
	// Parse evidence ID
	idTok, err := p.expect(TokIdent)
	if err != nil {
		return ev, fmt.Errorf("evidence id: %w", err)
	}
	ev.ID = idTok.Value
	ev.LRLo = 1.0
	ev.LRHi = 1.0
	ev.Confidence = 1.0

	p.skipNewlines()
	if p.cur().Kind != TokIndent {
		return ev, nil
	}
	p.advance()

	for {
		p.skipNewlines()
		if p.cur().Kind == TokDedent || p.cur().Kind == TokEOF {
			p.advance()
			break
		}
		t := p.cur()
		if t.Kind != TokIdent {
			p.advance()
			continue
		}
		p.advance()
		switch t.Value {
		case "lr":
			if _, err := p.expect(TokColon); err != nil {
				return ev, err
			}
			p.skipNewlines()
			if p.cur().Kind == TokLBracket {
				p.advance()
				loTok := p.cur(); p.advance()
				lo, err := strconv.ParseFloat(loTok.Value, 64)
				if err != nil { return ev, fmt.Errorf("lr lo: %w", err) }
				if _, err := p.expect(TokComma); err != nil { return ev, err }
				hiTok := p.cur(); p.advance()
				hi, err := strconv.ParseFloat(hiTok.Value, 64)
				if err != nil { return ev, fmt.Errorf("lr hi: %w", err) }
				if _, err := p.expect(TokRBracket); err != nil { return ev, err }
				ev.LRLo, ev.LRHi = lo, hi
			} else {
				valTok := p.cur(); p.advance()
				v, err := strconv.ParseFloat(valTok.Value, 64)
				if err != nil { return ev, fmt.Errorf("lr: %w", err) }
				ev.LRLo, ev.LRHi = v, v
			}
		case "confidence":
			if _, err := p.expect(TokColon); err != nil { return ev, err }
			p.skipNewlines()
			valTok := p.cur(); p.advance()
			v, err := strconv.ParseFloat(valTok.Value, 64)
			if err != nil { return ev, fmt.Errorf("evidence confidence: %w", err) }
			ev.Confidence = v
		case "source":
			if _, err := p.expect(TokColon); err != nil { return ev, err }
			p.skipNewlines()
			srcTok := p.cur(); p.advance()
			ev.Source = srcTok.Value
		case "correlates-with":
			// correlates-with: <evidence-id> <float>
			if _, err := p.expect(TokColon); err != nil { return ev, err }
			p.skipNewlines()
			targetTok := p.cur(); p.advance()
			p.skipNewlines()
			rTok := p.cur(); p.advance()
			r, err := strconv.ParseFloat(rTok.Value, 64)
			if err != nil { return ev, fmt.Errorf("correlates-with r: %w", err) }
			ev.CorrelatesWith[targetTok.Value] = r
		}
	}
	return ev, nil
}

func LoadFile(src string, store *Store, now time.Time) error {
	// Use ParseFull which handles all declaration types.
	result, err := ParseFull(src)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	// loadParsed is defined in imports.go and handles all declaration types
	// including bridges, imports, retracts, frames, records, and beliefs.
	return loadParsed(result, store, now)
}

// ParsedCompose holds a parsed compose declaration inside a believe block.
type ParsedCompose struct {
	Prior    float64
	Evidence []ParsedEvidence
}

// ParsedEvidence is one piece of evidence in a compose block.
type ParsedEvidence struct {
	SourceID        string
	LikelihoodRatio float64
	Confidence      float64
}

// ParsedCorrelation is a top-level correlation declaration.
type ParsedCorrelation struct {
	IDA, IDB    string
	Correlation float64
}

// ParsedRetract is a top-level retract statement.
type ParsedRetract struct {
	ID     string
	Reason string
}

// ParsedImport is a top-level import statement.
type ParsedImport struct {
	Path string
}

// ParseResult holds the full result of parsing a Lumen source file.
// Extends the existing Parse function result to include correlation,
// retract, and import statements.
type ParsedBridge struct {
	Name        string
	FromFrame   string
	ToFrame     string
	Loss        string
	Method      string
	Verified    bool
	Assumptions string
}

type ParseResult struct {
	Frames       []ParsedFrame
	Records      []ParsedRecord
	Beliefs      []ParsedBelief
	Correlations []ParsedCorrelation
	Retracts     []ParsedRetract
	Imports      []ParsedImport
	Bridges      []ParsedBridge
	Queries      []ParsedQuery
}

// ParseFull tokenizes and parses a complete Lumen source, returning
// all declaration types including correlations, retracts, and imports.
func ParseFull(src string) (*ParseResult, error) {
	tokens, err := Tokenize(src)
	if err != nil {
		return nil, err
	}
	p := &Parser{tokens: tokens}
	result := &ParseResult{}

	for {
		tok := p.cur()
		if tok.Kind == TokEOF {
			break
		}
		if tok.Kind == TokNewline {
			p.advance()
			continue
		}
		if tok.Kind != TokIdent {
			return nil, fmt.Errorf("unexpected token %v at %d:%d", tok, tok.Line, tok.Col)
		}
		p.advance() // consume the keyword token
		switch tok.Value {
		case "frame":
			pf, err := p.parseFrame()
			if err != nil {
				return nil, err
			}
			result.Frames = append(result.Frames, pf)
		case "record":
			pr, err := p.parseRecord()
			if err != nil {
				return nil, err
			}
			result.Records = append(result.Records, pr)
		case "believe", "belief": // both spellings accepted
			pb, err := p.parseBelief()
			if err != nil {
				return nil, err
			}
			result.Beliefs = append(result.Beliefs, pb)
		case "correlation":
			pc, err := p.parseCorrelation()
			if err != nil {
				return nil, err
			}
			result.Correlations = append(result.Correlations, pc)
		case "retract":
			pr, err := p.parseRetractStmt()
			if err != nil {
				return nil, err
			}
			result.Retracts = append(result.Retracts, pr)
		case "import":
			pi, err := p.parseImport()
			if err != nil {
				return nil, err
			}
			result.Imports = append(result.Imports, pi)
		case "bridge":
			pb, err := p.parseBridge()
			if err != nil {
				return nil, err
			}
			result.Bridges = append(result.Bridges, pb)
		case "query":
			q, err := p.parseQuery()
			if err != nil {
				return nil, err
			}
			result.Queries = append(result.Queries, q)
		default:
			return nil, fmt.Errorf("unexpected keyword %q at %d:%d", tok.Value, tok.Line, tok.Col)
		}
	}
	return result, nil
}

// parseCorrelation parses: correlation <idA> <idB>: <float>
func (p *Parser) parseCorrelation() (ParsedCorrelation, error) {
	var pc ParsedCorrelation
	// keyword "correlation" already consumed by ParseFull
	idA := p.advance() // source A ID
	if idA.Kind != TokIdent {
		return pc, fmt.Errorf("correlation idA: expected IDENT, got %v", idA)
	}
	idB := p.advance() // source B ID
	if idB.Kind != TokIdent {
		return pc, fmt.Errorf("correlation idB: expected IDENT, got %v", idB)
	}
	if _, err := p.expect(TokColon); err != nil {
		return pc, err
	}
	tok := p.advance()
	var r float64
	var err error
	switch tok.Kind {
	case TokFloat:
		r, err = strconv.ParseFloat(tok.Value, 64)
	case TokInt:
		var i int64
		i, err = strconv.ParseInt(tok.Value, 10, 64)
		r = float64(i)
	default:
		return pc, fmt.Errorf("correlation value: expected number, got %v", tok)
	}
	if err != nil {
		return pc, fmt.Errorf("correlation value: %w", err)
	}
	pc.IDA, pc.IDB, pc.Correlation = idA.Value, idB.Value, r
	p.skipNewlines()
	return pc, nil
}

// parseRetractStmt parses: retract <id> [reason: "<string>"]
func (p *Parser) parseRetractStmt() (ParsedRetract, error) {
	var pr ParsedRetract
	// keyword "retract" already consumed by ParseFull
	idTok := p.advance()
	if idTok.Kind != TokIdent {
		return pr, fmt.Errorf("retract id: expected IDENT, got %v", idTok)
	}
	pr.ID = idTok.Value
	// Optional: reason: "<string>"
	if p.cur().Kind == TokIdent && p.cur().Value == "reason" {
		p.advance() // consume "reason"
		if _, err := p.expect(TokColon); err != nil {
			return pr, err
		}
		reason, err := p.expect(TokString)
		if err != nil {
			return pr, fmt.Errorf("retract reason: %w", err)
		}
		pr.Reason = reason.Value
	}
	p.skipNewlines()
	return pr, nil
}

// parseImport parses: import "<path>"
func (p *Parser) parseImport() (ParsedImport, error) {
	var pi ParsedImport
	// keyword "import" already consumed by ParseFull
	path, err := p.expect(TokString)
	if err != nil {
		return pi, fmt.Errorf("import path: %w", err)
	}
	pi.Path = path.Value
	p.skipNewlines()
	return pi, nil
}

// parseBridge parses a bridge declaration:
//
//	bridge <name> : <fromFrame> → <toFrame>
//	  loss: <description>
//	  method: <name>
//	  verified: true|false
//	  assumes: "<text>"
func (p *Parser) parseBridge() (ParsedBridge, error) {
	p.skipNewlines()
	name, err := p.expect(TokIdent)
	if err != nil {
		return ParsedBridge{}, fmt.Errorf("bridge name: %w", err)
	}
	// Expect ":"
	if _, err := p.expect(TokColon); err != nil {
		return ParsedBridge{}, fmt.Errorf("bridge %s: expected ':'", name.Value)
	}
	// Expect fromFrame
	from, err := p.expect(TokIdent)
	if err != nil {
		return ParsedBridge{}, fmt.Errorf("bridge %s: from frame: %w", name.Value, err)
	}
	// Optional arrow "→" or "->" between frames; skip if present
	cur := p.cur()
	if cur.Kind == TokIdent && (cur.Value == "→" || cur.Value == "->") {
		p.advance()
	}
	// toFrame: next ident
	to, err := p.expect(TokIdent)
	if err != nil {
		return ParsedBridge{}, fmt.Errorf("bridge %s: to frame: %w", name.Value, err)
	}

	b := ParsedBridge{Name: name.Value, FromFrame: from.Value, ToFrame: to.Value}

	// Optional body block
	p.skipNewlines()
	if p.cur().Kind != TokIndent {
		return b, nil
	}
	p.advance()

	for {
		p.skipNewlines()
		if p.cur().Kind == TokDedent || p.cur().Kind == TokEOF {
			p.advance()
			break
		}
		t := p.cur()
		p.advance()
		if t.Kind != TokIdent {
			continue
		}
		switch t.Value {
		case "loss":
			if _, err := p.expect(TokColon); err != nil {
				return b, err
			}
			// Read rest of line as loss description (string or ident)
			val := p.cur()
			p.advance()
			if val.Kind == TokString {
				b.Loss = val.Value
			} else {
				b.Loss = val.Value
			}
		case "method":
			if _, err := p.expect(TokColon); err != nil {
				return b, err
			}
			val := p.cur()
			p.advance()
			b.Method = val.Value
		case "verified":
			if _, err := p.expect(TokColon); err != nil {
				return b, err
			}
			val := p.cur()
			p.advance()
			b.Verified = val.Value == "true"
		case "assumes":
			if _, err := p.expect(TokColon); err != nil {
				return b, err
			}
			val := p.cur()
			p.advance()
			if val.Kind == TokString {
				b.Assumptions = val.Value
			}
		}
	}
	return b, nil
}
