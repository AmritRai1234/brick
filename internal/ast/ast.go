// Package ast defines the abstract syntax tree for Brick.
package ast

import (
	"strings"

	"brick/internal/token"
)

// Node is any AST node.
type Node interface {
	Pos() (line, col int)
}

// Stmt is a statement.
type Stmt interface {
	Node
	stmtNode()
}

// Expr is an expression.
type Expr interface {
	Node
	exprNode()
}

// Type is a resolved Brick type.
type Type struct {
	Name string // "int", "float", "bool", "text", "List", "void", or a struct/enum name
	Ptr  bool   // true for a borrow/reference type
	Elem *Type  // element type for List
}

func (t Type) String() string {
	base := t.Name
	if t.Name == "List" && t.Elem != nil {
		base = "List(" + t.Elem.String() + ")"
	}
	if t.Ptr {
		return "&" + base
	}
	return base
}

// IsNumeric reports whether t is int or float.
func (t Type) IsNumeric() bool {
	return !t.Ptr && (t.Name == "int" || t.Name == "float")
}

// Equals reports whether two types are identical.
func (t Type) Equals(o Type) bool {
	if t.Name != o.Name || t.Ptr != o.Ptr {
		return false
	}
	if (t.Elem == nil) != (o.Elem == nil) {
		return false
	}
	if t.Elem != nil {
		return t.Elem.Equals(*o.Elem)
	}
	return true
}

type base struct{ Line, Col int }

func (b base) Pos() (int, int) { return b.Line, b.Col }

func (b *base) setBase(line, col int) { b.Line = line; b.Col = col }

// SetBase records the source position of a token on node n.
func SetBase(n Node, t token.Token) {
	if s, ok := n.(interface{ setBase(int, int) }); ok {
		s.setBase(t.Line, t.Col)
	}
}

// ---------- Expressions ----------

type Ident struct {
	base
	Name string
}

type IntLit struct {
	base
	Value int64
}

type FloatLit struct {
	base
	Value float64
	Text  string
}

type StringLit struct {
	base
	Value string
}

type BoolLit struct {
	base
	Value bool
}

type BinaryExpr struct {
	base
	Op    string
	Left  Expr
	Right Expr
}

type UnaryExpr struct {
	base
	Op      string // "-", "!", "&", "&mut"
	Operand Expr
}

type CallExpr struct {
	base
	Callee Expr
	Args   []Expr
}

type FieldExpr struct {
	base
	Base  Expr
	Field string
}

// IndexExpr is Base[Index].
type IndexExpr struct {
	base
	Base  Expr
	Index Expr
}

// StructLit is Name{ field: value, ... }.
type StructLit struct {
	base
	Name   string
	Fields []FieldInit
}

type FieldInit struct {
	Name  string
	Value Expr
}

func (*Ident) exprNode()      {}
func (*IntLit) exprNode()     {}
func (*FloatLit) exprNode()   {}
func (*StringLit) exprNode()  {}
func (*BoolLit) exprNode()    {}
func (*BinaryExpr) exprNode() {}
func (*UnaryExpr) exprNode()  {}
func (*CallExpr) exprNode()   {}
func (*FieldExpr) exprNode()  {}
func (*IndexExpr) exprNode()  {}
func (*StructLit) exprNode()  {}

// ---------- Statements ----------

type VarStmt struct {
	base
	Name  string
	Type  *Type // optional explicit type
	Value Expr
}

type AssignStmt struct {
	base
	Target Expr
	Value  Expr
}

type ReturnStmt struct {
	base
	Value Expr // may be nil
}

type ExprStmt struct {
	base
	X Expr
}

type BlockStmt struct {
	base
	Stmts []Stmt
}

type IfStmt struct {
	base
	Cond Expr
	Then *BlockStmt
	Else Stmt // *BlockStmt or *IfStmt, may be nil
}

type WhileStmt struct {
	base
	Cond Expr
	Body *BlockStmt
}

func (*VarStmt) stmtNode()    {}
func (*AssignStmt) stmtNode() {}
func (*ReturnStmt) stmtNode() {}
func (*ExprStmt) stmtNode()   {}
func (*BlockStmt) stmtNode()  {}
func (*IfStmt) stmtNode()     {}
func (*WhileStmt) stmtNode()  {}

// ---------- Declarations ----------

type Param struct {
	Name string
	Type Type
}

type FuncDecl struct {
	base
	Name       string
	Params     []Param
	ReturnType Type
	Body       *BlockStmt // nil for extern functions
	Extern     bool
}

func (*FuncDecl) stmtNode() {}

type Field struct {
	Name string
	Type Type
}

type StructDecl struct {
	base
	Name   string
	Fields []Field
}

func (*StructDecl) stmtNode() {}

type ImportStmt struct {
	base
	Path string
}

func (*ImportStmt) stmtNode() {}

// Variant is one case of an enum, optionally carrying payload fields.
type Variant struct {
	Name   string
	Fields []Type
}

// EnumDecl defines a tagged union.
type EnumDecl struct {
	base
	Name     string
	Variants []Variant
}

func (*EnumDecl) stmtNode() {}

// Pattern is one arm of a match: Enum.Variant, with optional bindings, or "_".
type Pattern struct {
	Line, Col int
	Enum      string
	Variant   string
	Binds     []string
	IsWild    bool
}

// Pos lets a Pattern satisfy Node for error reporting.
func (p Pattern) Pos() (int, int) { return p.Line, p.Col }

// MatchArm pairs a pattern with a body statement.
type MatchArm struct {
	Pattern Pattern
	Body    Stmt
}

// MatchStmt chooses between enum variants.
type MatchStmt struct {
	base
	Scrutinee Expr
	Arms      []MatchArm
}

func (*MatchStmt) stmtNode() {}

// Program is a parsed Brick file.
type Program struct {
	Stmts []Stmt
}

// ---------- Debug printing ----------

// Dump returns an indented, readable rendering of the tree.
func Dump(p *Program) string {
	var b strings.Builder
	for _, s := range p.Stmts {
		dumpStmt(&b, s, 0)
	}
	return b.String()
}

func indent(b *strings.Builder, n int) {
	for i := 0; i < n; i++ {
		b.WriteString("  ")
	}
}

func dumpStmt(b *strings.Builder, s Stmt, d int) {
	switch st := s.(type) {
	case *VarStmt:
		indent(b, d)
		b.WriteString("Var " + st.Name)
		if st.Type != nil {
			b.WriteString(": " + st.Type.String())
		}
		b.WriteString("\n")
		dumpExpr(b, st.Value, d+1)
	case *AssignStmt:
		indent(b, d)
		b.WriteString("Assign\n")
		indent(b, d+1)
		b.WriteString("target:\n")
		dumpExpr(b, st.Target, d+2)
		indent(b, d+1)
		b.WriteString("value:\n")
		dumpExpr(b, st.Value, d+2)
	case *ReturnStmt:
		indent(b, d)
		b.WriteString("Return\n")
		if st.Value != nil {
			dumpExpr(b, st.Value, d+1)
		}
	case *ExprStmt:
		indent(b, d)
		b.WriteString("ExprStmt\n")
		dumpExpr(b, st.X, d+1)
	case *BlockStmt:
		indent(b, d)
		b.WriteString("Block\n")
		for _, s := range st.Stmts {
			dumpStmt(b, s, d+1)
		}
	case *IfStmt:
		indent(b, d)
		b.WriteString("If\n")
		indent(b, d+1)
		b.WriteString("cond:\n")
		dumpExpr(b, st.Cond, d+2)
		indent(b, d+1)
		b.WriteString("then:\n")
		dumpStmt(b, st.Then, d+2)
		if st.Else != nil {
			indent(b, d+1)
			b.WriteString("else:\n")
			dumpStmt(b, st.Else, d+2)
		}
	case *WhileStmt:
		indent(b, d)
		b.WriteString("While\n")
		indent(b, d+1)
		b.WriteString("cond:\n")
		dumpExpr(b, st.Cond, d+2)
		dumpStmt(b, st.Body, d+1)
	case *FuncDecl:
		indent(b, d)
		b.WriteString("Function " + st.Name + "(")
		for i, p := range st.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p.Name + ": " + p.Type.String())
		}
		b.WriteString(") -> " + st.ReturnType.String())
		if st.Extern {
			b.WriteString("  [extern]\n")
			return
		}
		b.WriteString("\n")
		dumpStmt(b, st.Body, d+1)
	case *StructDecl:
		indent(b, d)
		b.WriteString("Struct " + st.Name + " { ")
		for i, f := range st.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(f.Name + ": " + f.Type.String())
		}
		b.WriteString(" }\n")
	case *ImportStmt:
		indent(b, d)
		b.WriteString("Import " + st.Path + "\n")
	case *EnumDecl:
		indent(b, d)
		b.WriteString("Enum " + st.Name + "\n")
		for _, v := range st.Variants {
			indent(b, d+1)
			b.WriteString(v.Name)
			if len(v.Fields) > 0 {
				b.WriteString("(")
				for i, f := range v.Fields {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(f.String())
				}
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
	case *MatchStmt:
		indent(b, d)
		b.WriteString("Match\n")
		indent(b, d+1)
		b.WriteString("scrutinee:\n")
		dumpExpr(b, st.Scrutinee, d+2)
		for _, arm := range st.Arms {
			indent(b, d+1)
			b.WriteString("arm ")
			if arm.Pattern.IsWild {
				b.WriteString("_")
			} else {
				b.WriteString(arm.Pattern.Enum + "." + arm.Pattern.Variant)
				if len(arm.Pattern.Binds) > 0 {
					b.WriteString("(")
					b.WriteString(strings.Join(arm.Pattern.Binds, ", "))
					b.WriteString(")")
				}
			}
			b.WriteString(":\n")
			dumpStmt(b, arm.Body, d+2)
		}
	}
}

func dumpExpr(b *strings.Builder, e Expr, d int) {
	indent(b, d)
	switch ex := e.(type) {
	case *Ident:
		b.WriteString("Ident " + ex.Name + "\n")
	case *IntLit:
		b.WriteString("Int " + itoa(ex.Value) + "\n")
	case *FloatLit:
		b.WriteString("Float " + ex.Text + "\n")
	case *StringLit:
		b.WriteString("String " + strconv(ex.Value) + "\n")
	case *BoolLit:
		if ex.Value {
			b.WriteString("Bool true\n")
		} else {
			b.WriteString("Bool false\n")
		}
	case *BinaryExpr:
		b.WriteString("Binary " + ex.Op + "\n")
		dumpExpr(b, ex.Left, d+1)
		dumpExpr(b, ex.Right, d+1)
	case *UnaryExpr:
		b.WriteString("Unary " + ex.Op + "\n")
		dumpExpr(b, ex.Operand, d+1)
	case *CallExpr:
		b.WriteString("Call\n")
		indent(b, d+1)
		b.WriteString("callee:\n")
		dumpExpr(b, ex.Callee, d+2)
		if len(ex.Args) > 0 {
			indent(b, d+1)
			b.WriteString("args:\n")
			for _, a := range ex.Args {
				dumpExpr(b, a, d+2)
			}
		}
	case *FieldExpr:
		b.WriteString("Field ." + ex.Field + "\n")
		dumpExpr(b, ex.Base, d+1)
	case *IndexExpr:
		b.WriteString("Index\n")
		indent(b, d+1)
		b.WriteString("base:\n")
		dumpExpr(b, ex.Base, d+2)
		indent(b, d+1)
		b.WriteString("index:\n")
		dumpExpr(b, ex.Index, d+2)
	case *StructLit:
		b.WriteString("StructLit " + ex.Name + "\n")
		for _, f := range ex.Fields {
			indent(b, d+1)
			b.WriteString(f.Name + ":\n")
			dumpExpr(b, f.Value, d+2)
		}
	}
}

func itoa(v int64) string {
	return strings.TrimSpace(strings.ReplaceAll(fmtInt(v), " ", ""))
}

func fmtInt(v int64) string {
	// local, dependency-free integer formatting
	if v == 0 {
		return "0"
	}
	neg := v < 0
	var buf [20]byte
	i := len(buf)
	for v != 0 {
		i--
		d := v % 10
		if d < 0 {
			d = -d
		}
		buf[i] = byte('0' + d)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func strconv(s string) string {
	return "\"" + s + "\""
}
