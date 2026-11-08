// Package checker performs semantic analysis: it builds symbol tables, verifies
// types and records the type of every expression for the code generator.
package checker

import (
	"fmt"

	"brick/internal/ast"
)

// Error is a semantic error with a source position.
type Error struct {
	Msg  string
	Line int
	Col  int
}

func (e *Error) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg) }

// FuncInfo is a function signature.
type FuncInfo struct {
	Params []ast.Param
	Ret    ast.Type
	Decl   *ast.FuncDecl
}

// ConstInfo is a compile-time constant: its type and its folded literal value.
type ConstInfo struct {
	Type ast.Type
	Lit  ast.Expr // a literal node the code generator and interpreter can reuse
}

// StructInfo is a struct definition.
type StructInfo struct {
	Name   string
	Fields map[string]ast.Type
	Order  []string
	Decl   *ast.StructDecl
}

// EnumInfo is an enum definition: an ordered list of tagged variants.
type EnumInfo struct {
	Name     string
	Order    []string               // variant names, in declaration order
	Variants map[string]ast.Variant // by name
	Decl     *ast.EnumDecl
}

// Index returns the tag value of a variant (-1 if unknown).
func (e *EnumInfo) Index(name string) int {
	for i, v := range e.Order {
		if v == name {
			return i
		}
	}
	return -1
}

// Checker holds program-wide symbols and inferred expression types.
type Checker struct {
	Funcs   map[string]*FuncInfo
	Structs map[string]*StructInfo
	Enums   map[string]*EnumInfo
	Consts  map[string]*ConstInfo
	Types   map[ast.Expr]ast.Type

	scopes    []map[string]ast.Type
	curRet    ast.Type
	inMain    bool
	loopDepth int
}

var (
	intT   = ast.Type{Name: "int"}
	floatT = ast.Type{Name: "float"}
	boolT  = ast.Type{Name: "bool"}
	textT  = ast.Type{Name: "text"}
	bytesT = ast.Type{Name: "bytes"}
	voidT  = ast.Type{Name: "void"}
)

// Check analyses a program, returning a *Error on the first problem found.
func Check(prog *ast.Program) (c *Checker, err error) {
	c = &Checker{
		Funcs:   map[string]*FuncInfo{},
		Structs: map[string]*StructInfo{},
		Enums:   map[string]*EnumInfo{},
		Consts:  map[string]*ConstInfo{},
		Types:   map[ast.Expr]ast.Type{},
	}
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*Error); ok {
				c = nil
				err = ce
				return
			}
			panic(r)
		}
	}()
	c.collect(prog)
	c.checkBodies(prog)
	return c, nil
}

func (c *Checker) errorf(n ast.Node, format string, args ...any) {
	line, col := 0, 0
	if n != nil {
		line, col = n.Pos()
	}
	panic(&Error{Msg: fmt.Sprintf(format, args...), Line: line, Col: col})
}

func (c *Checker) resolve(n ast.Node, t ast.Type) ast.Type {
	if t.Name == "List" {
		if t.Elem == nil {
			c.errorf(n, "List needs an element type, e.g. List(int)")
		}
		et := c.resolve(n, *t.Elem)
		return ast.Type{Name: "List", Ptr: t.Ptr, Elem: &et}
	}
	base := t.Name
	switch base {
	case "int", "float", "bool", "text", "bytes", "void":
		return t
	default:
		if _, ok := c.Structs[base]; ok {
			return t
		}
		if _, ok := c.Enums[base]; ok {
			return t
		}
		c.errorf(n, "unknown type %q", base)
	}
	return t
}

// isBuiltinTypeName reports whether name is one of the type names the language
// itself provides. A struct or enum may not take one: the name would be
// ambiguous everywhere a type is expected.
func isBuiltinTypeName(name string) bool {
	switch name {
	case "int", "float", "bool", "text", "bytes", "void":
		return true
	}
	return false
}

// isBuiltinFuncName reports whether name is one of the functions the language
// itself provides, such as print or exit. Declaring a function with one of
// these names would be silently ignored at every call site.
func isBuiltinFuncName(name string) bool {
	switch name {
	case "print", "print_bytes", "eprint", "exit", "system", "getenv",
		"read_dir", "is_dir", "read_file", "write_file", "slice", "args",
		"to_text", "to_bytes":
		return true
	}
	return false
}

// ---------- pass 1: declarations ----------

func (c *Checker) collect(prog *ast.Program) {
	// structs first so functions may refer to them
	for _, s := range prog.Stmts {
		sd, ok := s.(*ast.StructDecl)
		if !ok {
			continue
		}
		if _, dup := c.Structs[sd.Name]; dup {
			c.errorf(sd, "duplicate struct %q", sd.Name)
		}
		if isBuiltinTypeName(sd.Name) {
			c.errorf(sd, "cannot name a struct %q: %s is a built-in type", sd.Name, sd.Name)
		}
		si := &StructInfo{Name: sd.Name, Fields: map[string]ast.Type{}, Decl: sd}
		for _, f := range sd.Fields {
			if _, dup := si.Fields[f.Name]; dup {
				c.errorf(sd, "struct %q has duplicate field %q", sd.Name, f.Name)
			}
			si.Fields[f.Name] = f.Type
			si.Order = append(si.Order, f.Name)
		}
		c.Structs[sd.Name] = si
	}
	// enums
	for _, s := range prog.Stmts {
		ed, ok := s.(*ast.EnumDecl)
		if !ok {
			continue
		}
		if _, dup := c.Enums[ed.Name]; dup {
			c.errorf(ed, "duplicate enum %q", ed.Name)
		}
		if _, dup := c.Structs[ed.Name]; dup {
			c.errorf(ed, "name %q is already a struct", ed.Name)
		}
		if isBuiltinTypeName(ed.Name) {
			c.errorf(ed, "cannot name an enum %q: %s is a built-in type", ed.Name, ed.Name)
		}
		ei := &EnumInfo{Name: ed.Name, Variants: map[string]ast.Variant{}, Decl: ed}
		for _, v := range ed.Variants {
			if _, dup := ei.Variants[v.Name]; dup {
				c.errorf(ed, "enum %q has duplicate variant %q", ed.Name, v.Name)
			}
			ei.Variants[v.Name] = v
			ei.Order = append(ei.Order, v.Name)
		}
		c.Enums[ed.Name] = ei
	}
	// validate struct field and enum payload types now that all names are known
	for _, si := range c.Structs {
		for _, f := range si.Decl.Fields {
			c.resolve(si.Decl, f.Type)
		}
	}
	for _, ei := range c.Enums {
		for _, v := range ei.Decl.Variants {
			for _, ft := range v.Fields {
				c.resolve(ei.Decl, ft)
			}
		}
	}
	// constants (evaluated left to right, so a const may reference earlier ones)
	for _, s := range prog.Stmts {
		cd, ok := s.(*ast.ConstDecl)
		if !ok {
			continue
		}
		if _, dup := c.Consts[cd.Name]; dup {
			c.errorf(cd, "duplicate constant %q", cd.Name)
		}
		lit, t := c.evalConst(cd, cd.Value)
		if cd.Type != nil {
			decl := c.resolve(cd, *cd.Type)
			if !decl.Equals(t) {
				c.errorf(cd, "constant %s: value is %s, not %s (no implicit conversion)", cd.Name, t, decl)
			}
			t = decl
		}
		c.Consts[cd.Name] = &ConstInfo{Type: t, Lit: lit}
	}

	// functions
	for _, s := range prog.Stmts {
		fd, ok := s.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if _, dup := c.Funcs[fd.Name]; dup {
			c.errorf(fd, "duplicate function %q", fd.Name)
		}
		if isBuiltinFuncName(fd.Name) {
			c.errorf(fd, "cannot name a function %q: %s is a built-in function", fd.Name, fd.Name)
		}
		for _, p := range fd.Params {
			c.resolve(fd, p.Type)
		}
		if fd.ReturnType.Name != "void" {
			c.resolve(fd, fd.ReturnType)
		}
		c.Funcs[fd.Name] = &FuncInfo{Params: fd.Params, Ret: fd.ReturnType, Decl: fd}
	}
	// top level must be declarations only
	for _, s := range prog.Stmts {
		switch s.(type) {
		case *ast.FuncDecl, *ast.StructDecl, *ast.EnumDecl, *ast.ImportStmt, *ast.ConstDecl:
		default:
			c.errorf(s, "only declarations are allowed at the top level (put code inside a function)")
		}
	}
}

// ---------- pass 2: bodies ----------

func (c *Checker) checkBodies(prog *ast.Program) {
	for _, s := range prog.Stmts {
		if fd, ok := s.(*ast.FuncDecl); ok && !fd.Extern && fd.Body != nil {
			c.checkFunc(fd)
		}
	}
}

func (c *Checker) checkFunc(fd *ast.FuncDecl) {
	c.curRet = fd.ReturnType
	c.inMain = fd.Name == "main"
	c.push()
	for _, p := range fd.Params {
		c.declare(fd, p.Name, p.Type)
	}
	for _, s := range fd.Body.Stmts {
		c.checkStmt(s)
	}
	c.pop()
	c.inMain = false
}

// ---------- scopes ----------

func (c *Checker) push() { c.scopes = append(c.scopes, map[string]ast.Type{}) }
func (c *Checker) pop()  { c.scopes = c.scopes[:len(c.scopes)-1] }

func (c *Checker) declare(n ast.Node, name string, t ast.Type) {
	if _, dup := c.scopes[len(c.scopes)-1][name]; dup {
		c.errorf(n, "%q is already declared in this scope", name)
	}
	c.scopes[len(c.scopes)-1][name] = t
}

func (c *Checker) lookupVar(name string) (ast.Type, bool) {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if t, ok := c.scopes[i][name]; ok {
			return t, true
		}
	}
	return ast.Type{}, false
}

// ---------- statements ----------

func (c *Checker) checkStmt(s ast.Stmt) {
	switch st := s.(type) {
	case *ast.VarStmt:
		vt := c.infer(st.Value)
		if st.Type != nil {
			decl := c.resolve(st, *st.Type)
			if !decl.Equals(vt) {
				c.errorf(st, "cannot assign %s to %q (no implicit conversion)", vt, decl)
			}
			vt = decl
		}
		c.declare(st, st.Name, vt)

	case *ast.AssignStmt:
		switch st.Target.(type) {
		case *ast.Ident, *ast.FieldExpr:
		case *ast.IndexExpr:
			bt := c.infer(st.Target.(*ast.IndexExpr).Base)
			if bt.Name != "List" {
				c.errorf(st, "cannot assign to this index")
			}
		default:
			c.errorf(st, "cannot assign to this expression")
		}
		tt := c.infer(st.Target)
		vt := c.infer(st.Value)
		if !tt.Equals(vt) {
			c.errorf(st, "cannot assign %s to %s (no implicit conversion)", vt, tt)
		}

	case *ast.ReturnStmt:
		if c.inMain && c.curRet.Name == "void" {
			// main may return nothing or an int
		}
		if st.Value == nil {
			if c.curRet.Name != "void" {
				c.errorf(st, "function must return a value of type %s", c.curRet)
			}
			return
		}
		vt := c.infer(st.Value)
		if c.curRet.Name == "void" {
			c.errorf(st, "cannot return a value from a function that returns nothing")
		}
		if !vt.Equals(c.curRet) {
			c.errorf(st, "cannot return %s from a function that returns %s", vt, c.curRet)
		}

	case *ast.ExprStmt:
		c.infer(st.X)

	case *ast.BlockStmt:
		c.push()
		for _, inner := range st.Stmts {
			c.checkStmt(inner)
		}
		c.pop()

	case *ast.IfStmt:
		if ct := c.infer(st.Cond); !ct.Equals(boolT) {
			c.errorf(st, "if condition must be bool, got %s", ct)
		}
		c.checkStmt(st.Then)
		if st.Else != nil {
			c.checkStmt(st.Else)
		}

	case *ast.WhileStmt:
		if ct := c.infer(st.Cond); !ct.Equals(boolT) {
			c.errorf(st, "while condition must be bool, got %s", ct)
		}
		c.loopDepth++
		c.checkStmt(st.Body)
		c.loopDepth--

	case *ast.ForStmt:
		it := c.infer(st.Iter)
		var et ast.Type
		switch {
		case it.Name == "List" && it.Elem != nil:
			et = *it.Elem
		case it.Name == "text" && !it.Ptr:
			et = intT // iterating text yields each byte as an int
		case it.Name == "bytes" && !it.Ptr:
			et = intT // iterating bytes yields each byte as an int
		default:
			c.errorf(st, "for can iterate a List or text, got %s", it)
		}
		c.push()
		c.declare(st, st.Var, et)
		c.loopDepth++
		c.checkStmt(st.Body)
		c.loopDepth--
		c.pop()

	case *ast.BreakStmt:
		if c.loopDepth == 0 {
			c.errorf(st, "break outside a loop")
		}

	case *ast.ContinueStmt:
		if c.loopDepth == 0 {
			c.errorf(st, "continue outside a loop")
		}

	case *ast.ArenaStmt:
		// An arena is a scope: names declared inside do not escape, and the
		// memory is freed at the end of the block (§6).
		c.checkStmt(st.Body)

	case *ast.MatchStmt:
		c.checkMatch(st)

	case *ast.FuncDecl, *ast.StructDecl, *ast.ConstDecl:
		c.errorf(s, "nested declarations are not allowed (const belongs at the top level)")

	default:
		c.errorf(s, "unhandled statement")
	}
}

// ---------- expressions ----------

// evalConst folds a constant expression to a literal node and its type. It
// accepts literals, references to earlier constants, unary +/-/!, and binary
// arithmetic, comparison and logical operators on those.
func (c *Checker) evalConst(n ast.Node, e ast.Expr) (ast.Expr, ast.Type) {
	switch ex := e.(type) {
	case *ast.IntLit:
		return ex, intT
	case *ast.FloatLit:
		return ex, floatT
	case *ast.StringLit:
		return ex, textT
	case *ast.BoolLit:
		return ex, boolT
	case *ast.Ident:
		ci, ok := c.Consts[ex.Name]
		if !ok {
			c.errorf(e, "%q is not a constant (a constant must be a literal or another constant)", ex.Name)
		}
		return ci.Lit, ci.Type
	case *ast.UnaryExpr:
		lit, t := c.evalConst(e, ex.Operand)
		switch ex.Op {
		case "-":
			switch v := lit.(type) {
			case *ast.IntLit:
				return &ast.IntLit{Value: -v.Value}, intT
			case *ast.FloatLit:
				return &ast.FloatLit{Value: -v.Value}, floatT
			}
		case "!":
			if b, ok := lit.(*ast.BoolLit); ok {
				return &ast.BoolLit{Value: !b.Value}, boolT
			}
		}
		_ = t
		c.errorf(e, "operator %q cannot be applied to a constant of type %s", ex.Op, t)
	case *ast.BinaryExpr:
		ll, lt := c.evalConst(e, ex.Left)
		rl, rt := c.evalConst(e, ex.Right)
		return c.evalConstBinary(e, ex, ll, lt, rl, rt)
	}
	c.errorf(e, "value is not a compile-time constant")
	return nil, voidT
}

func (c *Checker) evalConstBinary(n ast.Node, ex *ast.BinaryExpr, ll ast.Expr, lt ast.Type, rl ast.Expr, rt ast.Type) (ast.Expr, ast.Type) {
	if li, ok := ll.(*ast.IntLit); ok {
		if ri, ok := rl.(*ast.IntLit); ok {
			a, b := li.Value, ri.Value
			switch ex.Op {
			case "+":
				return &ast.IntLit{Value: a + b}, intT
			case "-":
				return &ast.IntLit{Value: a - b}, intT
			case "*":
				return &ast.IntLit{Value: a * b}, intT
			case "/":
				if b == 0 {
					c.errorf(n, "division by zero in constant")
				}
				return &ast.IntLit{Value: a / b}, intT
			case "%":
				if b == 0 {
					c.errorf(n, "division by zero in constant")
				}
				return &ast.IntLit{Value: a % b}, intT
			case "==":
				return &ast.BoolLit{Value: a == b}, boolT
			case "!=":
				return &ast.BoolLit{Value: a != b}, boolT
			case "<":
				return &ast.BoolLit{Value: a < b}, boolT
			case "<=":
				return &ast.BoolLit{Value: a <= b}, boolT
			case ">":
				return &ast.BoolLit{Value: a > b}, boolT
			case ">=":
				return &ast.BoolLit{Value: a >= b}, boolT
			}
		}
	}
	if lf, ok := ll.(*ast.FloatLit); ok {
		if rf, ok := rl.(*ast.FloatLit); ok {
			a, b := lf.Value, rf.Value
			switch ex.Op {
			case "+":
				return &ast.FloatLit{Value: a + b}, floatT
			case "-":
				return &ast.FloatLit{Value: a - b}, floatT
			case "*":
				return &ast.FloatLit{Value: a * b}, floatT
			case "/":
				return &ast.FloatLit{Value: a / b}, floatT
			case "==":
				return &ast.BoolLit{Value: a == b}, boolT
			case "!=":
				return &ast.BoolLit{Value: a != b}, boolT
			case "<":
				return &ast.BoolLit{Value: a < b}, boolT
			case "<=":
				return &ast.BoolLit{Value: a <= b}, boolT
			case ">":
				return &ast.BoolLit{Value: a > b}, boolT
			case ">=":
				return &ast.BoolLit{Value: a >= b}, boolT
			}
		}
	}
	if ls, ok := ll.(*ast.StringLit); ok {
		if rs, ok := rl.(*ast.StringLit); ok {
			switch ex.Op {
			case "+":
				return &ast.StringLit{Value: ls.Value + rs.Value}, textT
			case "==":
				return &ast.BoolLit{Value: ls.Value == rs.Value}, boolT
			case "!=":
				return &ast.BoolLit{Value: ls.Value != rs.Value}, boolT
			}
		}
	}
	if lb, ok := ll.(*ast.BoolLit); ok {
		if rb, ok := rl.(*ast.BoolLit); ok {
			switch ex.Op {
			case "&&":
				return &ast.BoolLit{Value: lb.Value && rb.Value}, boolT
			case "||":
				return &ast.BoolLit{Value: lb.Value || rb.Value}, boolT
			case "==":
				return &ast.BoolLit{Value: lb.Value == rb.Value}, boolT
			case "!=":
				return &ast.BoolLit{Value: lb.Value != rb.Value}, boolT
			}
		}
	}
	c.errorf(n, "unsupported constant expression (%s %s %s)", lt, ex.Op, rt)
	return nil, voidT
}

func (c *Checker) infer(e ast.Expr) ast.Type {
	t := c.inferInner(e)
	c.Types[e] = t
	return t
}

func (c *Checker) inferInner(e ast.Expr) ast.Type {
	switch ex := e.(type) {
	case *ast.IntLit:
		return intT
	case *ast.FloatLit:
		return floatT
	case *ast.StringLit:
		return textT
	case *ast.BoolLit:
		return boolT
	case *ast.Ident:
		if t, ok := c.lookupVar(ex.Name); ok {
			return t
		}
		if ci, ok := c.Consts[ex.Name]; ok {
			return ci.Type
		}
		c.errorf(ex, "undefined name %q", ex.Name)
	case *ast.UnaryExpr:
		return c.inferUnary(ex)
	case *ast.BinaryExpr:
		return c.inferBinary(ex)
	case *ast.CallExpr:
		return c.inferCall(ex)
	case *ast.FieldExpr:
		return c.inferField(ex)
	case *ast.IndexExpr:
		return c.inferIndex(ex)
	case *ast.StructLit:
		return c.inferStructLit(ex)
	}
	c.errorf(e, "internal error: unhandled expression")
	return voidT
}

func (c *Checker) inferUnary(ex *ast.UnaryExpr) ast.Type {
	t := c.infer(ex.Operand)
	switch ex.Op {
	case "-":
		if !t.IsNumeric() {
			c.errorf(ex, "operator '-' needs a number, got %s", t)
		}
		return t
	case "!":
		if !t.Equals(boolT) {
			c.errorf(ex, "operator '!' needs bool, got %s", t)
		}
		return boolT
	case "&", "&mut":
		ptr := t
		ptr.Ptr = true
		return ptr
	}
	c.errorf(ex, "unknown unary operator %q", ex.Op)
	return voidT
}

func (c *Checker) inferBinary(ex *ast.BinaryExpr) ast.Type {
	l := c.infer(ex.Left)
	r := c.infer(ex.Right)
	switch ex.Op {
	case "+":
		if l.Equals(textT) && r.Equals(textT) {
			return textT
		}
		if !l.IsNumeric() || !r.IsNumeric() {
			c.errorf(ex, "operator %q needs numbers or two texts, got %s and %s", ex.Op, l, r)
		}
		if !l.Equals(r) {
			c.errorf(ex, "operator %q needs matching types, got %s and %s", ex.Op, l, r)
		}
		return l
	case "-", "*", "/":
		if !l.IsNumeric() || !r.IsNumeric() {
			c.errorf(ex, "operator %q needs numbers, got %s and %s", ex.Op, l, r)
		}
		if !l.Equals(r) {
			c.errorf(ex, "operator %q needs matching types, got %s and %s", ex.Op, l, r)
		}
		return l
	case "%":
		if !l.Equals(intT) || !r.Equals(intT) {
			c.errorf(ex, "operator %% needs ints, got %s and %s", l, r)
		}
		return intT
	case "<", "<=", ">", ">=":
		if !l.IsNumeric() || !l.Equals(r) {
			c.errorf(ex, "operator %q needs two matching numbers, got %s and %s", ex.Op, l, r)
		}
		return boolT
	case "==", "!=":
		if !l.Equals(r) {
			c.errorf(ex, "operator %q needs matching types, got %s and %s", ex.Op, l, r)
		}
		return boolT
	case "&&", "||":
		if !l.Equals(boolT) || !r.Equals(boolT) {
			c.errorf(ex, "operator %q needs bools, got %s and %s", ex.Op, l, r)
		}
		return boolT
	}
	c.errorf(ex, "unknown binary operator %q", ex.Op)
	return voidT
}

func (c *Checker) inferCall(ex *ast.CallExpr) ast.Type {
	// method-style call: list.push(x)
	if fe, ok := ex.Callee.(*ast.FieldExpr); ok && fe.Field == "push" {
		bt := c.infer(fe.Base)
		if bt.Name != "List" || bt.Elem == nil {
			c.errorf(ex, "push is only available on List values, got %s", bt)
		}
		if len(ex.Args) != 1 {
			c.errorf(ex, "push takes exactly 1 argument, got %d", len(ex.Args))
		}
		at := c.infer(ex.Args[0])
		if !at.Equals(*bt.Elem) {
			c.errorf(ex, "push: expected %s, got %s", *bt.Elem, at)
		}
		return voidT
	}
	// map methods: m.set(k, v), m.get(k), m.has(k)
	if fe, ok := ex.Callee.(*ast.FieldExpr); ok {
		switch fe.Field {
		case "set", "get", "has":
			if bid, ok := fe.Base.(*ast.Ident); ok {
				if _, isVar := c.lookupVar(bid.Name); isVar {
					if bt := c.infer(fe.Base); bt.Name == "Map" && bt.Key != nil && bt.Elem != nil {
						return c.inferMapMethod(ex, fe, bt)
					}
				}
			}
		}
	}
	// builder methods: b.add(s), b.build(). Only look at a variable base, so an
	// enum constructor like Result.Ok(n) is not mistaken for a method call.
	if fe, ok := ex.Callee.(*ast.FieldExpr); ok {
		if bid, ok := fe.Base.(*ast.Ident); ok {
			if _, isVar := c.lookupVar(bid.Name); isVar {
				if bt := c.infer(fe.Base); bt.Name == "Builder" {
					return c.inferBuilderMethod(ex, fe)
				}
			}
		}
	}
	// enum variant constructor: Enum.Variant(args...)
	if fe, ok := ex.Callee.(*ast.FieldExpr); ok {
		if bid, ok := fe.Base.(*ast.Ident); ok {
			if _, isVar := c.lookupVar(bid.Name); !isVar {
				if ei, ok := c.Enums[bid.Name]; ok {
					v, ok := ei.Variants[fe.Field]
					if !ok {
						c.errorf(ex, "enum %s has no variant %q", bid.Name, fe.Field)
					}
					if len(ex.Args) != len(v.Fields) {
						c.errorf(ex, "variant %s.%s takes %d argument(s), got %d", bid.Name, fe.Field, len(v.Fields), len(ex.Args))
					}
					for i, a := range ex.Args {
						at := c.infer(a)
						if !at.Equals(v.Fields[i]) {
							c.errorf(ex, "argument %d of %s.%s: expected %s, got %s", i+1, bid.Name, fe.Field, v.Fields[i], at)
						}
					}
					return ast.Type{Name: bid.Name}
				}
			}
		}
	}
	id, ok := ex.Callee.(*ast.Ident)
	if !ok {
		c.errorf(ex, "can only call named functions")
	}
	// builtin
	if id.Name == "print" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "print takes exactly 1 argument, got %d", len(ex.Args))
		}
		at := c.infer(ex.Args[0])
		if at.Ptr {
			c.errorf(ex, "print cannot print a reference")
		}
		return voidT
	}
	if id.Name == "print_bytes" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "print_bytes takes exactly 1 argument (bytes), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(bytesT) {
			c.errorf(ex, "print_bytes expects bytes, got %s", at)
		}
		return voidT
	}
	if id.Name == "exit" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "exit takes exactly 1 argument (a status), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(intT) {
			c.errorf(ex, "exit status must be int, got %s", at)
		}
		return voidT
	}
	if id.Name == "eprint" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "eprint takes exactly 1 argument, got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); at.Ptr {
			c.errorf(ex, "eprint cannot print a reference")
		}
		return voidT
	}
	// system(argv): run a program without a shell; returns its exit status.
	if id.Name == "system" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "system takes exactly 1 argument (a List(text) of program arguments), got %d", len(ex.Args))
		}
		elem := textT
		want := ast.Type{Name: "List", Elem: &elem}
		if at := c.infer(ex.Args[0]); !at.Equals(want) {
			c.errorf(ex, "system expects a List(text) of program arguments, got %s", at)
		}
		return intT
	}
	// read_dir(path): the names in a directory, sorted; empty when it cannot
	// be opened. ("." and ".." are never included.)
	if id.Name == "read_dir" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "read_dir takes exactly 1 argument (a path), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "read_dir path must be text, got %s", at)
		}
		elem := textT
		return ast.Type{Name: "List", Elem: &elem}
	}
	// is_dir(path): whether a path exists and is a directory.
	if id.Name == "is_dir" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "is_dir takes exactly 1 argument (a path), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "is_dir path must be text, got %s", at)
		}
		return boolT
	}
	// getenv(name): an environment variable, or "" when it is not set.
	if id.Name == "getenv" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "getenv takes exactly 1 argument (a name), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "getenv name must be text, got %s", at)
		}
		return textT
	}
	if id.Name == "read_file" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "read_file takes exactly 1 argument (a path), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "read_file path must be text, got %s", at)
		}
		return textT
	}
	if id.Name == "write_file" {
		if len(ex.Args) != 2 {
			c.errorf(ex, "write_file takes 2 arguments (path, contents), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "write_file path must be text, got %s", at)
		}
		if at := c.infer(ex.Args[1]); !at.Equals(textT) {
			c.errorf(ex, "write_file contents must be text, got %s", at)
		}
		return voidT
	}
	if id.Name == "args" {
		if len(ex.Args) != 0 {
			c.errorf(ex, "args takes no arguments, got %d", len(ex.Args))
		}
		elem := textT
		return ast.Type{Name: "List", Elem: &elem}
	}
	// List(T): an empty growable list of element type T
	if id.Name == "List" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "List(...) takes exactly one type, e.g. List(int)")
		}
		argIdent, ok := ex.Args[0].(*ast.Ident)
		if !ok {
			c.errorf(ex, "List(...) needs a type name, e.g. List(int)")
		}
		et := c.resolve(ex, ast.Type{Name: argIdent.Name})
		return ast.Type{Name: "List", Elem: &et}
	}
	if id.Name == "slice" {
		if len(ex.Args) != 3 {
			c.errorf(ex, "slice takes 3 arguments (text, start, end), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "slice source must be text, got %s", at)
		}
		for _, i := range []int{1, 2} {
			if at := c.infer(ex.Args[i]); !at.Equals(intT) {
				c.errorf(ex, "slice argument %d must be an int, got %s", i+1, at)
			}
		}
		return textT
	}
	if id.Name == "to_text" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "to_text takes exactly 1 argument (an int, float, bool or bytes), got %d", len(ex.Args))
		}
		at := c.infer(ex.Args[0])
		if at.Equals(bytesT) {
			return textT
		}
		if !at.Equals(intT) && !at.Equals(floatT) && !at.Equals(boolT) {
			c.errorf(ex, "to_text expects an int, float, bool or bytes, got %s", at)
		}
		return textT
	}
	// to_bytes(t): a length-carrying byte string with the bytes of t.
	if id.Name == "to_bytes" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "to_bytes takes exactly 1 argument (text), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "to_bytes expects text, got %s", at)
		}
		return bytesT
	}
	// bytes(xs): a byte string built from a List(int) of byte values (0..255).
	if id.Name == "bytes" {
		if len(ex.Args) != 1 {
			c.errorf(ex, "bytes(...) takes exactly one List(int), got %d", len(ex.Args))
		}
		at := c.infer(ex.Args[0])
		want := ast.Type{Name: "List", Elem: &intT}
		if !at.Equals(want) {
			c.errorf(ex, "bytes(...) expects a List(int) of byte values, got %s", at)
		}
		return bytesT
	}
	// Builder(): a growable text buffer, for building output without quadratic +.
	if id.Name == "Builder" {
		if len(ex.Args) != 0 {
			c.errorf(ex, "Builder() takes no arguments, got %d", len(ex.Args))
		}
		return ast.Type{Name: "Builder"}
	}
	// Map(K, V): an empty map. Keys must be text for now.
	if id.Name == "Map" {
		if len(ex.Args) != 2 {
			c.errorf(ex, "Map(...) takes a key type and a value type, e.g. Map(text, int)")
		}
		keyIdent, ok := ex.Args[0].(*ast.Ident)
		if !ok {
			c.errorf(ex, "Map(...) key must be a type name, e.g. Map(text, int)")
		}
		valIdent, ok := ex.Args[1].(*ast.Ident)
		if !ok {
			c.errorf(ex, "Map(...) value must be a type name, e.g. Map(text, int)")
		}
		kt := c.resolve(ex, ast.Type{Name: keyIdent.Name})
		if !kt.Equals(textT) {
			c.errorf(ex, "Map keys must be text for now, got %s", kt)
		}
		vt := c.resolve(ex, ast.Type{Name: valIdent.Name})
		return ast.Type{Name: "Map", Key: &kt, Elem: &vt}
	}
	fn, ok := c.Funcs[id.Name]
	if !ok {
		c.errorf(ex, "undefined function %q", id.Name)
	}
	if len(ex.Args) != len(fn.Params) {
		c.errorf(ex, "%s takes %d argument(s), got %d", id.Name, len(fn.Params), len(ex.Args))
	}
	for i, a := range ex.Args {
		at := c.infer(a)
		if !at.Equals(fn.Params[i].Type) {
			c.errorf(ex, "argument %d of %s: expected %s, got %s", i+1, id.Name, fn.Params[i].Type, at)
		}
	}
	return fn.Ret
}

// inferBuilderMethod checks a call like b.add(s) or b.build().
func (c *Checker) inferBuilderMethod(ex *ast.CallExpr, fe *ast.FieldExpr) ast.Type {
	switch fe.Field {
	case "add":
		if len(ex.Args) != 1 {
			c.errorf(ex, "add takes 1 argument (text), got %d", len(ex.Args))
		}
		if at := c.infer(ex.Args[0]); !at.Equals(textT) {
			c.errorf(ex, "add expects text, got %s", at)
		}
		return voidT
	case "build":
		if len(ex.Args) != 0 {
			c.errorf(ex, "build takes no arguments, got %d", len(ex.Args))
		}
		return textT
	}
	c.errorf(ex, "Builder has no method %q (try add or build)", fe.Field)
	return voidT
}

// inferMapMethod checks a call like m.set(k, v), m.get(k) or m.has(k).
func (c *Checker) inferMapMethod(ex *ast.CallExpr, fe *ast.FieldExpr, bt ast.Type) ast.Type {
	checkKey := func(a ast.Expr) {
		if kt := c.infer(a); !kt.Equals(*bt.Key) {
			c.errorf(ex, "map key must be %s, got %s", *bt.Key, kt)
		}
	}
	switch fe.Field {
	case "set":
		if len(ex.Args) != 2 {
			c.errorf(ex, "set takes 2 arguments (key, value), got %d", len(ex.Args))
		}
		checkKey(ex.Args[0])
		if vt := c.infer(ex.Args[1]); !vt.Equals(*bt.Elem) {
			c.errorf(ex, "map value must be %s, got %s", *bt.Elem, vt)
		}
		return voidT
	case "get":
		if len(ex.Args) != 1 {
			c.errorf(ex, "get takes 1 argument (key), got %d", len(ex.Args))
		}
		checkKey(ex.Args[0])
		return *bt.Elem
	case "has":
		if len(ex.Args) != 1 {
			c.errorf(ex, "has takes 1 argument (key), got %d", len(ex.Args))
		}
		checkKey(ex.Args[0])
		return boolT
	}
	return voidT
}

func (c *Checker) inferField(ex *ast.FieldExpr) ast.Type {
	if id, ok := ex.Base.(*ast.Ident); ok {
		if _, isVar := c.lookupVar(id.Name); !isVar {
			if ei, ok := c.Enums[id.Name]; ok {
				v, ok := ei.Variants[ex.Field]
				if !ok {
					c.errorf(ex, "enum %s has no variant %q", id.Name, ex.Field)
				}
				if len(v.Fields) != 0 {
					c.errorf(ex, "variant %s.%s needs %d argument(s)", id.Name, ex.Field, len(v.Fields))
				}
				return ast.Type{Name: id.Name}
			}
			c.errorf(ex, "undefined name %q", id.Name)
		}
	}
	bt := c.infer(ex.Base)
	if ex.Field == "len" {
		if bt.Name == "List" || bt.Name == "text" || bt.Name == "Map" || bt.Name == "Builder" || bt.Name == "bytes" {
			return intT
		}
	}
	si, ok := c.Structs[bt.Name]
	if !ok {
		c.errorf(ex, "cannot access field %q on non-struct type %s", ex.Field, bt)
	}
	ft, ok := si.Fields[ex.Field]
	if !ok {
		c.errorf(ex, "struct %s has no field %q", bt.Name, ex.Field)
	}
	return ft
}

func (c *Checker) inferIndex(ex *ast.IndexExpr) ast.Type {
	bt := c.infer(ex.Base)
	it := c.infer(ex.Index)
	if !it.Equals(intT) {
		c.errorf(ex, "index must be int, got %s", it)
	}
	if bt.Name == "List" && bt.Elem != nil {
		return *bt.Elem
	}
	if bt.Name == "text" && !bt.Ptr {
		return intT
	}
	if bt.Name == "bytes" && !bt.Ptr {
		return intT
	}
	c.errorf(ex, "cannot index a value of type %s", bt)
	return voidT
}

func (c *Checker) inferStructLit(ex *ast.StructLit) ast.Type {
	si, ok := c.Structs[ex.Name]
	if !ok {
		c.errorf(ex, "unknown struct %q", ex.Name)
	}
	seen := map[string]bool{}
	for _, f := range ex.Fields {
		ft, ok := si.Fields[f.Name]
		if !ok {
			c.errorf(ex, "struct %s has no field %q", ex.Name, f.Name)
		}
		if seen[f.Name] {
			c.errorf(ex, "field %q set twice", f.Name)
		}
		seen[f.Name] = true
		vt := c.infer(f.Value)
		if !vt.Equals(ft) {
			c.errorf(ex, "field %s.%s: expected %s, got %s", ex.Name, f.Name, ft, vt)
		}
	}
	for _, name := range si.Order {
		if !seen[name] {
			c.errorf(ex, "struct %s: missing field %q", ex.Name, name)
		}
	}
	return ast.Type{Name: ex.Name}
}

// ---------- match ----------

func (c *Checker) checkMatch(st *ast.MatchStmt) {
	stT := c.infer(st.Scrutinee)
	ei, ok := c.Enums[stT.Name]
	if !ok {
		c.errorf(st, "match expects an enum, got %s", stT)
	}
	covered := map[string]bool{}
	wild := false
	for i := range st.Arms {
		arm := &st.Arms[i]
		p := arm.Pattern
		if p.IsWild {
			if wild {
				c.errorf(st, "match has more than one wildcard arm")
			}
			wild = true
			c.push()
			c.checkStmt(arm.Body)
			c.pop()
			continue
		}
		if p.Enum != stT.Name {
			c.errorf(&p, "pattern %s.%s does not match enum %s", p.Enum, p.Variant, stT.Name)
		}
		v, ok := ei.Variants[p.Variant]
		if !ok {
			c.errorf(&p, "enum %s has no variant %q", stT.Name, p.Variant)
		}
		if len(p.Binds) != len(v.Fields) {
			c.errorf(&p, "variant %s.%s binds %d name(s), wants %d", stT.Name, p.Variant, len(p.Binds), len(v.Fields))
		}
		if covered[p.Variant] {
			c.errorf(&p, "variant %s.%s is matched more than once", stT.Name, p.Variant)
		}
		covered[p.Variant] = true
		c.push()
		for j, b := range p.Binds {
			c.declare(&p, b, v.Fields[j])
		}
		c.checkStmt(arm.Body)
		c.pop()
	}
	if !wild {
		for _, name := range ei.Order {
			if !covered[name] {
				c.errorf(st, "match on %s is missing variant %s.%s", stT.Name, stT.Name, name)
			}
		}
	}
}
