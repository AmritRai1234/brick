package checker

import (
	"fmt"
	"strings"

	"brick/internal/ast"
)

// TypeDump renders the inferred type of every expression in the program, in
// source order, one `line:col KIND type` line each. It is the canonical,
// order-stable fingerprint that the self-hosted checker (selfhost/checker.brick)
// is diffed against, the way `brick ast` is used for the parser.
func TypeDump(prog *ast.Program, ck *Checker) string {
	var b strings.Builder
	for _, s := range prog.Stmts {
		walkStmt(&b, ck, s)
	}
	return b.String()
}

func walkStmt(b *strings.Builder, ck *Checker, s ast.Stmt) {
	switch st := s.(type) {
	case *ast.VarStmt:
		walkExpr(b, ck, st.Value)
	case *ast.AssignStmt:
		walkExpr(b, ck, st.Target)
		walkExpr(b, ck, st.Value)
	case *ast.ReturnStmt:
		walkExpr(b, ck, st.Value)
	case *ast.ExprStmt:
		walkExpr(b, ck, st.X)
	case *ast.BlockStmt:
		for _, s := range st.Stmts {
			walkStmt(b, ck, s)
		}
	case *ast.IfStmt:
		walkExpr(b, ck, st.Cond)
		walkStmt(b, ck, st.Then)
		if st.Else != nil {
			walkStmt(b, ck, st.Else)
		}
	case *ast.WhileStmt:
		walkExpr(b, ck, st.Cond)
		walkStmt(b, ck, st.Body)
	case *ast.ArenaStmt:
		walkStmt(b, ck, st.Body)
	case *ast.ForStmt:
		walkExpr(b, ck, st.Iter)
		walkStmt(b, ck, st.Body)
	case *ast.FuncDecl:
		if st.Body != nil {
			walkStmt(b, ck, st.Body)
		}
	case *ast.ConstDecl:
		walkExpr(b, ck, st.Value)
	case *ast.MatchStmt:
		walkExpr(b, ck, st.Scrutinee)
		for _, arm := range st.Arms {
			walkStmt(b, ck, arm.Body)
		}
	}
}

func walkExpr(b *strings.Builder, ck *Checker, e ast.Expr) {
	if e == nil {
		return
	}
	if t, ok := ck.Types[e]; ok {
		line, col := e.Pos()
		fmt.Fprintf(b, "%d:%d %s %s\n", line, col, kindOf(e), t.String())
	}
	switch ex := e.(type) {
	case *ast.BinaryExpr:
		walkExpr(b, ck, ex.Left)
		walkExpr(b, ck, ex.Right)
	case *ast.UnaryExpr:
		walkExpr(b, ck, ex.Operand)
	case *ast.CallExpr:
		walkExpr(b, ck, ex.Callee)
		for _, a := range ex.Args {
			walkExpr(b, ck, a)
		}
	case *ast.FieldExpr:
		walkExpr(b, ck, ex.Base)
	case *ast.IndexExpr:
		walkExpr(b, ck, ex.Base)
		walkExpr(b, ck, ex.Index)
	case *ast.StructLit:
		for _, f := range ex.Fields {
			walkExpr(b, ck, f.Value)
		}
	}
}

func kindOf(e ast.Expr) string {
	switch e.(type) {
	case *ast.Ident:
		return "Ident"
	case *ast.IntLit:
		return "Int"
	case *ast.FloatLit:
		return "Float"
	case *ast.StringLit:
		return "String"
	case *ast.BoolLit:
		return "Bool"
	case *ast.BinaryExpr:
		return "Binary"
	case *ast.UnaryExpr:
		return "Unary"
	case *ast.CallExpr:
		return "Call"
	case *ast.FieldExpr:
		return "Field"
	case *ast.IndexExpr:
		return "Index"
	case *ast.StructLit:
		return "StructLit"
	}
	return "Expr"
}
