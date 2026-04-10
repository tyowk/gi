package parser

import (
	"fmt"
	"strconv"

	"github.com/tyowk/gi/ast"
	"github.com/tyowk/gi/lexer"
)

type Parser struct {
	tokens []lexer.Token
	pos    int
	errors []string
	cache  map[string]*ast.Program
}

func New(tokens []lexer.Token) *Parser {
	return &Parser{tokens: tokens, pos: 0, cache: make(map[string]*ast.Program)}
}

func (p *Parser) Errors() []string { return p.errors }

func (p *Parser) peek() lexer.Token {
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type == lexer.TOKEN_NEWLINE {
		p.pos++
	}
	if p.pos >= len(p.tokens) {
		return lexer.Token{Type: lexer.TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekRaw() lexer.Token {
	if p.pos >= len(p.tokens) {
		return lexer.Token{Type: lexer.TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() lexer.Token {
	tok := p.peek()
	p.pos++
	return tok
}

func (p *Parser) expect(t lexer.TokenType) (lexer.Token, bool) {
	tok := p.peek()
	if tok.Type != t {
		p.errors = append(p.errors, fmt.Sprintf("line %d:%d: expected %s, got %s (%q)", tok.Line, tok.Col, t, tok.Type, tok.Literal))
		return tok, false
	}
	p.advance()
	return tok, true
}

func (p *Parser) skipTerminators() {
	for p.pos < len(p.tokens) {
		t := p.tokens[p.pos].Type
		if t == lexer.TOKEN_NEWLINE || t == lexer.TOKEN_SEMICOLON {
			p.pos++
		} else {
			break
		}
	}
}

func (p *Parser) pos2ast(tok lexer.Token) ast.Pos {
	return ast.Pos{Line: tok.Line, Col: tok.Col}
}

func (p *Parser) Parse() *ast.Program {
	prog := &ast.Program{}
	p.skipTerminators()
	for p.peek().Type != lexer.TOKEN_EOF {
		tok := p.peek()
		switch tok.Type {
		case lexer.TOKEN_IMPORT, lexer.TOKEN_FUNC, lexer.TOKEN_STRUCT:
			stmt := p.parseStatement()
			if stmt != nil {
				prog.Statements = append(prog.Statements, stmt)
			}
		default:
			p.errors = append(p.errors, fmt.Sprintf(
				"line %d:%d: top-level code is not allowed outside of func — wrap it inside func main() {}",
				tok.Line, tok.Col,
			))
			for p.pos < len(p.tokens) {
				t := p.tokens[p.pos].Type
				if t == lexer.TOKEN_NEWLINE || t == lexer.TOKEN_SEMICOLON || t == lexer.TOKEN_EOF {
					break
				}
				p.pos++
			}
		}
		p.skipTerminators()
	}
	return prog
}

func (p *Parser) parseStatement() ast.Node {
	tok := p.peek()
	switch tok.Type {
	case lexer.TOKEN_IMPORT:
		return p.parseImport()
	case lexer.TOKEN_FUNC:
		return p.parseFuncDecl()
	case lexer.TOKEN_STRUCT:
		return p.parseStructDecl()
	case lexer.TOKEN_VAR:
		return p.parseVarDecl(false)
	case lexer.TOKEN_CONST:
		return p.parseVarDecl(true)
	case lexer.TOKEN_RETURN:
		return p.parseReturn()
	case lexer.TOKEN_IF:
		return p.parseIf()
	case lexer.TOKEN_FOR:
		return p.parseFor()
	case lexer.TOKEN_BREAK:
		pos := p.pos2ast(tok)
		p.advance()
		return &ast.BreakStmt{NodePos: pos}
	case lexer.TOKEN_CONTINUE:
		pos := p.pos2ast(tok)
		p.advance()
		return &ast.ContinueStmt{NodePos: pos}
	case lexer.TOKEN_LBRACE:
		return p.parseBlock()
	default:
		return p.parseExprOrAssign()
	}
}

func (p *Parser) parseImport() ast.Node {
	tok := p.advance()
	imp := &ast.ImportStmt{NodePos: p.pos2ast(tok)}

	p.expect(lexer.TOKEN_LBRACE)
	for p.peek().Type != lexer.TOKEN_RBRACE && p.peek().Type != lexer.TOKEN_EOF {
		t, ok := p.expect(lexer.TOKEN_IDENT)
		if ok {
			name := t.Literal
			alias := ""
			if p.peek().Type == lexer.TOKEN_AS {
				p.advance()
				asTok, _ := p.expect(lexer.TOKEN_IDENT)
				alias = asTok.Literal
			}
			imp.Names = append(imp.Names, name)
			imp.Aliases = append(imp.Aliases, alias)
		}
		if p.peek().Type == lexer.TOKEN_COMMA {
			p.advance()
		} else {
			break
		}
	}
	p.expect(lexer.TOKEN_RBRACE)
	p.expect(lexer.TOKEN_FROM)
	pathTok, _ := p.expect(lexer.TOKEN_STRING)
	imp.Path = pathTok.Literal
	return imp
}

func (p *Parser) parseFuncDecl() ast.Node {
	tok := p.advance()
	nameTok, _ := p.expect(lexer.TOKEN_IDENT)
	fd := &ast.FuncDecl{Name: nameTok.Literal, NodePos: p.pos2ast(tok)}
	fd.Params = p.parseParams()
	fd.Returns = p.parseReturnTypes()
	fd.Body = p.parseBlock()
	return fd
}

func (p *Parser) parseParams() []ast.Param {
	p.expect(lexer.TOKEN_LPAREN)
	var params []ast.Param

	for p.peek().Type != lexer.TOKEN_RPAREN && p.peek().Type != lexer.TOKEN_EOF {
		nameTok, _ := p.expect(lexer.TOKEN_IDENT)
		param := ast.Param{Name: nameTok.Literal}
		if p.peek().Type == lexer.TOKEN_IDENT {
			param.Type = p.advance().Literal
		}

		if p.peek().Type == lexer.TOKEN_QUESTION {
			p.advance()
			param.Required = false
		}

		if p.peek().Type == lexer.TOKEN_ASSIGN {
			p.advance()
			param.Default = p.parseExpr()
		}

		params = append(params, param)
		if p.peek().Type == lexer.TOKEN_COMMA {
			p.advance()
		} else {
			break
		}
	}

	p.expect(lexer.TOKEN_RPAREN)
	return params
}

func (p *Parser) parseReturnTypes() []string {
	var returns []string

	if p.peek().Type == lexer.TOKEN_LPAREN {
		p.advance()
		for p.peek().Type != lexer.TOKEN_RPAREN && p.peek().Type != lexer.TOKEN_EOF {
			returns = append(returns, p.advance().Literal)
			if p.peek().Type == lexer.TOKEN_COMMA {
				p.advance()
			} else {
				break
			}
		}
		p.expect(lexer.TOKEN_RPAREN)
	} else if p.peek().Type == lexer.TOKEN_IDENT {
		returns = append(returns, p.advance().Literal)
	}

	return returns
}

func (p *Parser) parseStructDecl() ast.Node {
	tok := p.advance()
	nameTok, _ := p.expect(lexer.TOKEN_IDENT)
	sd := &ast.StructDecl{Name: nameTok.Literal, NodePos: p.pos2ast(tok)}
	p.expect(lexer.TOKEN_LBRACE)
	p.skipTerminators()
	for p.peek().Type != lexer.TOKEN_RBRACE && p.peek().Type != lexer.TOKEN_EOF {
		fieldName, _ := p.expect(lexer.TOKEN_IDENT)
		fieldType := ""
		if p.peek().Type == lexer.TOKEN_IDENT {
			fieldType = p.advance().Literal
		}
		sd.Fields = append(sd.Fields, ast.StructField{Name: fieldName.Literal, Type: fieldType})
		p.skipTerminators()
	}
	p.expect(lexer.TOKEN_RBRACE)
	return sd
}

func (p *Parser) parseBlockStatement() ast.Node {
	tok := p.peek()
	if tok.Type == lexer.TOKEN_FUNC {
		nextPos := p.pos + 1
		for nextPos < len(p.tokens) && p.tokens[nextPos].Type == lexer.TOKEN_NEWLINE {
			nextPos++
		}
		if nextPos < len(p.tokens) && p.tokens[nextPos].Type == lexer.TOKEN_IDENT {
			p.errors = append(p.errors, fmt.Sprintf(
				"line %d:%d: func declaration is not allowed inside a function — use a func literal instead: var f = func() {}",
				tok.Line, tok.Col,
			))
			p.advance()
			depth := 0
			for p.pos < len(p.tokens) {
				t := p.tokens[p.pos].Type
				if t == lexer.TOKEN_LBRACE {
					depth++
				} else if t == lexer.TOKEN_RBRACE {
					if depth <= 1 {
						p.pos++
						break
					}
					depth--
				}
				p.pos++
			}
			return nil
		}
	}
	return p.parseStatement()
}

func (p *Parser) parseBlock() *ast.Block {
	tok := p.peek()
	p.expect(lexer.TOKEN_LBRACE)
	block := &ast.Block{NodePos: p.pos2ast(tok)}
	p.skipTerminators()
	for p.peek().Type != lexer.TOKEN_RBRACE && p.peek().Type != lexer.TOKEN_EOF {
		stmt := p.parseBlockStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.skipTerminators()
	}
	p.expect(lexer.TOKEN_RBRACE)
	return block
}

func (p *Parser) parseVarDecl(isConst bool) ast.Node {
	tok := p.advance()
	pos := p.pos2ast(tok)

	if p.peek().Type == lexer.TOKEN_LPAREN {
		p.advance()
		var names []string
		for p.peek().Type != lexer.TOKEN_RPAREN && p.peek().Type != lexer.TOKEN_EOF {
			n, _ := p.expect(lexer.TOKEN_IDENT)
			names = append(names, n.Literal)
			if p.peek().Type == lexer.TOKEN_COMMA {
				p.advance()
			}
		}
		p.expect(lexer.TOKEN_RPAREN)
		p.expect(lexer.TOKEN_ASSIGN)
		val := p.parseExpr()
		return &ast.MultiVarDecl{Names: names, Value: val, NodePos: pos}
	}

	nameTok, _ := p.expect(lexer.TOKEN_IDENT)
	if p.peek().Type == lexer.TOKEN_IDENT {
		p.advance()
	}
	p.expect(lexer.TOKEN_ASSIGN)
	val := p.parseExpr()
	return &ast.VarDecl{Name: nameTok.Literal, Value: val, IsConst: isConst, NodePos: pos}
}

func (p *Parser) parseReturn() ast.Node {
	tok := p.advance()
	pos := p.pos2ast(tok)
	raw := p.peekRaw()
	if raw.Type == lexer.TOKEN_NEWLINE || raw.Type == lexer.TOKEN_SEMICOLON || raw.Type == lexer.TOKEN_EOF || raw.Type == lexer.TOKEN_RBRACE {
		return &ast.ReturnStmt{NodePos: pos}
	}
	var vals []ast.Node
	vals = append(vals, p.parseExpr())
	for p.peek().Type == lexer.TOKEN_COMMA {
		p.advance()
		vals = append(vals, p.parseExpr())
	}
	return &ast.ReturnStmt{Values: vals, NodePos: pos}
}

func (p *Parser) parseIf() ast.Node {
	tok := p.advance()
	pos := p.pos2ast(tok)
	cond := p.parseExpr()
	then := p.parseBlock()
	stmt := &ast.IfStmt{Condition: cond, Then: then, NodePos: pos}
	p.skipTerminators()
	if p.peek().Type == lexer.TOKEN_ELSE {
		p.advance()
		if p.peek().Type == lexer.TOKEN_IF {
			stmt.Else = p.parseIf()
		} else {
			stmt.Else = p.parseBlock()
		}
	}
	return stmt
}

func (p *Parser) parseFor() ast.Node {
	tok := p.advance()
	pos := p.pos2ast(tok)
	stmt := &ast.ForStmt{NodePos: pos}

	if p.peek().Type == lexer.TOKEN_LBRACE {
		stmt.Body = p.parseBlock()
		return stmt
	}

	first := p.parseExprOrAssign()
	if p.peek().Type == lexer.TOKEN_SEMICOLON || (p.pos < len(p.tokens) && p.tokens[p.pos].Type == lexer.TOKEN_SEMICOLON) {
		p.skipTerminators()
		stmt.Init = first
		stmt.Condition = p.parseExpr()
		p.skipTerminators()
		stmt.Post = p.parseExprOrAssign()
	} else {
		stmt.Condition = exprFromNode(first)
	}
	stmt.Body = p.parseBlock()
	return stmt
}

func exprFromNode(n ast.Node) ast.Node {
	if es, ok := n.(*ast.ExprStmt); ok {
		return es.Expr
	}
	return n
}

func (p *Parser) parseExprOrAssign() ast.Node {
	tok := p.peek()
	pos := p.pos2ast(tok)
	expr := p.parseExpr()

	raw := p.peekRaw()
	if raw.Type == lexer.TOKEN_NEWLINE || raw.Type == lexer.TOKEN_SEMICOLON || raw.Type == lexer.TOKEN_EOF {
		return &ast.ExprStmt{Expr: expr, NodePos: pos}
	}

	switch p.peek().Type {
	case lexer.TOKEN_ASSIGN, lexer.TOKEN_PLUSEQ, lexer.TOKEN_MINUSEQ, lexer.TOKEN_STAREQ, lexer.TOKEN_SLASHEQ:
		op := p.advance().Literal
		val := p.parseExpr()
		return &ast.AssignStmt{Target: expr, Op: op, Value: val, NodePos: pos}
	case lexer.TOKEN_WALRUS:
		p.advance()
		val := p.parseExpr()
		if ident, ok := expr.(*ast.Identifier); ok {
			return &ast.VarDecl{Name: ident.Name, Value: val, Short: true, NodePos: pos}
		}
		if mv, ok := expr.(*ast.MultiVarDecl); ok {
			return &ast.MultiVarDecl{Names: mv.Names, Value: val, Short: true, NodePos: pos}
		}
		return &ast.VarDecl{Name: "?", Value: val, Short: true, NodePos: pos}
	}

	return &ast.ExprStmt{Expr: expr, NodePos: pos}
}

func (p *Parser) parseExpr() ast.Node {
	return p.parseOr()
}

func (p *Parser) parseOr() ast.Node {
	tok := p.peek()
	left := p.parseAnd()
	for p.peek().Type == lexer.TOKEN_OR {
		op := p.advance().Literal
		right := p.parseAnd()
		left = &ast.BinaryExpr{Left: left, Op: op, Right: right, NodePos: p.pos2ast(tok)}
	}
	return left
}

func (p *Parser) parseAnd() ast.Node {
	tok := p.peek()
	left := p.parseEquality()
	for p.peek().Type == lexer.TOKEN_AND {
		op := p.advance().Literal
		right := p.parseEquality()
		left = &ast.BinaryExpr{Left: left, Op: op, Right: right, NodePos: p.pos2ast(tok)}
	}
	return left
}

func (p *Parser) parseEquality() ast.Node {
	tok := p.peek()
	left := p.parseComparison()
	for p.peek().Type == lexer.TOKEN_EQ || p.peek().Type == lexer.TOKEN_NEQ {
		op := p.advance().Literal
		right := p.parseComparison()
		left = &ast.BinaryExpr{Left: left, Op: op, Right: right, NodePos: p.pos2ast(tok)}
	}
	return left
}

func (p *Parser) parseComparison() ast.Node {
	tok := p.peek()
	left := p.parseAddSub()
	for t := p.peek().Type; t == lexer.TOKEN_LT || t == lexer.TOKEN_GT || t == lexer.TOKEN_LTE || t == lexer.TOKEN_GTE; t = p.peek().Type {
		op := p.advance().Literal
		right := p.parseAddSub()
		left = &ast.BinaryExpr{Left: left, Op: op, Right: right, NodePos: p.pos2ast(tok)}
	}
	return left
}

func (p *Parser) parseAddSub() ast.Node {
	tok := p.peek()
	left := p.parseMulDiv()
	for t := p.peek().Type; t == lexer.TOKEN_PLUS || t == lexer.TOKEN_MINUS; t = p.peek().Type {
		op := p.advance().Literal
		right := p.parseMulDiv()
		left = &ast.BinaryExpr{Left: left, Op: op, Right: right, NodePos: p.pos2ast(tok)}
	}
	return left
}

func (p *Parser) parseMulDiv() ast.Node {
	tok := p.peek()
	left := p.parseUnary()
	for t := p.peek().Type; t == lexer.TOKEN_STAR || t == lexer.TOKEN_SLASH || t == lexer.TOKEN_PERCENT; t = p.peek().Type {
		op := p.advance().Literal
		right := p.parseUnary()
		left = &ast.BinaryExpr{Left: left, Op: op, Right: right, NodePos: p.pos2ast(tok)}
	}
	return left
}

func (p *Parser) parseUnary() ast.Node {
	tok := p.peek()
	if t := tok.Type; t == lexer.TOKEN_BANG || t == lexer.TOKEN_MINUS {
		op := p.advance().Literal
		operand := p.parseUnary()
		return &ast.UnaryExpr{Op: op, Operand: operand, NodePos: p.pos2ast(tok)}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() ast.Node {
	expr := p.parseCall()
	tok := p.peekRaw()
	if tok.Type == lexer.TOKEN_PLUSPLUS || tok.Type == lexer.TOKEN_MINUSMINUS {
		op := p.advance().Literal
		return &ast.PostfixExpr{Op: op, Operand: expr, NodePos: p.pos2ast(tok)}
	}
	return expr
}

func (p *Parser) parseCall() ast.Node {
	expr := p.parsePrimary()
	for {
		if p.peek().Type == lexer.TOKEN_LPAREN {
			expr = p.parseCallArgs(expr)
		} else if p.peek().Type == lexer.TOKEN_DOT {
			tok := p.advance()
			member, _ := p.expect(lexer.TOKEN_IDENT)
			expr = &ast.MemberExpr{Object: expr, Member: member.Literal, NodePos: p.pos2ast(tok)}
		} else if p.peek().Type == lexer.TOKEN_LBRACKET {
			tok := p.advance()
			idx := p.parseExpr()
			p.expect(lexer.TOKEN_RBRACKET)
			expr = &ast.IndexExpr{Object: expr, Index: idx, NodePos: p.pos2ast(tok)}
		} else {
			break
		}
	}
	return expr
}

func (p *Parser) parseCallArgs(callee ast.Node) ast.Node {
	tok := p.peek()
	p.expect(lexer.TOKEN_LPAREN)

	var args []ast.Node
	for p.peek().Type != lexer.TOKEN_RPAREN && p.peek().Type != lexer.TOKEN_EOF {
		args = append(args, p.parseExpr())

		if p.peek().Type == lexer.TOKEN_COMMA {
			p.advance()
		} else {
			break
		}
	}

	p.expect(lexer.TOKEN_RPAREN)
	return &ast.CallExpr{Callee: callee, Args: args, NodePos: p.pos2ast(tok)}
}

func (p *Parser) parsePrimary() ast.Node {
	tok := p.peek()
	pos := p.pos2ast(tok)
	switch tok.Type {
	case lexer.TOKEN_INT:
		p.advance()
		v, _ := strconv.ParseInt(tok.Literal, 10, 64)
		return &ast.IntLiteral{Value: v, NodePos: pos}
	case lexer.TOKEN_FLOAT:
		p.advance()
		v, _ := strconv.ParseFloat(tok.Literal, 64)
		return &ast.FloatLiteral{Value: v, NodePos: pos}
	case lexer.TOKEN_STRING:
		p.advance()
		return &ast.StringLiteral{Value: tok.Literal, NodePos: pos}
	case lexer.TOKEN_TRUE:
		p.advance()
		return &ast.BoolLiteral{Value: true, NodePos: pos}
	case lexer.TOKEN_FALSE:
		p.advance()
		return &ast.BoolLiteral{Value: false, NodePos: pos}
	case lexer.TOKEN_NIL:
		p.advance()
		return &ast.NilLiteral{NodePos: pos}
	case lexer.TOKEN_IDENT:
		p.advance()
		if p.peek().Type == lexer.TOKEN_LBRACE {
			return p.parseStructLiteral(tok)
		}
		return &ast.Identifier{Name: tok.Literal, NodePos: pos}
	case lexer.TOKEN_LPAREN:
		p.advance()
		if p.peek().Type == lexer.TOKEN_IDENT {
			saved := p.pos
			names := []string{}
			for p.peek().Type == lexer.TOKEN_IDENT {
				names = append(names, p.advance().Literal)
				if p.peek().Type == lexer.TOKEN_COMMA {
					p.advance()
				} else {
					break
				}
			}
			if p.peek().Type == lexer.TOKEN_RPAREN && len(names) > 1 {
				p.advance()
				return &ast.MultiVarDecl{Names: names, NodePos: pos}
			}
			p.pos = saved
		}
		expr := p.parseExpr()
		p.expect(lexer.TOKEN_RPAREN)
		return expr
	case lexer.TOKEN_LBRACKET:
		return p.parseArrayLiteral()
	case lexer.TOKEN_LBRACE:
		return p.parseMapLiteral()
	case lexer.TOKEN_FUNC:
		return p.parseFuncLiteral()
	}
	p.errors = append(p.errors, fmt.Sprintf("line %d:%d: unexpected token %s (%q)", tok.Line, tok.Col, tok.Type, tok.Literal))
	p.advance()
	return &ast.NilLiteral{NodePos: pos}
}

func (p *Parser) parseStructLiteral(nameTok lexer.Token) ast.Node {
	pos := p.pos2ast(nameTok)
	p.expect(lexer.TOKEN_LBRACE)
	sl := &ast.StructLiteral{Name: nameTok.Literal, NodePos: pos}
	p.skipTerminators()
	for p.peek().Type != lexer.TOKEN_RBRACE && p.peek().Type != lexer.TOKEN_EOF {
		fieldName, _ := p.expect(lexer.TOKEN_IDENT)
		p.expect(lexer.TOKEN_COLON)
		val := p.parseExpr()
		sl.Fields = append(sl.Fields, ast.StructFieldVal{Name: fieldName.Literal, Value: val})
		if p.peek().Type == lexer.TOKEN_COMMA {
			p.advance()
		}
		p.skipTerminators()
	}
	p.expect(lexer.TOKEN_RBRACE)
	return sl
}

func (p *Parser) parseArrayLiteral() ast.Node {
	tok := p.peek()
	p.expect(lexer.TOKEN_LBRACKET)

	var elems []ast.Node
	for p.peek().Type != lexer.TOKEN_RBRACKET && p.peek().Type != lexer.TOKEN_EOF {
		elems = append(elems, p.parseExpr())

		if p.peek().Type == lexer.TOKEN_COMMA {
			p.advance()
		} else {
			break
		}
	}

	p.expect(lexer.TOKEN_RBRACKET)
	return &ast.ArrayLiteral{Elements: elems, NodePos: p.pos2ast(tok)}
}

func (p *Parser) parseMapLiteral() ast.Node {
	tok := p.peek()
	p.expect(lexer.TOKEN_LBRACE)
	var pairs []ast.MapPair
	p.skipTerminators()
	for p.peek().Type != lexer.TOKEN_RBRACE && p.peek().Type != lexer.TOKEN_EOF {
		key := p.parseExpr()
		p.expect(lexer.TOKEN_COLON)
		val := p.parseExpr()
		pairs = append(pairs, ast.MapPair{Key: key, Value: val})
		if p.peek().Type == lexer.TOKEN_COMMA {
			p.advance()
		}
		p.skipTerminators()
	}
	p.expect(lexer.TOKEN_RBRACE)
	return &ast.MapLiteral{Pairs: pairs, NodePos: p.pos2ast(tok)}
}

func (p *Parser) parseFuncLiteral() ast.Node {
	tok := p.advance()
	params := p.parseParams()
	returns := p.parseReturnTypes()
	body := p.parseBlock()
	return &ast.FuncLiteral{Params: params, Returns: returns, Body: body, NodePos: p.pos2ast(tok)}
}
