// Package parser implements a hand-written recursive-descent parser for Brick.
package parser

import (
	"fmt"
	"strconv"

	"brick/internal/ast"
	"brick/internal/lexer"
	"brick/internal/token"
)

// ParseError is a syntax error with a source position.
type ParseError struct {
	Msg  string
	Line int
	Col  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg)
}

// Parser holds the token stream and cursor.
type Parser struct {
	toks        []token.Token
	pos         int
	noStructLit int // >0 disables struct-literal parsing (if/while conditions)
}

// Parse turns source text into a Program, or returns a *ParseError.
func Parse(src string) (prog *ast.Program, err error) {
	p := &Parser{toks: lexer.New(src).Tokens()}
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(*ParseError); ok {
				err = pe
				return
			}
			panic(r)
		}
	}()
	prog = p.parseProgram()
	return prog, nil
}

// ---------- cursor helpers ----------

func (p *Parser) cur() token.Token { return p.toks[p.pos] }

func (p *Parser) peek() token.Token {
	if p.pos+1 < len(p.toks) {
		return p.toks[p.pos+1]
	}
	return p.toks[len(p.toks)-1]
}

func (p *Parser) advance() token.Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) check(t token.Type) bool { return p.cur().Type == t }

func (p *Parser) match(t token.Type) bool {
	if p.check(t) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(t token.Type) token.Token {
	if !p.check(t) {
		p.errorf(p.cur(), "expected %q, found %q", t.String(), describe(p.cur()))
	}
	return p.advance()
}

func (p *Parser) errorf(tok token.Token, format string, args ...any) {
	panic(&ParseError{Msg: fmt.Sprintf(format, args...), Line: tok.Line, Col: tok.Col})
}

func describe(t token.Token) string {
	switch t.Type {
	case token.EOF:
		return "end of file"
	case token.IDENT, token.INT, token.FLOAT, token.STRING:
		return t.Literal
	default:
		return t.Type.String()
	}
}

// ---------- program & statements ----------

func (p *Parser) parseProgram() *ast.Program {
	prog := &ast.Program{}
	for !p.check(token.EOF) {
		p.match(token.SEMICOLON) // tolerate stray semicolons
		if p.check(token.EOF) {
			break
		}
		prog.Stmts = append(prog.Stmts, p.parseStmt())
	}
	return prog
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.cur().Type {
	case token.FUNCTION:
		return p.parseFuncDecl(false, token.Token{})
	case token.EXTERN:
		ext := p.advance()
		return p.parseFuncDecl(true, ext)
	case token.STRUCT:
		return p.parseStructDecl()
	case token.ENUM:
		return p.parseEnumDecl()
	case token.MATCH:
		return p.parseMatch()
	case token.IMPORT:
		return p.parseImport()
	case token.VAR:
		return p.parseVarStmt()
	case token.IF:
		return p.parseIf()
	case token.WHILE:
		return p.parseWhile()
	case token.RETURN:
		return p.parseReturn()
	case token.LBRACE:
		return p.parseBlock()
	default:
		return p.parseSimpleStmt()
	}
}

func (p *Parser) parseBlock() *ast.BlockStmt {
	lb := p.expect(token.LBRACE)
	b := &ast.BlockStmt{}
	ast.SetBase(b, lb)
	for !p.check(token.RBRACE) && !p.check(token.EOF) {
		p.match(token.SEMICOLON)
		if p.check(token.RBRACE) {
			break
		}
		b.Stmts = append(b.Stmts, p.parseStmt())
	}
	p.expect(token.RBRACE)
	return b
}

func (p *Parser) parseVarStmt() ast.Stmt {
	v := p.advance() // var
	name := p.expect(token.IDENT)
	s := &ast.VarStmt{Name: name.Literal}
	ast.SetBase(s, v)
	if p.match(token.COLON) {
		t := p.parseType()
		s.Type = &t
	}
	p.expect(token.ASSIGN)
	s.Value = p.parseExpr()
	return s
}

func (p *Parser) parseReturn() ast.Stmt {
	r := p.advance()
	s := &ast.ReturnStmt{}
	ast.SetBase(s, r)
	if !p.check(token.RBRACE) && !p.check(token.EOF) {
		s.Value = p.parseExpr()
	}
	return s
}

func (p *Parser) parseIf() ast.Stmt {
	ifTok := p.advance()
	s := &ast.IfStmt{}
	ast.SetBase(s, ifTok)
	p.noStructLit++
	s.Cond = p.parseExpr()
	p.noStructLit--
	s.Then = p.parseBlock()
	if p.match(token.ELSE) {
		if p.check(token.IF) {
			s.Else = p.parseIf()
		} else {
			s.Else = p.parseBlock()
		}
	}
	return s
}

func (p *Parser) parseWhile() ast.Stmt {
	w := p.advance()
	s := &ast.WhileStmt{}
	ast.SetBase(s, w)
	p.noStructLit++
	s.Cond = p.parseExpr()
	p.noStructLit--
	s.Body = p.parseBlock()
	return s
}

func (p *Parser) parseSimpleStmt() ast.Stmt {
	start := p.cur()
	expr := p.parseExpr()
	if p.check(token.ASSIGN) {
		p.advance()
		value := p.parseExpr()
		s := &ast.AssignStmt{Target: expr, Value: value}
		ast.SetBase(s, start)
		return s
	}
	s := &ast.ExprStmt{X: expr}
	ast.SetBase(s, start)
	return s
}

func (p *Parser) parseFuncDecl(extern bool, extTok token.Token) ast.Stmt {
	fnTok := p.expect(token.FUNCTION)
	if extern {
		fnTok = extTok
	}
	name := p.expect(token.IDENT)
	decl := &ast.FuncDecl{Name: name.Literal, Extern: extern}
	ast.SetBase(decl, fnTok)
	p.expect(token.LPAREN)
	for !p.check(token.RPAREN) {
		pn := p.expect(token.IDENT)
		p.expect(token.COLON)
		pt := p.parseType()
		decl.Params = append(decl.Params, ast.Param{Name: pn.Literal, Type: pt})
		if !p.match(token.COMMA) {
			break
		}
	}
	p.expect(token.RPAREN)
	if p.match(token.ARROW) {
		decl.ReturnType = p.parseType()
	} else {
		decl.ReturnType = ast.Type{Name: "void"}
	}
	if !extern {
		decl.Body = p.parseBlock()
	}
	return decl
}

func (p *Parser) parseStructDecl() ast.Stmt {
	sTok := p.advance()
	name := p.expect(token.IDENT)
	decl := &ast.StructDecl{Name: name.Literal}
	ast.SetBase(decl, sTok)
	p.expect(token.LBRACE)
	for !p.check(token.RBRACE) {
		fn := p.expect(token.IDENT)
		p.expect(token.COLON)
		ft := p.parseType()
		decl.Fields = append(decl.Fields, ast.Field{Name: fn.Literal, Type: ft})
		if !p.match(token.COMMA) {
			break
		}
	}
	p.expect(token.RBRACE)
	return decl
}

func (p *Parser) parseImport() ast.Stmt {
	iTok := p.advance()
	s := &ast.ImportStmt{}
	ast.SetBase(s, iTok)
	path := p.expect(token.STRING)
	s.Path = path.Literal
	return s
}

func (p *Parser) parseEnumDecl() ast.Stmt {
	eTok := p.advance() // enum
	name := p.expect(token.IDENT)
	decl := &ast.EnumDecl{Name: name.Literal}
	ast.SetBase(decl, eTok)
	p.expect(token.LBRACE)
	for !p.check(token.RBRACE) {
		vn := p.expect(token.IDENT)
		v := ast.Variant{Name: vn.Literal}
		if p.match(token.LPAREN) {
			for !p.check(token.RPAREN) {
				v.Fields = append(v.Fields, p.parseType())
				if !p.match(token.COMMA) {
					break
				}
			}
			p.expect(token.RPAREN)
		}
		decl.Variants = append(decl.Variants, v)
		if !p.match(token.COMMA) {
			break
		}
	}
	p.expect(token.RBRACE)
	return decl
}

func (p *Parser) parseMatch() ast.Stmt {
	mTok := p.advance() // match
	s := &ast.MatchStmt{}
	ast.SetBase(s, mTok)
	p.noStructLit++
	s.Scrutinee = p.parseExpr()
	p.noStructLit--
	p.expect(token.LBRACE)
	for !p.check(token.RBRACE) && !p.check(token.EOF) {
		p.match(token.SEMICOLON)
		if p.check(token.RBRACE) {
			break
		}
		pat := p.parsePattern()
		p.expect(token.FATARROW)
		var body ast.Stmt
		if p.check(token.LBRACE) {
			body = p.parseBlock()
		} else {
			body = p.parseStmt()
		}
		s.Arms = append(s.Arms, ast.MatchArm{Pattern: pat, Body: body})
		p.match(token.COMMA)
	}
	p.expect(token.RBRACE)
	return s
}

func (p *Parser) parsePattern() ast.Pattern {
	tok := p.cur()
	if tok.Type == token.IDENT && tok.Literal == "_" {
		p.advance()
		return ast.Pattern{IsWild: true, Line: tok.Line, Col: tok.Col}
	}
	enum := p.expect(token.IDENT)
	p.expect(token.DOT)
	variant := p.expect(token.IDENT)
	pat := ast.Pattern{Enum: enum.Literal, Variant: variant.Literal, Line: tok.Line, Col: tok.Col}
	if p.match(token.LPAREN) {
		for !p.check(token.RPAREN) {
			b := p.expect(token.IDENT)
			pat.Binds = append(pat.Binds, b.Literal)
			if !p.match(token.COMMA) {
				break
			}
		}
		p.expect(token.RPAREN)
	}
	return pat
}

func (p *Parser) parseType() ast.Type {
	ptr := false
	if p.check(token.AMP) || p.check(token.AMPMUT) {
		ptr = true
		p.advance()
	}
	nameTok := p.expect(token.IDENT)
	t := ast.Type{Name: nameTok.Literal, Ptr: ptr}
	if nameTok.Literal == "List" && p.check(token.LPAREN) {
		p.advance()
		elem := p.parseType()
		p.expect(token.RPAREN)
		t.Elem = &elem
	}
	return t
}

// ---------- expressions ----------

var prec = map[token.Type]int{
	token.OR:  1,
	token.AND: 2,
	token.EQ:  3, token.NEQ: 3,
	token.LT: 4, token.LE: 4, token.GT: 4, token.GE: 4,
	token.PLUS: 5, token.MINUS: 5,
	token.STAR: 6, token.SLASH: 6, token.PERCENT: 6,
}

func (p *Parser) parseExpr() ast.Expr { return p.parseBinary(1) }

func (p *Parser) parseBinary(minPrec int) ast.Expr {
	left := p.parseUnary()
	for {
		op := p.cur()
		pr, ok := prec[op.Type]
		if !ok || pr < minPrec {
			break
		}
		p.advance()
		right := p.parseBinary(pr + 1)
		e := &ast.BinaryExpr{Op: op.Literal, Left: left, Right: right}
		ast.SetBase(e, op)
		left = e
	}
	return left
}

func (p *Parser) parseUnary() ast.Expr {
	switch p.cur().Type {
	case token.MINUS, token.NOT, token.AMP, token.AMPMUT:
		op := p.advance()
		operand := p.parseUnary()
		e := &ast.UnaryExpr{Op: op.Literal, Operand: operand}
		ast.SetBase(e, op)
		return e
	default:
		return p.parsePostfix()
	}
}

func (p *Parser) parsePostfix() ast.Expr {
	e := p.parsePrimary()
	for {
		switch p.cur().Type {
		case token.LPAREN:
			lp := p.advance()
			call := &ast.CallExpr{Callee: e}
			ast.SetBase(call, lp)
			for !p.check(token.RPAREN) {
				call.Args = append(call.Args, p.parseExpr())
				if !p.match(token.COMMA) {
					break
				}
			}
			p.expect(token.RPAREN)
			e = call
		case token.DOT:
			dot := p.advance()
			name := p.expect(token.IDENT)
			fe := &ast.FieldExpr{Base: e, Field: name.Literal}
			ast.SetBase(fe, dot)
			e = fe
		case token.LBRACKET:
			lb := p.advance()
			idx := p.parseExpr()
			p.expect(token.RBRACKET)
			ie := &ast.IndexExpr{Base: e, Index: idx}
			ast.SetBase(ie, lb)
			e = ie
		default:
			return e
		}
	}
}

func (p *Parser) parsePrimary() ast.Expr {
	tok := p.cur()
	switch tok.Type {
	case token.INT:
		p.advance()
		v, err := strconv.ParseInt(tok.Literal, 10, 64)
		if err != nil {
			p.errorf(tok, "invalid integer %q", tok.Literal)
		}
		e := &ast.IntLit{Value: v}
		ast.SetBase(e, tok)
		return e
	case token.FLOAT:
		p.advance()
		v, err := strconv.ParseFloat(tok.Literal, 64)
		if err != nil {
			p.errorf(tok, "invalid float %q", tok.Literal)
		}
		e := &ast.FloatLit{Value: v, Text: tok.Literal}
		ast.SetBase(e, tok)
		return e
	case token.STRING:
		p.advance()
		e := &ast.StringLit{Value: tok.Literal}
		ast.SetBase(e, tok)
		return e
	case token.TRUE, token.FALSE:
		p.advance()
		e := &ast.BoolLit{Value: tok.Type == token.TRUE}
		ast.SetBase(e, tok)
		return e
	case token.LPAREN:
		p.advance()
		e := p.parseExpr()
		p.expect(token.RPAREN)
		return e
	case token.IDENT:
		p.advance()
		if p.check(token.LBRACE) && p.noStructLit == 0 {
			return p.parseStructLit(tok)
		}
		e := &ast.Ident{Name: tok.Literal}
		ast.SetBase(e, tok)
		return e
	default:
		p.errorf(tok, "unexpected %q", describe(tok))
		return nil
	}
}

func (p *Parser) parseStructLit(nameTok token.Token) ast.Expr {
	lit := &ast.StructLit{Name: nameTok.Literal}
	ast.SetBase(lit, nameTok)
	p.expect(token.LBRACE)
	for !p.check(token.RBRACE) {
		fn := p.expect(token.IDENT)
		p.expect(token.COLON)
		v := p.parseExpr()
		lit.Fields = append(lit.Fields, ast.FieldInit{Name: fn.Literal, Value: v})
		if !p.match(token.COMMA) {
			break
		}
	}
	p.expect(token.RBRACE)
	return lit
}
