// Package lexer turns Brick source text into a stream of tokens.
package lexer

import (
	"fmt"

	"brick/internal/token"
)

// Lexer scans Brick source byte by byte (ASCII-focused).
type Lexer struct {
	input   string
	pos     int // index of ch
	readPos int // index of next char
	ch      byte
	line    int
	col     int
}

// New returns a lexer over input.
func New(input string) *Lexer {
	l := &Lexer{input: input, line: 1, col: 0}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
	if l.ch == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
}

func (l *Lexer) peek() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

// Tokens scans the whole input and returns all tokens (EOF-terminated).
func (l *Lexer) Tokens() []token.Token {
	var toks []token.Token
	for {
		t := l.NextToken()
		toks = append(toks, t)
		if t.Type == token.EOF {
			break
		}
	}
	return toks
}

// NextToken returns the next token.
func (l *Lexer) NextToken() token.Token {
	l.skipIgnored()

	line, col := l.line, l.col
	mk := func(t token.Type, lit string) token.Token {
		return token.Token{Type: t, Literal: lit, Line: line, Col: col}
	}

	switch l.ch {
	case 0:
		return mk(token.EOF, "")
	case '(':
		l.readChar()
		return mk(token.LPAREN, "(")
	case ')':
		l.readChar()
		return mk(token.RPAREN, ")")
	case '{':
		l.readChar()
		return mk(token.LBRACE, "{")
	case '}':
		l.readChar()
		return mk(token.RBRACE, "}")
	case '[':
		l.readChar()
		return mk(token.LBRACKET, "[")
	case ']':
		l.readChar()
		return mk(token.RBRACKET, "]")
	case ',':
		l.readChar()
		return mk(token.COMMA, ",")
	case ';':
		l.readChar()
		return mk(token.SEMICOLON, ";")
	case '.':
		l.readChar()
		return mk(token.DOT, ".")
	case ':':
		l.readChar()
		return mk(token.COLON, ":")
	case '+':
		l.readChar()
		return mk(token.PLUS, "+")
	case '*':
		l.readChar()
		return mk(token.STAR, "*")
	case '%':
		l.readChar()
		return mk(token.PERCENT, "%")
	case '/':
		l.readChar()
		return mk(token.SLASH, "/")
	case '-':
		l.readChar()
		if l.ch == '>' {
			l.readChar()
			return mk(token.ARROW, "->")
		}
		return mk(token.MINUS, "-")
	case '=':
		l.readChar()
		if l.ch == '=' {
			l.readChar()
			return mk(token.EQ, "==")
		}
		if l.ch == '>' {
			l.readChar()
			return mk(token.FATARROW, "=>")
		}
		return mk(token.ASSIGN, "=")
	case '!':
		l.readChar()
		if l.ch == '=' {
			l.readChar()
			return mk(token.NEQ, "!=")
		}
		return mk(token.NOT, "!")
	case '<':
		l.readChar()
		if l.ch == '=' {
			l.readChar()
			return mk(token.LE, "<=")
		}
		return mk(token.LT, "<")
	case '>':
		l.readChar()
		if l.ch == '=' {
			l.readChar()
			return mk(token.GE, ">=")
		}
		return mk(token.GT, ">")
	case '|':
		l.readChar()
		if l.ch == '|' {
			l.readChar()
			return mk(token.OR, "||")
		}
		return mk(token.ILLEGAL, "|")
	case '&':
		l.readChar()
		if l.ch == '&' {
			l.readChar()
			return mk(token.AND, "&&")
		}
		// &mut, written with no space between & and mut
		if l.ch == 'm' && l.peek() == 'u' && l.pos+3 < len(l.input) &&
			l.input[l.pos:l.pos+3] == "mut" {
			// ensure the char after mut is not part of a longer identifier
			after := byte(0)
			if l.pos+3 < len(l.input) {
				after = l.input[l.pos+3]
			}
			if !isIdentChar(after) {
				l.readChar() // m
				l.readChar() // u
				l.readChar() // t
				l.readChar() // past t
				return mk(token.AMPMUT, "&mut")
			}
		}
		return mk(token.AMP, "&")
	}

	if isLetter(l.ch) {
		lit := l.readIdentifier()
		return mk(token.LookupIdent(lit), lit)
	}
	if isDigit(l.ch) {
		return l.readNumber(line, col)
	}
	if l.ch == '"' {
		return l.readString(line, col)
	}
	if l.ch == '\'' {
		return l.readCharLiteral(line, col)
	}

	ch := l.ch
	l.readChar()
	return mk(token.ILLEGAL, string(ch))
}

func (l *Lexer) skipIgnored() {
	for {
		switch {
		case l.ch == ' ' || l.ch == '\t' || l.ch == '\r' || l.ch == '\n':
			l.readChar()
		case l.ch == '/' && l.peek() == '/':
			for l.ch != '\n' && l.ch != 0 {
				l.readChar()
			}
		default:
			return
		}
	}
}

func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readNumber(line, col int) token.Token {
	start := l.pos
	isFloat := false
	for isDigit(l.ch) {
		l.readChar()
	}
	if l.ch == '.' && isDigit(l.peek()) {
		isFloat = true
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
	}
	lit := l.input[start:l.pos]
	t := token.INT
	if isFloat {
		t = token.FLOAT
	}
	return token.Token{Type: t, Literal: lit, Line: line, Col: col}
}

func (l *Lexer) readString(line, col int) token.Token {
	l.readChar() // consume opening quote
	start := l.pos
	var buf []byte
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar()
			switch l.ch {
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case 'r':
				buf = append(buf, '\r')
			case '\\':
				buf = append(buf, '\\')
			case '"':
				buf = append(buf, '"')
			case '0':
				buf = append(buf, 0)
			default:
				buf = append(buf, '\\', l.ch)
			}
			l.readChar()
			continue
		}
		buf = append(buf, l.ch)
		l.readChar()
	}
	_ = start
	if l.ch != '"' {
		return token.Token{Type: token.ILLEGAL, Literal: "unterminated string", Line: line, Col: col}
	}
	l.readChar() // consume closing quote
	return token.Token{Type: token.STRING, Literal: string(buf), Line: line, Col: col}
}

// readCharLiteral scans a single-quoted byte literal such as 'a', '\n' or
// '\”. The Literal holds the decoded one-byte string; the parser turns it into
// an int, since a byte is an int in Brick (text indexing yields ints).
func (l *Lexer) readCharLiteral(line, col int) token.Token {
	bad := func(msg string) token.Token {
		return token.Token{Type: token.ILLEGAL, Literal: msg, Line: line, Col: col}
	}
	l.readChar() // consume opening quote
	if l.ch == 0 || l.ch == '\n' {
		return bad("unterminated character literal")
	}
	var b byte
	if l.ch == '\\' {
		l.readChar()
		switch l.ch {
		case 'n':
			b = '\n'
		case 't':
			b = '\t'
		case 'r':
			b = '\r'
		case '\\':
			b = '\\'
		case '\'':
			b = '\''
		case '"':
			b = '"'
		case '0':
			b = 0
		default:
			return bad(fmt.Sprintf("unknown escape \\%c in character literal", l.ch))
		}
		l.readChar()
	} else {
		b = l.ch
		l.readChar()
	}
	if l.ch != '\'' {
		return bad("character literal must hold exactly one byte")
	}
	l.readChar() // consume closing quote
	return token.Token{Type: token.CHAR, Literal: string([]byte{b}), Line: line, Col: col}
}

func isLetter(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }

func isIdentChar(ch byte) bool { return isLetter(ch) || isDigit(ch) }
