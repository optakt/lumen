package lumen

import (
	"fmt"
	"strings"
	"unicode"
)

type TokenKind int

const (
	TokIdent    TokenKind = iota
	TokString
	TokFloat
	TokInt
	TokDuration
	TokColon
	TokComma
	TokLBracket
	TokRBracket
	TokNewline
	TokIndent
	TokDedent
	TokEOF
)

var tokenNames = map[TokenKind]string{
	TokIdent: "IDENT", TokString: "STRING", TokFloat: "FLOAT",
	TokInt: "INT", TokDuration: "DURATION", TokColon: "COLON",
	TokComma: "COMMA", TokLBracket: "LBRACKET", TokRBracket: "RBRACKET", TokNewline: "NEWLINE", TokIndent: "INDENT",
	TokDedent: "DEDENT", TokEOF: "EOF",
}

func (k TokenKind) String() string {
	if s, ok := tokenNames[k]; ok {
		return s
	}
	return fmt.Sprintf("Token(%d)", int(k))
}

type Token struct {
	Kind  TokenKind
	Value string
	Line  int
	Col   int
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%q)@%d:%d", t.Kind, t.Value, t.Line, t.Col)
}

// Tokenize runs the full lexer in one pass, producing a flat token stream.
// Indentation is handled by a separate post-processing step so we avoid recursion.
func Tokenize(src string) ([]Token, error) {
	lines := strings.Split(src, "\n")
	var tokens []Token
	indentStack := []int{0}

	for lineNum, line := range lines {
		lineNo := lineNum + 1

		// Measure indentation
		indent := 0
		i := 0
		for i < len(line) {
			if line[i] == ' ' {
				indent++
				i++
			} else if line[i] == '\t' {
				indent += 4
				i++
			} else {
				break
			}
		}

		// Strip comments (// and # style).
		body := strings.TrimSpace(line[i:])
		for _, marker := range []string{"//", "#"} {
			if idx := strings.Index(body, marker); idx >= 0 {
				body = strings.TrimSpace(body[:idx])
				break
			}
		}

		// Skip blank lines
		if body == "" {
			continue
		}

		// Handle indent/dedent relative to stack
		current := indentStack[len(indentStack)-1]
		if indent > current {
			indentStack = append(indentStack, indent)
			tokens = append(tokens, Token{TokIndent, "", lineNo, 1})
		} else if indent < current {
			for len(indentStack) > 1 && indentStack[len(indentStack)-1] > indent {
				indentStack = indentStack[:len(indentStack)-1]
				tokens = append(tokens, Token{TokDedent, "", lineNo, 1})
			}
		}

		// Lex the body tokens
		lineTokens, err := lexLine(body, lineNo, indent+1)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, lineTokens...)
		tokens = append(tokens, Token{TokNewline, "", lineNo, len(line) + 1})
	}

	// Close any remaining indents
	for len(indentStack) > 1 {
		indentStack = indentStack[:len(indentStack)-1]
		tokens = append(tokens, Token{TokDedent, "", len(lines), 1})
	}
	tokens = append(tokens, Token{TokEOF, "", len(lines), 1})
	return tokens, nil
}

func lexLine(line string, lineNo, startCol int) ([]Token, error) {
	var tokens []Token
	runes := []rune(line)
	pos := 0
	col := startCol

	for pos < len(runes) {
		// Skip spaces
		for pos < len(runes) && (runes[pos] == ' ' || runes[pos] == '\t') {
			pos++
			col++
		}
		if pos >= len(runes) {
			break
		}

		ch := runes[pos]

		switch {
		case ch == ':':
			tokens = append(tokens, Token{TokColon, ":", lineNo, col})
			pos++
			col++

		case ch == ',':
			tokens = append(tokens, Token{TokComma, ",", lineNo, col})
			pos++
			col++

		case ch == '[':
			tokens = append(tokens, Token{TokLBracket, "[", lineNo, col})
			pos++
			col++

		case ch == ']':
			tokens = append(tokens, Token{TokRBracket, "]", lineNo, col})
			pos++
			col++

		case ch == '"':
			startC := col
			pos++
			col++
			var sb strings.Builder
			for pos < len(runes) && runes[pos] != '"' {
				if runes[pos] == '\\' && pos+1 < len(runes) {
					pos++
					col++
					switch runes[pos] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					default:
						sb.WriteRune(runes[pos])
					}
				} else {
					sb.WriteRune(runes[pos])
				}
				pos++
				col++
			}
			if pos >= len(runes) {
				return nil, fmt.Errorf("unterminated string at %d:%d", lineNo, startC)
			}
			pos++ // closing "
			col++
			tokens = append(tokens, Token{TokString, sb.String(), lineNo, startC})

		case unicode.IsDigit(ch) || ch == '-' || (ch == '+' && pos+1 < len(runes) && unicode.IsDigit(runes[pos+1])):
			startC := col
			var sb strings.Builder
			if ch == '-' || ch == '+' {
				sb.WriteRune(runes[pos])
				pos++
				col++
			}
			isFloat := false
			for pos < len(runes) && unicode.IsDigit(runes[pos]) {
				sb.WriteRune(runes[pos])
				pos++
				col++
			}
			if pos < len(runes) && runes[pos] == '.' {
				isFloat = true
				sb.WriteRune(runes[pos])
				pos++
				col++
				for pos < len(runes) && unicode.IsDigit(runes[pos]) {
					sb.WriteRune(runes[pos])
					pos++
					col++
				}
			}
			// Duration suffix
			if pos < len(runes) && (runes[pos] == 'd' || runes[pos] == 'h' || runes[pos] == 'm' || runes[pos] == 's') {
				sb.WriteRune(runes[pos])
				pos++
				col++
				tokens = append(tokens, Token{TokDuration, sb.String(), lineNo, startC})
			} else if isFloat {
				tokens = append(tokens, Token{TokFloat, sb.String(), lineNo, startC})
			} else {
				tokens = append(tokens, Token{TokInt, sb.String(), lineNo, startC})
			}

		case unicode.IsLetter(ch) || ch == '_' || ch == '\u2192':
			startC := col
			var sb strings.Builder
			for pos < len(runes) && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) || runes[pos] == '_' || runes[pos] == '-' || runes[pos] == '\u2192') {
				sb.WriteRune(runes[pos])
				pos++
				col++
			}
			tokens = append(tokens, Token{TokIdent, sb.String(), lineNo, startC})

		case ch == '>':
			// '>' is used in query where-clauses and bridge arrows.
			pos++; col++
			tokens = append(tokens, Token{TokIdent, ">", lineNo, col - 1})
		case ch == '<':
			pos++; col++
			tokens = append(tokens, Token{TokIdent, "<", lineNo, col - 1})
		default:
			return nil, fmt.Errorf("unexpected character %q at %d:%d", ch, lineNo, col)
		}
	}
	return tokens, nil
}

// NewLexer exists for backward compatibility; use Tokenize directly.
func NewLexer(src string) *struct{ src string } {
	return &struct{ src string }{src}
}

// Shim to let tests call l.Tokenize()
type LexerShim struct{ src string }

func (l *LexerShim) Tokenize() ([]Token, error) {
	return Tokenize(l.src)
}
