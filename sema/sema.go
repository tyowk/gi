package sema

import (
	"fmt"
	"strings"

	"github.com/tyowk/gi/ast"
)

type Type int

const (
	TyUnknown Type = iota
	TyNil
	TyInt
	TyFloat
	TyString
	TyBool
	TyArray
	TyMap
	TyFunc
	TyStruct
	TyAny
)

func (t Type) String() string {
	switch t {
	case TyNil:
		return "nil"
	case TyInt:
		return "int"
	case TyFloat:
		return "float"
	case TyString:
		return "string"
	case TyBool:
		return "bool"
	case TyArray:
		return "array"
	case TyMap:
		return "map"
	case TyFunc:
		return "func"
	case TyStruct:
		return "struct"
	case TyAny:
		return "any"
	}
	return "unknown"
}

func typeFromString(s string) Type {
	switch s {
	case "int":
		return TyInt
	case "float":
		return TyFloat
	case "string":
		return TyString
	case "bool":
		return TyBool
	case "array":
		return TyArray
	case "map":
		return TyMap
	case "func":
		return TyFunc
	case "nil":
		return TyNil
	}
	return TyAny
}

type Symbol struct {
	Name    string
	Type    Type
	IsConst bool
	IsFunc  bool
	Params  []ast.Param
	Pos     ast.Pos
}

type Scope struct {
	symbols map[string]*Symbol
	parent  *Scope
}

func newScope(parent *Scope) *Scope {
	return &Scope{symbols: make(map[string]*Symbol), parent: parent}
}

func (s *Scope) define(sym *Symbol) {
	s.symbols[sym.Name] = sym
}

func (s *Scope) lookup(name string) (*Symbol, bool) {
	if sym, ok := s.symbols[name]; ok {
		return sym, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil, false
}

func (s *Scope) lookupLocal(name string) (*Symbol, bool) {
	if sym, ok := s.symbols[name]; ok {
		return sym, true
	}
	return nil, false
}

type Error struct {
	Pos     ast.Pos
	Message string
}

func (e *Error) Error() string {
	if e.Pos.Line == 0 {
		return e.Message
	}
	return fmt.Sprintf("line %d:%d: %s", e.Pos.Line, e.Pos.Col, e.Message)
}

type Analyzer struct {
	errors     []*Error
	scope      *Scope
	structDefs map[string][]ast.StructField
	funcName   string
	inLoop     int
	inFunc     int
}

func New() *Analyzer {
	a := &Analyzer{
		structDefs: make(map[string][]ast.StructField),
	}
	a.scope = newScope(nil)
	a.registerBuiltins()
	return a
}

func (a *Analyzer) Errors() []*Error { return a.errors }

func (a *Analyzer) HasErrors() bool { return len(a.errors) > 0 }

func (a *Analyzer) ErrorStrings() []string {
	out := make([]string, len(a.errors))
	for i, e := range a.errors {
		out[i] = e.Error()
	}
	return out
}

func (a *Analyzer) errorf(pos ast.Pos, format string, args ...interface{}) {
	a.errors = append(a.errors, &Error{
		Pos:     pos,
		Message: fmt.Sprintf(format, args...),
	})
}

func (a *Analyzer) registerBuiltins() {
	builtins := []string{
		"println", "print", "printf", "sprintf", "input",
		"len", "append", "delete", "keys", "typeof",
		"int", "float", "string", "bool",
		"exit", "panic", "readFile", "writeFile", "args",
		"buildGi", "values", "push", "pop", "contains", "indexOf",
		"split", "join", "trim", "upper", "lower", "parseInt",
		"parseFloat", "Number", "String", "Boolean", "now",
		"math",
		"http", "json", "os", "time", "strings", "strconv", "rand", "path", "std",
	}
	for _, name := range builtins {
		a.scope.define(&Symbol{Name: name, Type: TyAny, IsFunc: true})
	}
}

func (a *Analyzer) Analyze(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		switch n := stmt.(type) {
		case *ast.StructDecl:
			a.declareStruct(n)
		case *ast.FuncDecl:
			a.scope.define(&Symbol{
				Name:   n.Name,
				Type:   TyFunc,
				IsFunc: true,
				Params: n.Params,
				Pos:    n.NodePos,
			})
		}
	}

	for _, stmt := range prog.Statements {
		a.analyzeStmt(stmt)
	}
}

func (a *Analyzer) declareStruct(n *ast.StructDecl) {
	seen := make(map[string]bool)
	for _, f := range n.Fields {
		if seen[f.Name] {
			a.errorf(n.NodePos, "struct %q has duplicate field %q", n.Name, f.Name)
		}
		seen[f.Name] = true
	}
	a.structDefs[n.Name] = n.Fields
	a.scope.define(&Symbol{Name: n.Name, Type: TyStruct, Pos: n.NodePos})
}

func (a *Analyzer) pushScope() { a.scope = newScope(a.scope) }
func (a *Analyzer) popScope()  { a.scope = a.scope.parent }

func (a *Analyzer) analyzeStmt(node ast.Node) {
	switch n := node.(type) {
	case *ast.ImportStmt:
		if len(n.Names) == 0 {
			a.errorf(n.NodePos, "import statement has no names")
		}
		if n.Path == "" {
			a.errorf(n.NodePos, "import statement has empty path")
		}
		for i, name := range n.Names {
			visibleName := name
			if i < len(n.Aliases) && n.Aliases[i] != "" {
				visibleName = n.Aliases[i]
			}
			if _, ok := a.scope.lookupLocal(visibleName); ok {
				a.errorf(n.NodePos, "import name %q is already defined", visibleName)
			}
			a.scope.define(&Symbol{Name: visibleName, Type: TyAny, Pos: n.NodePos})
		}

	case *ast.StructDecl:
		a.declareStruct(n)

	case *ast.FuncDecl:
		a.scope.define(&Symbol{Name: n.Name, Type: TyFunc, IsFunc: true, Params: n.Params, Pos: n.NodePos})
		a.analyzeFunc(n.Params, n.Body, n.Name)

	case *ast.VarDecl:
		ty := a.analyzeExpr(n.Value)
		a.scope.define(&Symbol{Name: n.Name, Type: ty, IsConst: n.IsConst, Pos: n.NodePos})

	case *ast.MultiVarDecl:
		a.analyzeExpr(n.Value)
		for _, name := range n.Names {
			a.scope.define(&Symbol{Name: name, Type: TyAny, Pos: n.NodePos})
		}

	case *ast.AssignStmt:
		a.analyzeAssignTarget(n.Target, n.NodePos)
		a.analyzeExpr(n.Value)

	case *ast.ReturnStmt:
		if a.inFunc == 0 {
			a.errorf(n.NodePos, "return statement outside of function")
		}
		for _, rv := range n.Values {
			a.analyzeExpr(rv)
		}

	case *ast.BreakStmt:
		if a.inLoop == 0 {
			a.errorf(n.NodePos, "break statement outside of loop")
		}

	case *ast.ContinueStmt:
		if a.inLoop == 0 {
			a.errorf(n.NodePos, "continue statement outside of loop")
		}

	case *ast.IfStmt:
		a.analyzeExpr(n.Condition)
		a.pushScope()
		a.analyzeBlock(n.Then)
		a.popScope()
		if n.Else != nil {
			a.analyzeStmt(n.Else)
		}

	case *ast.ForStmt:
		a.pushScope()
		a.inLoop++
		if n.Init != nil {
			a.analyzeStmt(n.Init)
		}
		if n.Condition != nil {
			a.analyzeExpr(n.Condition)
		}
		if n.Post != nil {
			a.analyzeStmt(n.Post)
		}
		a.analyzeBlock(n.Body)
		a.inLoop--
		a.popScope()

	case *ast.Block:
		a.pushScope()
		a.analyzeBlock(n)
		a.popScope()

	case *ast.ExprStmt:
		a.analyzeExpr(n.Expr)
	}
}

func (a *Analyzer) analyzeAssignTarget(target ast.Node, pos ast.Pos) {
	switch t := target.(type) {
	case *ast.Identifier:
		sym, ok := a.scope.lookup(t.Name)
		if !ok {
			a.errorf(t.NodePos, "undefined variable %q", t.Name)
			return
		}
		if sym.IsConst {
			a.errorf(t.NodePos, "cannot assign to const %q", t.Name)
		}
	case *ast.IndexExpr:
		a.analyzeExpr(t.Object)
		a.analyzeExpr(t.Index)
	case *ast.MemberExpr:
		a.analyzeExpr(t.Object)
	default:
		a.errorf(pos, "invalid assignment target")
	}
}

func (a *Analyzer) analyzeFunc(params []ast.Param, body *ast.Block, name string) {
	a.pushScope()
	a.inFunc++
	prev := a.funcName
	a.funcName = name
	for _, p := range params {
		ty := TyAny
		if p.Type != "" {
			ty = typeFromString(p.Type)
		}
		if p.Default != nil {
			a.analyzeExpr(p.Default)
		}
		a.scope.define(&Symbol{Name: p.Name, Type: ty})
	}
	a.analyzeBlock(body)
	a.funcName = prev
	a.inFunc--
	a.popScope()
}

func (a *Analyzer) analyzeBlock(block *ast.Block) {
	for _, stmt := range block.Statements {
		a.analyzeStmt(stmt)
	}
}

func (a *Analyzer) analyzeExpr(node ast.Node) Type {
	if node == nil {
		return TyNil
	}
	switch n := node.(type) {
	case *ast.IntLiteral:
		return TyInt
	case *ast.FloatLiteral:
		return TyFloat
	case *ast.StringLiteral:
		return TyString
	case *ast.BoolLiteral:
		return TyBool
	case *ast.NilLiteral:
		return TyNil

	case *ast.Identifier:
		if _, ok := a.scope.lookup(n.Name); !ok {
			a.errorf(n.NodePos, "undefined variable %q", n.Name)
			return TyUnknown
		}
		sym, _ := a.scope.lookup(n.Name)
		return sym.Type

	case *ast.ArrayLiteral:
		for _, el := range n.Elements {
			a.analyzeExpr(el)
		}
		return TyArray

	case *ast.MapLiteral:
		for _, pair := range n.Pairs {
			a.analyzeExpr(pair.Key)
			a.analyzeExpr(pair.Value)
		}
		return TyMap

	case *ast.StructLiteral:
		if _, ok := a.structDefs[n.Name]; !ok {
			if _, isScope := a.scope.lookup(n.Name); !isScope {
				a.errorf(n.NodePos, "undefined struct type %q", n.Name)
			}
		}
		if fields, ok := a.structDefs[n.Name]; ok {
			defined := make(map[string]bool)
			for _, f := range fields {
				defined[f.Name] = true
			}
			for _, fv := range n.Fields {
				if !defined[fv.Name] {
					a.errorf(n.NodePos, "struct %q has no field %q", n.Name, fv.Name)
				}
				a.analyzeExpr(fv.Value)
			}
		} else {
			for _, fv := range n.Fields {
				a.analyzeExpr(fv.Value)
			}
		}
		return TyStruct

	case *ast.FuncLiteral:
		a.analyzeFunc(n.Params, n.Body, "<anonymous>")
		return TyFunc

	case *ast.BinaryExpr:
		lt := a.analyzeExpr(n.Left)
		rt := a.analyzeExpr(n.Right)
		return a.binaryResultType(n.Op, lt, rt, n.NodePos)

	case *ast.UnaryExpr:
		t := a.analyzeExpr(n.Operand)
		switch n.Op {
		case "!":
			return TyBool
		case "-":
			if t == TyFloat {
				return TyFloat
			}
			return TyInt
		}
		return t

	case *ast.PostfixExpr:
		t := a.analyzeExpr(n.Operand)
		if t != TyInt && t != TyFloat && t != TyAny && t != TyUnknown {
			a.errorf(n.NodePos, "operator %s requires numeric operand, got %s", n.Op, t)
		}
		return t

	case *ast.CallExpr:
		a.analyzeCallExpr(n)
		return TyAny

	case *ast.MemberExpr:
		a.analyzeExpr(n.Object)
		return TyAny

	case *ast.IndexExpr:
		obj := a.analyzeExpr(n.Object)
		idx := a.analyzeExpr(n.Index)
		if obj == TyString && idx != TyInt && idx != TyAny {
			a.errorf(n.NodePos, "string index must be int, got %s", idx)
		}
		return TyAny

	case *ast.MultiVarDecl:
		return TyAny
	}
	return TyUnknown
}

func (a *Analyzer) analyzeCallExpr(n *ast.CallExpr) {
	if ident, ok := n.Callee.(*ast.Identifier); ok {
		sym, found := a.scope.lookup(ident.Name)
		if !found {
			a.errorf(n.NodePos, "undefined function %q", ident.Name)
			for _, arg := range n.Args {
				a.analyzeExpr(arg)
			}
			return
		}

		if sym.IsFunc && len(sym.Params) > 0 {
			required := 0
			for _, p := range sym.Params {
				if p.Default == nil && p.Required != false {
					required++
				}
			}
			if len(n.Args) < required {
				a.errorf(n.NodePos, "function %q expects at least %d argument(s), got %d",
					ident.Name, required, len(n.Args))
			}
		}
	} else {
		a.analyzeExpr(n.Callee)
	}
	for _, arg := range n.Args {
		a.analyzeExpr(arg)
	}
}

func (a *Analyzer) binaryResultType(op string, lt, rt Type, pos ast.Pos) Type {
	switch op {
	case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
		return TyBool
	case "+":
		if lt == TyString || rt == TyString {
			return TyString
		}
		if lt == TyFloat || rt == TyFloat {
			return TyFloat
		}
		if lt == TyInt && rt == TyInt {
			return TyInt
		}
		return TyAny
	case "-", "*", "/", "%":
		if lt == TyString || rt == TyString {
			a.errorf(pos, "operator %q is not defined for string operands", op)
			return TyUnknown
		}
		if lt == TyFloat || rt == TyFloat {
			return TyFloat
		}
		return TyInt
	}
	return TyAny
}

type Report struct {
	Errors   []string
	Warnings []string
	Symbols  map[string]string
}

func (a *Analyzer) Report() *Report {
	r := &Report{
		Symbols: make(map[string]string),
	}
	for _, e := range a.errors {
		r.Errors = append(r.Errors, e.Error())
	}
	for name, sym := range a.scope.symbols {
		if !isBuiltin(name) {
			r.Symbols[name] = sym.Type.String()
		}
	}
	return r
}

func isBuiltin(name string) bool {
	builtins := map[string]bool{
		"println": true, "print": true, "printf": true, "sprintf": true,
		"input": true, "len": true, "append": true, "delete": true,
		"keys": true, "typeof": true, "int": true, "float": true,
		"string": true, "bool": true, "exit": true, "panic": true,
		"readFile": true, "writeFile": true, "args": true, "math": true,
		"values": true, "push": true, "pop": true, "contains": true,
		"indexOf": true, "split": true, "join": true, "trim": true,
		"upper": true, "lower": true, "parseInt": true, "parseFloat": true,
		"Number": true, "String": true, "Boolean": true, "now": true,
		"http": true, "json": true, "os": true, "time": true,
		"strings": true, "strconv": true, "rand": true, "path": true, "std": true,
	}
	return builtins[name]
}

func FormatReport(r *Report) string {
	var sb strings.Builder
	if len(r.Errors) == 0 {
		sb.WriteString("semantic analysis: no errors\n")
	} else {
		sb.WriteString(fmt.Sprintf("semantic analysis: %d error(s)\n", len(r.Errors)))
		for _, e := range r.Errors {
			sb.WriteString("  error: " + e + "\n")
		}
	}
	if len(r.Warnings) > 0 {
		for _, w := range r.Warnings {
			sb.WriteString("  warning: " + w + "\n")
		}
	}
	return sb.String()
}
