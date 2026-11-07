// Package token defines the lexical tokens of the Brick language.
package token

// Type is the kind of a lexical token.
type Type int

const (
	ILLEGAL Type = iota
	EOF
	NEWLINE

	// literals & identifiers
	INT
	FLOAT
	STRING
	CHAR
	IDENT

	// keywords
	VAR
	CONST
	FUNCTION
	IF
	ELSE
	WHILE
	FOR
	IN
	BREAK
	CONTINUE
	RETURN
	STRUCT
	ENUM
	MATCH
	ARENA
	IMPORT
	EXTERN
	TRUE
	FALSE

	// operators & punctuation
	ASSIGN    // =
	PLUS      // +
	MINUS     // -
	STAR      // *
	SLASH     // /
	PERCENT   // %
	EQ        // ==
	NEQ       // !=
	LT        // <
	LE        // <=
	GT        // >
	GE        // >=
	AND       // &&
	OR        // ||
	NOT       // !
	AMP       // &
	AMPMUT    // &mut
	ARROW     // ->
	FATARROW  // =>
	DOT       // .
	COMMA     // ,
	COLON     // :
	SEMICOLON // ;
	LPAREN    // (
	RPAREN    // )
	LBRACE    // {
	RBRACE    // }
	LBRACKET  // [
	RBRACKET  // ]
)

var names = map[Type]string{
	ILLEGAL: "ILLEGAL", EOF: "EOF", NEWLINE: "NEWLINE",
	INT: "INT", FLOAT: "FLOAT", STRING: "STRING", CHAR: "CHAR", IDENT: "IDENT",
	VAR: "var", CONST: "const", FUNCTION: "function", IF: "if", ELSE: "else",
	WHILE: "while", FOR: "for", IN: "in",
	BREAK: "break", CONTINUE: "continue",
	RETURN: "return", STRUCT: "struct", ENUM: "enum",
	MATCH: "match", ARENA: "arena", IMPORT: "import", EXTERN: "extern",
	TRUE: "true", FALSE: "false",
	ASSIGN: "=", PLUS: "+", MINUS: "-", STAR: "*", SLASH: "/", PERCENT: "%",
	EQ: "==", NEQ: "!=", LT: "<", LE: "<=", GT: ">", GE: ">=",
	AND: "&&", OR: "||", NOT: "!", AMP: "&", AMPMUT: "&mut",
	ARROW: "->", FATARROW: "=>", DOT: ".", COMMA: ",", COLON: ":",
	SEMICOLON: ";", LPAREN: "(", RPAREN: ")", LBRACE: "{", RBRACE: "}",
	LBRACKET: "[", RBRACKET: "]",
}

func (t Type) String() string {
	if s, ok := names[t]; ok {
		return s
	}
	return "UNKNOWN"
}

var keywords = map[string]Type{
	"var":      VAR,
	"const":    CONST,
	"function": FUNCTION,
	"if":       IF,
	"else":     ELSE,
	"while":    WHILE,
	"for":      FOR,
	"in":       IN,
	"break":    BREAK,
	"continue": CONTINUE,
	"return":   RETURN,
	"struct":   STRUCT,
	"enum":     ENUM,
	"match":    MATCH,
	"arena":    ARENA,
	"import":   IMPORT,
	"extern":   EXTERN,
	"true":     TRUE,
	"false":    FALSE,
}

// LookupIdent returns the keyword token type for ident, or IDENT.
func LookupIdent(ident string) Type {
	if t, ok := keywords[ident]; ok {
		return t
	}
	return IDENT
}

// Token is a single lexical token with source position.
type Token struct {
	Type    Type
	Literal string
	Line    int
	Col     int
}
