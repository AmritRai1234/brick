// Package interp is a slow, simple tree-walking interpreter for Brick.
//
// It exists for differential testing (§10): run the same program through the
// interpreter and through the compiled binary, and compare the output. Any
// difference means one of them has a bug. The interpreter is deliberately
// naive -- it favors being obviously correct over being fast.
package interp

import (
	stdbytes "bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"brick/internal/ast"
	"brick/internal/checker"
)

// RuntimeError is a Brick-level runtime failure (e.g. an out-of-range index).
type RuntimeError struct {
	Msg  string
	Line int
	Col  int
}

func (e *RuntimeError) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg) }

// ---------- values ----------

// Value is any Brick runtime value.
type Value interface{}

// List is a growable list value.
type List struct {
	Elem  ast.Type
	Items []Value
}

// Builder is a growable text buffer.
type Builder struct {
	Buf []byte
}

// Bytes is a length-carrying byte string, so it can hold bytes that are not
// printable (including NUL) without truncation.
type Bytes struct {
	Data []byte
}

// Map is a map value with text keys.
type Map struct {
	Key  ast.Type
	Elem ast.Type
	Data map[string]Value
}

// Struct is an instance of a struct type. Fields are slots so a field can be
// the target of a borrow (e.g. p.x = ... through a &mut).
type Struct struct {
	Type   string
	Fields map[string]*Value
}

// Enum is a tagged-union value.
type Enum struct {
	Type    string
	Variant string
	Fields  []Value
}

// Ref is a borrow (& or &mut): a pointer to a variable slot.
type Ref struct {
	Slot *Value
}

// ---------- environment ----------

// Env maps names to variable slots. Slots are pointers so a borrow can alias
// the variable it points at.
type Env struct {
	parent *Env
	vars   map[string]*Value
}

func newEnv(parent *Env) *Env {
	return &Env{parent: parent, vars: map[string]*Value{}}
}

func (e *Env) declare(name string, v Value) *Value {
	slot := new(Value)
	*slot = v
	e.vars[name] = slot
	return slot
}

func (e *Env) lookup(name string) (*Value, bool) {
	for env := e; env != nil; env = env.parent {
		if slot, ok := env.vars[name]; ok {
			return slot, true
		}
	}
	return nil, false
}

// ---------- interpreter ----------

type interp struct {
	ck      *checker.Checker
	funcs   map[string]*ast.FuncDecl
	globals *Env
	out     io.Writer
	errw    io.Writer
	args    []string
	steps   int
}

// Options configures a Run.
type Options struct {
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
}

// Run interprets prog. The program must already have passed the checker; its
// results are reused for types and struct/enum layouts.
func Run(prog *ast.Program, ck *checker.Checker, opt Options) error {
	in := &interp{
		ck:      ck,
		funcs:   map[string]*ast.FuncDecl{},
		globals: newEnv(nil),
		out:     opt.Stdout,
		errw:    opt.Stderr,
		args:    opt.Args,
	}
	if in.out == nil {
		in.out = os.Stdout
	}
	if in.errw == nil {
		in.errw = os.Stderr
	}
	for _, s := range prog.Stmts {
		if fd, ok := s.(*ast.FuncDecl); ok {
			in.funcs[fd.Name] = fd
		}
	}
	main, ok := in.funcs["main"]
	if !ok {
		return &RuntimeError{Msg: "no main function"}
	}
	_, err := in.callFunc(main, nil)
	return err
}

const stepLimit = 200_000_000

func (in *interp) tick(line, col int) error {
	in.steps++
	if in.steps > stepLimit {
		return &RuntimeError{Msg: "step limit exceeded (possible infinite loop)", Line: line, Col: col}
	}
	return nil
}

// ---------- statements ----------

func (in *interp) execStmts(stmts []ast.Stmt, env *Env) error {
	for _, s := range stmts {
		if err := in.execStmt(s, env); err != nil {
			return err
		}
	}
	return nil
}

func (in *interp) execStmt(s ast.Stmt, env *Env) error {
	line, col := s.Pos()
	if err := in.tick(line, col); err != nil {
		return err
	}
	switch st := s.(type) {
	case *ast.VarStmt:
		v, err := in.eval(st.Value, env)
		if err != nil {
			return err
		}
		env.declare(st.Name, deepCopy(read(v)))
		return nil
	case *ast.AssignStmt:
		v, err := in.eval(st.Value, env)
		if err != nil {
			return err
		}
		slot, err := in.lvalue(st.Target, env)
		if err != nil {
			return err
		}
		*slot = deepCopy(read(v))
		return nil
	case *ast.ReturnStmt:
		if st.Value == nil {
			return returnSignal{nil}
		}
		v, err := in.eval(st.Value, env)
		if err != nil {
			return err
		}
		return returnSignal{read(v)}
	case *ast.ExprStmt:
		_, err := in.eval(st.X, env)
		return err
	case *ast.BlockStmt:
		return in.execStmts(st.Stmts, newEnv(env))
	case *ast.ArenaStmt:
		// The interpreter has no memory to reclaim; an arena is just a scope.
		return in.execStmt(st.Body, env)
	case *ast.IfStmt:
		c, err := in.eval(st.Cond, env)
		if err != nil {
			return err
		}
		if read(c).(bool) {
			return in.execStmt(st.Then, env)
		}
		if st.Else != nil {
			return in.execStmt(st.Else, env)
		}
		return nil
	case *ast.WhileStmt:
		for {
			c, err := in.eval(st.Cond, env)
			if err != nil {
				return err
			}
			if !read(c).(bool) {
				return nil
			}
			if err := in.execStmt(st.Body, env); err != nil {
				if _, ok := err.(continueSignal); ok {
					continue
				}
				if _, ok := err.(breakSignal); ok {
					return nil
				}
				return err
			}
		}

	case *ast.ForStmt:
		return in.execFor(st, env)

	case *ast.BreakStmt:
		return breakSignal{}

	case *ast.ContinueStmt:
		return continueSignal{}
	case *ast.MatchStmt:
		return in.execMatch(st, env)
	case *ast.FuncDecl:
		in.funcs[st.Name] = st // forward/extern declarations
		return nil
	case *ast.StructDecl, *ast.EnumDecl, *ast.ImportStmt, *ast.ConstDecl:
		return nil
	}
	return &RuntimeError{Msg: "unsupported statement", Line: line, Col: col}
}

func (in *interp) execFor(st *ast.ForStmt, env *Env) error {
	v, err := in.eval(st.Iter, env)
	if err != nil {
		return err
	}
	run := func(item Value) error {
		iterScope := newEnv(env)
		iterScope.declare(st.Var, deepCopy(item))
		err := in.execStmt(st.Body, iterScope)
		if _, ok := err.(continueSignal); ok {
			return nil
		}
		if _, ok := err.(breakSignal); ok {
			return errBreak
		}
		return err
	}
	switch it := read(v).(type) {
	case *List:
		for _, item := range it.Items {
			if err := run(item); err != nil {
				if err == errBreak {
					return nil
				}
				return err
			}
		}
	case *Bytes:
		for _, c := range it.Data {
			if err := run(int64(c)); err != nil {
				if err == errBreak {
					return nil
				}
				return err
			}
		}
	case string:
		for i := 0; i < len(it); i++ {
			if err := run(int64(it[i])); err != nil {
				if err == errBreak {
					return nil
				}
				return err
			}
		}
	default:
		return &RuntimeError{Msg: "for can only iterate a List or text", Line: st.Line, Col: st.Col}
	}
	return nil
}

// errBreak is an internal marker used by execFor to stop iterating.
var errBreak = errors.New("break")

// returnSignal unwinds the stack out of a function body.
type returnSignal struct{ v Value }

func (returnSignal) Error() string { return "return" }

// breakSignal and continueSignal unwind to the innermost loop.
type breakSignal struct{}

func (breakSignal) Error() string { return "break" }

type continueSignal struct{}

func (continueSignal) Error() string { return "continue" }

func (in *interp) execMatch(st *ast.MatchStmt, env *Env) error {
	v, err := in.eval(st.Scrutinee, env)
	if err != nil {
		return err
	}
	e, ok := read(v).(*Enum)
	if !ok {
		return &RuntimeError{Msg: "match scrutinee is not an enum", Line: st.Line, Col: st.Col}
	}
	for _, arm := range st.Arms {
		if !arm.Pattern.IsWild {
			if arm.Pattern.Enum != e.Type || arm.Pattern.Variant != e.Variant {
				continue
			}
		}
		armEnv := newEnv(env)
		for i, name := range arm.Pattern.Binds {
			var fv Value
			if i < len(e.Fields) {
				fv = e.Fields[i]
			}
			armEnv.declare(name, fv)
		}
		return in.execStmt(arm.Body, armEnv)
	}
	return &RuntimeError{Msg: "no match arm handled " + e.Type + "." + e.Variant, Line: st.Line, Col: st.Col}
}

// ---------- functions ----------

func (in *interp) callFunc(fd *ast.FuncDecl, argv []Value) (Value, error) {
	if fd.Extern {
		return nil, &RuntimeError{Msg: "cannot interpret extern function " + fd.Name, Line: fd.Line, Col: fd.Col}
	}
	env := newEnv(in.globals)
	for i, p := range fd.Params {
		var v Value
		if i < len(argv) {
			v = argv[i]
		} else {
			v = zeroValue(p.Type)
		}
		env.declare(p.Name, v)
	}
	err := in.execStmts(fd.Body.Stmts, env)
	if err != nil {
		if r, ok := err.(returnSignal); ok {
			return r.v, nil
		}
		return nil, err
	}
	return zeroValue(fd.ReturnType), nil
}

// ---------- expressions ----------

func (in *interp) eval(e ast.Expr, env *Env) (Value, error) {
	line, col := e.Pos()
	if err := in.tick(line, col); err != nil {
		return nil, err
	}
	switch ex := e.(type) {
	case *ast.IntLit:
		return ex.Value, nil
	case *ast.FloatLit:
		return ex.Value, nil
	case *ast.StringLit:
		return ex.Value, nil
	case *ast.BoolLit:
		return ex.Value, nil
	case *ast.Ident:
		if slot, ok := env.lookup(ex.Name); ok {
			return *slot, nil
		}
		if ci, ok := in.ck.Consts[ex.Name]; ok {
			return in.eval(ci.Lit, env)
		}
		return nil, &RuntimeError{Msg: "undefined name " + ex.Name, Line: line, Col: col}
	case *ast.UnaryExpr:
		return in.evalUnary(ex, env)
	case *ast.BinaryExpr:
		return in.evalBinary(ex, env)
	case *ast.FieldExpr:
		return in.evalField(ex, env)
	case *ast.IndexExpr:
		return in.evalIndex(ex, env)
	case *ast.StructLit:
		return in.evalStructLit(ex, env)
	case *ast.CallExpr:
		return in.evalCall(ex, env)
	}
	return nil, &RuntimeError{Msg: "unsupported expression", Line: line, Col: col}
}

func (in *interp) evalUnary(ex *ast.UnaryExpr, env *Env) (Value, error) {
	switch ex.Op {
	case "&", "&mut":
		slot, err := in.lvalue(ex.Operand, env)
		if err != nil {
			return nil, err
		}
		return &Ref{Slot: slot}, nil
	case "-":
		v, err := in.eval(ex.Operand, env)
		if err != nil {
			return nil, err
		}
		switch n := read(v).(type) {
		case int64:
			return -n, nil
		case float64:
			return -n, nil
		}
	case "!":
		v, err := in.eval(ex.Operand, env)
		if err != nil {
			return nil, err
		}
		return !read(v).(bool), nil
	}
	return nil, &RuntimeError{Msg: "unsupported unary operator " + ex.Op, Line: ex.Line, Col: ex.Col}
}

func (in *interp) evalBinary(ex *ast.BinaryExpr, env *Env) (Value, error) {
	// Short-circuit the logical operators.
	if ex.Op == "&&" || ex.Op == "||" {
		l, err := in.eval(ex.Left, env)
		if err != nil {
			return nil, err
		}
		lb := read(l).(bool)
		if ex.Op == "&&" && !lb {
			return false, nil
		}
		if ex.Op == "||" && lb {
			return true, nil
		}
		r, err := in.eval(ex.Right, env)
		if err != nil {
			return nil, err
		}
		return read(r).(bool), nil
	}

	lv, err := in.eval(ex.Left, env)
	if err != nil {
		return nil, err
	}
	rv, err := in.eval(ex.Right, env)
	if err != nil {
		return nil, err
	}
	l := read(lv)
	r := read(rv)

	switch ex.Op {
	case "+":
		switch a := l.(type) {
		case int64:
			return a + r.(int64), nil
		case float64:
			return a + r.(float64), nil
		case string:
			return a + r.(string), nil
		}
	case "-":
		if a, ok := l.(int64); ok {
			return a - r.(int64), nil
		}
		return l.(float64) - r.(float64), nil
	case "*":
		if a, ok := l.(int64); ok {
			return a * r.(int64), nil
		}
		return l.(float64) * r.(float64), nil
	case "/":
		if a, ok := l.(int64); ok {
			b := r.(int64)
			if b == 0 {
				return nil, &RuntimeError{Msg: "division by zero", Line: ex.Line, Col: ex.Col}
			}
			return a / b, nil
		}
		return l.(float64) / r.(float64), nil
	case "%":
		b := r.(int64)
		if b == 0 {
			return nil, &RuntimeError{Msg: "division by zero", Line: ex.Line, Col: ex.Col}
		}
		return l.(int64) % b, nil
	case "==":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	case "<", "<=", ">", ">=":
		return cmp(ex.Op, l, r), nil
	}
	return nil, &RuntimeError{Msg: "unsupported binary operator " + ex.Op, Line: ex.Line, Col: ex.Col}
}

func equal(a, b Value) bool {
	switch x := a.(type) {
	case int64:
		return x == b.(int64)
	case float64:
		return x == b.(float64)
	case bool:
		return x == b.(bool)
	case string:
		return x == b.(string)
	case *Bytes:
		y, ok := b.(*Bytes)
		return ok && stdbytes.Equal(x.Data, y.Data)
	}
	return false
}

func cmp(op string, a, b Value) bool {
	switch x := a.(type) {
	case int64:
		y := b.(int64)
		return applyCmp(op, x < y, x > y, x == y)
	case float64:
		y := b.(float64)
		return applyCmp(op, x < y, x > y, x == y)
	case string:
		y := b.(string)
		return applyCmp(op, x < y, x > y, x == y)
	}
	return false
}

func applyCmp(op string, lt, gt, eq bool) bool {
	switch op {
	case "<":
		return lt
	case "<=":
		return lt || eq
	case ">":
		return gt
	case ">=":
		return gt || eq
	}
	return false
}

func (in *interp) evalField(ex *ast.FieldExpr, env *Env) (Value, error) {
	// Enum variant as a value: Color.Red.
	if id, ok := ex.Base.(*ast.Ident); ok {
		if _, isEnum := in.ck.Enums[id.Name]; isEnum {
			return &Enum{Type: id.Name, Variant: ex.Field}, nil
		}
	}
	base, err := in.eval(ex.Base, env)
	if err != nil {
		return nil, err
	}
	switch v := read(base).(type) {
	case *Struct:
		fv, ok := v.Fields[ex.Field]
		if !ok {
			return nil, &RuntimeError{Msg: "no field " + ex.Field + " on " + v.Type, Line: ex.Line, Col: ex.Col}
		}
		return *fv, nil
	case string:
		if ex.Field == "len" {
			return int64(len(v)), nil
		}
	case *List:
		if ex.Field == "len" {
			return int64(len(v.Items)), nil
		}
	case *Map:
		if ex.Field == "len" {
			return int64(len(v.Data)), nil
		}
	case *Builder:
		if ex.Field == "len" {
			return int64(len(v.Buf)), nil
		}
	case *Bytes:
		if ex.Field == "len" {
			return int64(len(v.Data)), nil
		}
	}
	return nil, &RuntimeError{Msg: "no field " + ex.Field, Line: ex.Line, Col: ex.Col}
}

func (in *interp) evalIndex(ex *ast.IndexExpr, env *Env) (Value, error) {
	base, err := in.eval(ex.Base, env)
	if err != nil {
		return nil, err
	}
	iv, err := in.eval(ex.Index, env)
	if err != nil {
		return nil, err
	}
	i := read(iv).(int64)
	switch v := read(base).(type) {
	case *List:
		if i < 0 || int(i) >= len(v.Items) {
			return nil, &RuntimeError{Msg: "list index out of range", Line: ex.Line, Col: ex.Col}
		}
		return v.Items[i], nil
	case string:
		if i < 0 || int(i) >= len(v) {
			return nil, &RuntimeError{Msg: "text index out of range", Line: ex.Line, Col: ex.Col}
		}
		return int64(v[i]), nil
	case *Bytes:
		if i < 0 || int(i) >= len(v.Data) {
			return nil, &RuntimeError{Msg: "bytes index out of range", Line: ex.Line, Col: ex.Col}
		}
		return int64(v.Data[i]), nil
	}
	return nil, &RuntimeError{Msg: "cannot index this value", Line: ex.Line, Col: ex.Col}
}

func (in *interp) evalStructLit(ex *ast.StructLit, env *Env) (Value, error) {
	s := &Struct{Type: ex.Name, Fields: map[string]*Value{}}
	for _, f := range ex.Fields {
		v, err := in.eval(f.Value, env)
		if err != nil {
			return nil, err
		}
		slot := new(Value)
		*slot = deepCopy(read(v))
		s.Fields[f.Name] = slot
	}
	return s, nil
}

func (in *interp) evalCall(ex *ast.CallExpr, env *Env) (Value, error) {
	// Builtins.
	if id, ok := ex.Callee.(*ast.Ident); ok {
		switch id.Name {
		case "print":
			return nil, in.print(in.out, ex.Args[0], env)
		case "print_bytes":
			v, err := in.eval(ex.Args[0], env)
			if err != nil {
				return nil, err
			}
			b, ok := read(v).(*Bytes)
			if !ok {
				return nil, &RuntimeError{Msg: "print_bytes expects bytes", Line: ex.Line, Col: ex.Col}
			}
			if _, err := in.out.Write(b.Data); err != nil {
				return nil, &RuntimeError{Msg: err.Error(), Line: ex.Line, Col: ex.Col}
			}
			return nil, nil
		case "eprint":
			return nil, in.print(in.errw, ex.Args[0], env)
		case "read_file":
			p, err := in.textArg(ex.Args[0], env)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, &RuntimeError{Msg: "cannot read " + p, Line: ex.Line, Col: ex.Col}
			}
			return string(data), nil
		case "write_file":
			p, err := in.textArg(ex.Args[0], env)
			if err != nil {
				return nil, err
			}
			c, err := in.textArg(ex.Args[1], env)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
				return nil, &RuntimeError{Msg: "cannot write " + p, Line: ex.Line, Col: ex.Col}
			}
			return nil, nil
		case "slice":
			s, err := in.textArg(ex.Args[0], env)
			if err != nil {
				return nil, err
			}
			start, err := in.intArg(ex.Args[1], env)
			if err != nil {
				return nil, err
			}
			end, err := in.intArg(ex.Args[2], env)
			if err != nil {
				return nil, err
			}
			if start < 0 || end < start || end > int64(len(s)) {
				return nil, &RuntimeError{Msg: "slice out of range", Line: ex.Line, Col: ex.Col}
			}
			return s[start:end], nil
		case "args":
			items := make([]Value, len(in.args))
			for i, a := range in.args {
				items[i] = a
			}
			return &List{Elem: ast.Type{Name: "text"}, Items: items}, nil
		case "List":
			return &List{}, nil
		case "Map":
			return &Map{Data: map[string]Value{}}, nil
		case "Builder":
			return &Builder{}, nil
		case "bytes":
			xs, err := in.eval(ex.Args[0], env)
			if err != nil {
				return nil, err
			}
			l, ok := read(xs).(*List)
			if !ok {
				return nil, &RuntimeError{Msg: "bytes(...) expects a List(int)", Line: ex.Line, Col: ex.Col}
			}
			data := make([]byte, len(l.Items))
			for i, it := range l.Items {
				data[i] = byte(read(it).(int64) & 0xff)
			}
			return &Bytes{Data: data}, nil
		case "to_bytes":
			s, err := in.textArg(ex.Args[0], env)
			if err != nil {
				return nil, err
			}
			return &Bytes{Data: []byte(s)}, nil
		case "to_text":
			v, err := in.eval(ex.Args[0], env)
			if err != nil {
				return nil, err
			}
			switch x := read(v).(type) {
			case int64:
				return strconv.FormatInt(x, 10), nil
			case float64:
				return formatFloat(x), nil
			case bool:
				if x {
					return "true", nil
				}
				return "false", nil
			case *Bytes:
				return string(x.Data), nil
			}
			return nil, &RuntimeError{Msg: "to_text: unsupported type"}
		}
	}

	// Enum variant constructor: Shape.Circle(1.0).
	if fe, ok := ex.Callee.(*ast.FieldExpr); ok {
		if id, ok := fe.Base.(*ast.Ident); ok {
			if _, isEnum := in.ck.Enums[id.Name]; isEnum {
				fields := make([]Value, len(ex.Args))
				for i, a := range ex.Args {
					v, err := in.eval(a, env)
					if err != nil {
						return nil, err
					}
					fields[i] = deepCopy(read(v))
				}
				return &Enum{Type: id.Name, Variant: fe.Field, Fields: fields}, nil
			}
		}
		// Method call: xs.push(v), or m.set/get/has(k).
		if id, ok := fe.Base.(*ast.Ident); ok {
			if slot, found := env.lookup(id.Name); found {
				if l, ok := read(*slot).(*List); ok && fe.Field == "push" {
					v, err := in.eval(ex.Args[0], env)
					if err != nil {
						return nil, err
					}
					l.Items = append(l.Items, deepCopy(read(v)))
					return nil, nil
				}
				if m, ok := read(*slot).(*Map); ok {
					if mv, err := in.mapMethod(m, fe.Field, ex, env); err != errNotMapMethod {
						return mv, err
					}
				}
				if b, ok := read(*slot).(*Builder); ok {
					if bv, err := in.builderMethod(b, fe.Field, ex, env); err != errNotMapMethod {
						return bv, err
					}
				}
			}
		}
	}

	// User function.
	id, ok := ex.Callee.(*ast.Ident)
	if !ok {
		return nil, &RuntimeError{Msg: "unsupported call", Line: ex.Line, Col: ex.Col}
	}
	fd, ok := in.funcs[id.Name]
	if !ok {
		return nil, &RuntimeError{Msg: "call to undefined function " + id.Name, Line: ex.Line, Col: ex.Col}
	}
	argv := make([]Value, len(ex.Args))
	for i, a := range ex.Args {
		v, err := in.eval(a, env)
		if err != nil {
			return nil, err
		}
		// Reference parameters receive the borrow; value parameters a copy.
		if i < len(fd.Params) && fd.Params[i].Type.Ptr {
			argv[i] = v
		} else {
			argv[i] = deepCopy(read(v))
		}
	}
	return in.callFunc(fd, argv)
}

// errNotMapMethod signals that a field name is not a map method, so the caller
// can fall through to other call handling.
var errNotMapMethod = errors.New("not a map method")

func (in *interp) mapMethod(m *Map, field string, ex *ast.CallExpr, env *Env) (Value, error) {
	switch field {
	case "set":
		k, err := in.textArg(ex.Args[0], env)
		if err != nil {
			return nil, err
		}
		v, err := in.eval(ex.Args[1], env)
		if err != nil {
			return nil, err
		}
		m.Data[k] = deepCopy(read(v))
		return nil, nil
	case "get":
		k, err := in.textArg(ex.Args[0], env)
		if err != nil {
			return nil, err
		}
		v, ok := m.Data[k]
		if !ok {
			return nil, &RuntimeError{Msg: "missing map key: " + k, Line: ex.Line, Col: ex.Col}
		}
		return v, nil
	case "has":
		k, err := in.textArg(ex.Args[0], env)
		if err != nil {
			return nil, err
		}
		_, ok := m.Data[k]
		return ok, nil
	}
	return nil, errNotMapMethod
}

func (in *interp) builderMethod(b *Builder, field string, ex *ast.CallExpr, env *Env) (Value, error) {
	switch field {
	case "add":
		s, err := in.textArg(ex.Args[0], env)
		if err != nil {
			return nil, err
		}
		b.Buf = append(b.Buf, s...)
		return nil, nil
	case "build":
		return string(b.Buf), nil
	}
	return nil, errNotMapMethod
}

func (in *interp) textArg(e ast.Expr, env *Env) (string, error) {
	v, err := in.eval(e, env)
	if err != nil {
		return "", err
	}
	s, ok := read(v).(string)
	if !ok {
		line, col := e.Pos()
		return "", &RuntimeError{Msg: "expected text", Line: line, Col: col}
	}
	return s, nil
}

func (in *interp) intArg(e ast.Expr, env *Env) (int64, error) {
	v, err := in.eval(e, env)
	if err != nil {
		return 0, err
	}
	n, ok := read(v).(int64)
	if !ok {
		line, col := e.Pos()
		return 0, &RuntimeError{Msg: "expected int", Line: line, Col: col}
	}
	return n, nil
}

// ---------- lvalues ----------

// lvalue returns the slot an assignment target refers to.
func (in *interp) lvalue(e ast.Expr, env *Env) (*Value, error) {
	switch ex := e.(type) {
	case *ast.Ident:
		slot, ok := env.lookup(ex.Name)
		if !ok {
			return nil, &RuntimeError{Msg: "undefined name " + ex.Name, Line: ex.Line, Col: ex.Col}
		}
		return slot, nil
	case *ast.FieldExpr:
		base, err := in.eval(ex.Base, env)
		if err != nil {
			return nil, err
		}
		if s, ok := read(base).(*Struct); ok {
			slot, ok := s.Fields[ex.Field]
			if !ok {
				slot = new(Value)
				*slot = zeroAny()
				s.Fields[ex.Field] = slot
			}
			return slot, nil
		}
	case *ast.IndexExpr:
		base, err := in.eval(ex.Base, env)
		if err != nil {
			return nil, err
		}
		iv, err := in.eval(ex.Index, env)
		if err != nil {
			return nil, err
		}
		i := read(iv).(int64)
		if l, ok := read(base).(*List); ok {
			if i < 0 || int(i) >= len(l.Items) {
				return nil, &RuntimeError{Msg: "list index out of range", Line: ex.Line, Col: ex.Col}
			}
			return &l.Items[i], nil
		}
	}
	return nil, &RuntimeError{Msg: "invalid assignment target"}
}

// ---------- helpers ----------

// read dereferences a borrow, if any.
func read(v Value) Value {
	for {
		r, ok := v.(*Ref)
		if !ok {
			return v
		}
		v = *r.Slot
	}
}

func deepCopy(v Value) Value {
	switch x := v.(type) {
	case *Struct:
		fields := make(map[string]*Value, len(x.Fields))
		for k, fv := range x.Fields {
			slot := new(Value)
			*slot = deepCopy(read(*fv))
			fields[k] = slot
		}
		return &Struct{Type: x.Type, Fields: fields}
	case *List:
		items := make([]Value, len(x.Items))
		for i, it := range x.Items {
			items[i] = deepCopy(it)
		}
		return &List{Elem: x.Elem, Items: items}
	case *Enum:
		fields := make([]Value, len(x.Fields))
		for i, fv := range x.Fields {
			fields[i] = deepCopy(fv)
		}
		return &Enum{Type: x.Type, Variant: x.Variant, Fields: fields}
	case *Map:
		data := make(map[string]Value, len(x.Data))
		for k, v := range x.Data {
			data[k] = deepCopy(v)
		}
		return &Map{Key: x.Key, Elem: x.Elem, Data: data}
	case *Builder:
		buf := make([]byte, len(x.Buf))
		copy(buf, x.Buf)
		return &Builder{Buf: buf}
	case *Bytes:
		data := make([]byte, len(x.Data))
		copy(data, x.Data)
		return &Bytes{Data: data}
	}
	return v
}

func zeroValue(t ast.Type) Value {
	switch t.Name {
	case "int":
		return int64(0)
	case "float":
		return float64(0)
	case "bool":
		return false
	case "text":
		return ""
	case "List":
		return &List{Elem: derefElem(t)}
	case "Map":
		return &Map{Data: map[string]Value{}}
	case "Builder":
		return &Builder{}
	case "bytes":
		return &Bytes{}
	}
	return zeroAny()
}

func zeroAny() Value { return int64(0) }

func derefElem(t ast.Type) ast.Type {
	if t.Elem != nil {
		return *t.Elem
	}
	return ast.Type{Name: "int"}
}

// print mirrors the C backend's formatting so differential tests can compare
// byte-for-byte.
func (in *interp) print(w io.Writer, arg ast.Expr, env *Env) error {
	v, err := in.eval(arg, env)
	if err != nil {
		return err
	}
	switch x := read(v).(type) {
	case int64:
		fmt.Fprintf(w, "%d\n", x)
	case float64:
		fmt.Fprintf(w, "%s\n", formatFloat(x))
	case bool:
		if x {
			fmt.Fprintln(w, "true")
		} else {
			fmt.Fprintln(w, "false")
		}
	case string:
		fmt.Fprintf(w, "%s\n", x)
	default:
		return &RuntimeError{Msg: "cannot print this type"}
	}
	return nil
}

// formatFloat matches C's "%g" closely enough for the values our tests use.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
