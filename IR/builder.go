package IR

import (
	"fmt"

	"github.com/tyowk/gi/ast"
)

var builtinNames = map[string]bool{
	"println": true, "print": true, "printf": true, "sprintf": true,
	"input": true, "len": true, "append": true, "delete": true,
	"keys": true, "typeof": true, "int": true, "float": true,
	"string": true, "bool": true, "exit": true, "panic": true,
	"readFile": true, "writeFile": true, "args": true,
}

type Builder struct {
	module     *Module
	cur        *Function
	anonCount  int
	funcDefs   map[string][]ast.Param
	structDefs map[string][]ast.StructField
}

func NewBuilder() *Builder {
	return &Builder{
		module:     NewModule(),
		funcDefs:   make(map[string][]ast.Param),
		structDefs: make(map[string][]ast.StructField),
	}
}

func (b *Builder) Module() *Module { return b.module }

func (b *Builder) Build(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		switch n := stmt.(type) {
		case *ast.FuncDecl:
			b.funcDefs[n.Name] = n.Params
		case *ast.StructDecl:
			b.structDefs[n.Name] = n.Fields
		}
	}

	main := NewFunction("<main>", nil)
	b.module.AddFunction(main)
	b.cur = main

	for _, stmt := range prog.Statements {
		b.buildStmt(stmt)
	}

	b.emit(OpReturn, nil, "end of module", 0, 0)
}

func (b *Builder) emit(op Opcode, operand interface{}, comment string, line, col int) *Instr {
	return b.cur.Emit(op, operand, comment, line, col)
}

func (b *Builder) emitAt(op Opcode, operand interface{}, pos ast.Pos) *Instr {
	return b.cur.Emit(op, operand, "", pos.Line, pos.Col)
}

func (b *Builder) buildStmt(node ast.Node) {
	switch n := node.(type) {
	case *ast.ImportStmt:
		b.emitAt(OpNop, fmt.Sprintf("import %v from %q", n.Names, n.Path), n.NodePos)

	case *ast.StructDecl:
		b.structDefs[n.Name] = n.Fields
		b.emitAt(OpNop, fmt.Sprintf("struct %s", n.Name), n.NodePos)

	case *ast.FuncDecl:
		b.buildFuncDecl(n)

	case *ast.VarDecl:
		b.buildExpr(n.Value)
		b.emitAt(OpStore, n.Name, n.NodePos)

	case *ast.MultiVarDecl:
		b.buildExpr(n.Value)
		for i, name := range n.Names {
			b.emit(OpDup, nil, "dup for multi-assign", n.NodePos.Line, n.NodePos.Col)
			b.emit(OpConst, int64(i), "", n.NodePos.Line, n.NodePos.Col)
			b.emit(OpLoadIndex, nil, "", n.NodePos.Line, n.NodePos.Col)
			b.emit(OpStore, name, "", n.NodePos.Line, n.NodePos.Col)
		}
		b.emit(OpPop, nil, "drop tuple", n.NodePos.Line, n.NodePos.Col)

	case *ast.AssignStmt:
		b.buildExpr(n.Value)
		b.buildAssignTarget(n.Target, n.Op, n.NodePos)

	case *ast.ReturnStmt:
		if len(n.Values) == 0 {
			b.emitAt(OpNil, nil, n.NodePos)
		} else if len(n.Values) == 1 {
			b.buildExpr(n.Values[0])
		} else {
			b.emit(OpMakeArray, 0, "multi-return tuple", n.NodePos.Line, n.NodePos.Col)
			for _, rv := range n.Values {
				b.buildExpr(rv)
				b.emit(OpArrayPush, nil, "", n.NodePos.Line, n.NodePos.Col)
			}
		}
		b.emitAt(OpReturn, len(n.Values), n.NodePos)

	case *ast.BreakStmt:
		b.emitAt(OpJump, "<break>", n.NodePos)

	case *ast.ContinueStmt:
		b.emitAt(OpJump, "<continue>", n.NodePos)

	case *ast.IfStmt:
		b.buildIf(n)

	case *ast.ForStmt:
		b.buildFor(n)

	case *ast.Block:
		for _, s := range n.Statements {
			b.buildStmt(s)
		}

	case *ast.ExprStmt:
		b.buildExpr(n.Expr)
		b.emitAt(OpPop, nil, n.NodePos)
	}
}

func (b *Builder) buildFuncDecl(n *ast.FuncDecl) {
	params := make([]string, len(n.Params))
	for i, p := range n.Params {
		params[i] = p.Name
	}
	fn := NewFunction(n.Name, params)
	b.module.AddFunction(fn)

	outer := b.cur
	b.cur = fn

	b.emit(OpFuncDef, n.Name, "", n.NodePos.Line, n.NodePos.Col)
	for i, p := range n.Params {
		b.emit(OpStore, p.Name, fmt.Sprintf("param %d", i), n.NodePos.Line, n.NodePos.Col)
	}

	for _, stmt := range n.Body.Statements {
		b.buildStmt(stmt)
	}

	b.emit(OpNil, nil, "implicit nil return", 0, 0)
	b.emit(OpReturn, 1, "", 0, 0)
	b.emit(OpFuncEnd, n.Name, "", 0, 0)

	b.cur = outer
	b.emit(OpNop, fmt.Sprintf("declared func %s", n.Name), "", n.NodePos.Line, n.NodePos.Col)
}

func (b *Builder) buildAssignTarget(target ast.Node, op string, pos ast.Pos) {
	switch t := target.(type) {
	case *ast.Identifier:
		if op != "=" {
			b.emit(OpLoad, t.Name, "", pos.Line, pos.Col)
			b.emit(opFromCompound(op), nil, "", pos.Line, pos.Col)
		}
		b.emit(OpStore, t.Name, "", pos.Line, pos.Col)

	case *ast.IndexExpr:
		b.buildExpr(t.Object)
		b.buildExpr(t.Index)
		b.emit(OpStoreIndex, nil, "", pos.Line, pos.Col)

	case *ast.MemberExpr:
		b.buildExpr(t.Object)
		b.emit(OpStoreMember, t.Member, "", pos.Line, pos.Col)
	}
}

func opFromCompound(op string) Opcode {
	switch op {
	case "+=":
		return OpAdd
	case "-=":
		return OpSub
	case "*=":
		return OpMul
	case "/=":
		return OpDiv
	}
	return OpNop
}

func (b *Builder) buildIf(n *ast.IfStmt) {
	elseLabel := b.cur.NewLabel()
	endLabel := b.cur.NewLabel()

	b.buildExpr(n.Condition)
	b.emitAt(OpJumpIfFalse, elseLabel, n.NodePos)

	for _, s := range n.Then.Statements {
		b.buildStmt(s)
	}

	if n.Else != nil {
		b.emit(OpJump, endLabel, "", 0, 0)
	}

	b.cur.EmitLabel(elseLabel)

	if n.Else != nil {
		b.buildStmt(n.Else)
		b.cur.EmitLabel(endLabel)
	}
}

func (b *Builder) buildFor(n *ast.ForStmt) {
	loopLabel := b.cur.NewLabel()
	endLabel := b.cur.NewLabel()
	postLabel := b.cur.NewLabel()

	if n.Init != nil {
		b.buildStmt(n.Init)
	}

	b.cur.EmitLabel(loopLabel)

	if n.Condition != nil {
		b.buildExpr(n.Condition)
		b.emit(OpJumpIfFalse, endLabel, "", 0, 0)
	}

	for _, s := range n.Body.Statements {
		stmt := s
		if _, ok := stmt.(*ast.ContinueStmt); ok {
			b.emit(OpJump, postLabel, "continue", 0, 0)
			continue
		}
		if _, ok := stmt.(*ast.BreakStmt); ok {
			b.emit(OpJump, endLabel, "break", 0, 0)
			continue
		}
		b.buildStmt(stmt)
	}

	b.cur.EmitLabel(postLabel)

	if n.Post != nil {
		b.buildStmt(n.Post)
	}

	b.emit(OpJump, loopLabel, "", 0, 0)
	b.cur.EmitLabel(endLabel)
}

func (b *Builder) buildExpr(node ast.Node) {
	if node == nil {
		b.emit(OpNil, nil, "", 0, 0)
		return
	}
	switch n := node.(type) {
	case *ast.IntLiteral:
		b.emitAt(OpConst, n.Value, n.NodePos)

	case *ast.FloatLiteral:
		b.emitAt(OpConst, n.Value, n.NodePos)

	case *ast.StringLiteral:
		b.emitAt(OpConst, n.Value, n.NodePos)

	case *ast.BoolLiteral:
		if n.Value {
			b.emitAt(OpTrue, nil, n.NodePos)
		} else {
			b.emitAt(OpFalse, nil, n.NodePos)
		}

	case *ast.NilLiteral:
		b.emitAt(OpNil, nil, n.NodePos)

	case *ast.Identifier:
		b.emitAt(OpLoad, n.Name, n.NodePos)

	case *ast.ArrayLiteral:
		b.emitAt(OpMakeArray, len(n.Elements), n.NodePos)
		for _, el := range n.Elements {
			b.buildExpr(el)
			b.emit(OpArrayPush, nil, "", n.NodePos.Line, n.NodePos.Col)
		}

	case *ast.MapLiteral:
		b.emitAt(OpMakeMap, len(n.Pairs), n.NodePos)
		for _, pair := range n.Pairs {
			b.buildExpr(pair.Key)
			b.buildExpr(pair.Value)
			b.emit(OpMapSet, nil, "", n.NodePos.Line, n.NodePos.Col)
		}

	case *ast.StructLiteral:
		b.emitAt(OpMakeStruct, n.Name, n.NodePos)
		for _, fv := range n.Fields {
			b.buildExpr(fv.Value)
			b.emit(OpStructSet, fv.Name, "", n.NodePos.Line, n.NodePos.Col)
		}

	case *ast.FuncLiteral:
		b.anonCount++
		anonName := fmt.Sprintf("<anon_%d>", b.anonCount)
		params := make([]string, len(n.Params))
		for i, p := range n.Params {
			params[i] = p.Name
		}
		fn := NewFunction(anonName, params)
		b.module.AddFunction(fn)

		outer := b.cur
		b.cur = fn
		b.emit(OpFuncDef, anonName, "", n.NodePos.Line, n.NodePos.Col)
		for _, stmt := range n.Body.Statements {
			b.buildStmt(stmt)
		}
		b.emit(OpNil, nil, "implicit nil return", 0, 0)
		b.emit(OpReturn, 1, "", 0, 0)
		b.emit(OpFuncEnd, anonName, "", 0, 0)
		b.cur = outer
		b.emitAt(OpLoad, anonName, n.NodePos)

	case *ast.BinaryExpr:
		b.buildBinary(n)

	case *ast.UnaryExpr:
		b.buildExpr(n.Operand)
		switch n.Op {
		case "!":
			b.emitAt(OpNot, nil, n.NodePos)
		case "-":
			b.emitAt(OpNeg, nil, n.NodePos)
		}

	case *ast.PostfixExpr:
		b.buildExpr(n.Operand)
		b.emit(OpDup, nil, "save old value", n.NodePos.Line, n.NodePos.Col)
		b.emit(OpConst, int64(1), "", n.NodePos.Line, n.NodePos.Col)
		if n.Op == "++" {
			b.emitAt(OpAdd, nil, n.NodePos)
		} else {
			b.emitAt(OpSub, nil, n.NodePos)
		}
		b.buildAssignTarget(n.Operand, "=", n.NodePos)

	case *ast.CallExpr:
		b.buildCall(n)

	case *ast.MemberExpr:
		b.buildExpr(n.Object)
		b.emitAt(OpLoadMember, n.Member, n.NodePos)

	case *ast.IndexExpr:
		b.buildExpr(n.Object)
		b.buildExpr(n.Index)
		b.emitAt(OpLoadIndex, nil, n.NodePos)

	case *ast.MultiVarDecl:
		b.emit(OpNil, nil, "", 0, 0)
	}
}

func (b *Builder) buildBinary(n *ast.BinaryExpr) {
	if n.Op == "&&" {
		shortLabel := b.cur.NewLabel()
		endLabel := b.cur.NewLabel()
		b.buildExpr(n.Left)
		b.emit(OpDup, nil, "short-circuit &&", n.NodePos.Line, n.NodePos.Col)
		b.emit(OpJumpIfFalse, shortLabel, "", n.NodePos.Line, n.NodePos.Col)
		b.emit(OpPop, nil, "", n.NodePos.Line, n.NodePos.Col)
		b.buildExpr(n.Right)
		b.emit(OpJump, endLabel, "", n.NodePos.Line, n.NodePos.Col)
		b.cur.EmitLabel(shortLabel)
		b.cur.EmitLabel(endLabel)
		return
	}
	if n.Op == "||" {
		shortLabel := b.cur.NewLabel()
		endLabel := b.cur.NewLabel()
		b.buildExpr(n.Left)
		b.emit(OpDup, nil, "short-circuit ||", n.NodePos.Line, n.NodePos.Col)
		b.emit(OpJumpIfTrue, shortLabel, "", n.NodePos.Line, n.NodePos.Col)
		b.emit(OpPop, nil, "", n.NodePos.Line, n.NodePos.Col)
		b.buildExpr(n.Right)
		b.emit(OpJump, endLabel, "", n.NodePos.Line, n.NodePos.Col)
		b.cur.EmitLabel(shortLabel)
		b.cur.EmitLabel(endLabel)
		return
	}

	b.buildExpr(n.Left)
	b.buildExpr(n.Right)

	switch n.Op {
	case "+":
		b.emitAt(OpAdd, nil, n.NodePos)
	case "-":
		b.emitAt(OpSub, nil, n.NodePos)
	case "*":
		b.emitAt(OpMul, nil, n.NodePos)
	case "/":
		b.emitAt(OpDiv, nil, n.NodePos)
	case "%":
		b.emitAt(OpMod, nil, n.NodePos)
	case "==":
		b.emitAt(OpEq, nil, n.NodePos)
	case "!=":
		b.emitAt(OpNeq, nil, n.NodePos)
	case "<":
		b.emitAt(OpLt, nil, n.NodePos)
	case "<=":
		b.emitAt(OpLte, nil, n.NodePos)
	case ">":
		b.emitAt(OpGt, nil, n.NodePos)
	case ">=":
		b.emitAt(OpGte, nil, n.NodePos)
	}
}

func (b *Builder) buildCall(n *ast.CallExpr) {
	if mem, ok := n.Callee.(*ast.MemberExpr); ok {
		b.buildExpr(mem.Object)
		for _, arg := range n.Args {
			b.buildExpr(arg)
		}
		b.emitAt(OpCallMethod, fmt.Sprintf("%s/%d", mem.Member, len(n.Args)), n.NodePos)
		return
	}

	if ident, ok := n.Callee.(*ast.Identifier); ok {
		if builtinNames[ident.Name] {
			for _, arg := range n.Args {
				b.buildExpr(arg)
			}
			b.emitAt(OpCallBuiltin, fmt.Sprintf("%s/%d", ident.Name, len(n.Args)), n.NodePos)
			return
		}
	}

	b.buildExpr(n.Callee)
	for _, arg := range n.Args {
		b.buildExpr(arg)
	}
	b.emitAt(OpCall, len(n.Args), n.NodePos)
}
