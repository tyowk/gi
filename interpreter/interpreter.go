package interpreter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	mrand "math/rand"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tyowk/gi/ast"
	"github.com/tyowk/gi/gipack"
	"github.com/tyowk/gi/lexer"
	"github.com/tyowk/gi/parser"
)

type ValueKind int

const (
	KindNil ValueKind = iota
	KindInt
	KindFloat
	KindString
	KindBool
	KindArray
	KindMap
	KindStruct
	KindFunc
	KindBuiltin
)

type Value struct {
	Kind       ValueKind
	IntVal     int64
	FltVal     float64
	StrVal     string
	BoolVal    bool
	ArrVal     []*Value
	MapVal     map[string]*Value
	MapKeys    []string
	StructType string
	FnVal      *FuncValue
	BuiltFn    func(args []*Value) (*Value, error)
}

type FuncValue struct {
	Params  []ast.Param
	Returns []string
	Body    *ast.Block
	Env     *Env
}

func nilVal() *Value           { return &Value{Kind: KindNil} }
func intVal(n int64) *Value    { return &Value{Kind: KindInt, IntVal: n} }
func fltVal(f float64) *Value  { return &Value{Kind: KindFloat, FltVal: f} }
func strVal(s string) *Value   { return &Value{Kind: KindString, StrVal: s} }
func boolVal(b bool) *Value    { return &Value{Kind: KindBool, BoolVal: b} }
func arrVal(a []*Value) *Value { return &Value{Kind: KindArray, ArrVal: a} }

func (v *Value) Truthy() bool {
	switch v.Kind {
	case KindNil:
		return false
	case KindBool:
		return v.BoolVal
	case KindInt:
		return v.IntVal != 0
	case KindFloat:
		return v.FltVal != 0
	case KindString:
		return v.StrVal != ""
	case KindArray:
		return len(v.ArrVal) > 0
	}
	return true
}

func (v *Value) String() string {
	switch v.Kind {
	case KindNil:
		return "nil"
	case KindInt:
		return strconv.FormatInt(v.IntVal, 10)
	case KindFloat:
		return strconv.FormatFloat(v.FltVal, 'f', -1, 64)
	case KindString:
		return v.StrVal
	case KindBool:
		if v.BoolVal {
			return "true"
		}
		return "false"
	case KindArray:
		parts := make([]string, len(v.ArrVal))
		for i, el := range v.ArrVal {
			parts[i] = el.String()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case KindMap, KindStruct:
		var sb strings.Builder
		if v.Kind == KindStruct && v.StructType != "" {
			sb.WriteString(v.StructType)
		}
		sb.WriteString("{")
		for i, k := range v.MapKeys {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(k + ": " + v.MapVal[k].String())
		}
		sb.WriteString("}")
		return sb.String()
	case KindFunc, KindBuiltin:
		return "<func>"
	}
	return "nil"
}

func posStr(pos ast.Pos) string {
	if pos.Line == 0 {
		return ""
	}
	return fmt.Sprintf("line %d:%d: ", pos.Line, pos.Col)
}

type returnSignal struct{ values []*Value }
type breakSignal struct{}
type continueSignal struct{}

func (r *returnSignal) Error() string   { return "return" }
func (r *breakSignal) Error() string    { return "break" }
func (r *continueSignal) Error() string { return "continue" }

type Interpreter struct {
	global      *Env
	baseDir     string
	structDefs  map[string][]ast.StructField
	importCache map[string]*Env
}

func New(baseDir string) *Interpreter {
	interp := &Interpreter{
		global:      NewEnv(nil),
		baseDir:     baseDir,
		structDefs:  make(map[string][]ast.StructField),
		importCache: make(map[string]*Env),
	}
	interp.registerStdlib()
	return interp
}

func (interp *Interpreter) Run(prog *ast.Program) error {
	_, err := interp.execBlock(&ast.Block{Statements: prog.Statements}, interp.global)
	if err != nil {
		if _, ok := err.(*returnSignal); ok {
			return nil
		}
		return err
	}
	if mainFn, ok := interp.global.Get("main"); ok && mainFn.Kind == KindFunc {
		_, err := interp.callFunc(mainFn, []*Value{})
		if err != nil {
			if _, ok := err.(*returnSignal); ok {
				return nil
			}
			return err
		}
	}
	return nil
}

func (interp *Interpreter) execBlock(block *ast.Block, env *Env) (*Value, error) {
	for _, stmt := range block.Statements {
		v, err := interp.execNode(stmt, env)
		if err != nil {
			return nil, err
		}
		_ = v
	}
	return nilVal(), nil
}

func (interp *Interpreter) execNode(node ast.Node, env *Env) (*Value, error) {
	switch n := node.(type) {
	case *ast.ImportStmt:
		return interp.execImport(n, env)

	case *ast.StructDecl:
		interp.structDefs[n.Name] = n.Fields
		return nilVal(), nil

	case *ast.FuncDecl:
		fn := &Value{
			Kind:  KindFunc,
			FnVal: &FuncValue{Params: n.Params, Returns: n.Returns, Body: n.Body, Env: env},
		}
		env.Set(n.Name, fn)
		return nilVal(), nil

	case *ast.VarDecl:
		val, err := interp.evalExpr(n.Value, env)
		if err != nil {
			return nil, fmt.Errorf("%s%v", posStr(n.NodePos), err)
		}
		if n.IsConst {
			env.SetConst(n.Name, val)
		} else {
			env.Set(n.Name, val)
		}
		return nilVal(), nil

	case *ast.MultiVarDecl:
		val, err := interp.evalExpr(n.Value, env)
		if err != nil {
			return nil, fmt.Errorf("%s%v", posStr(n.NodePos), err)
		}
		if val.Kind == KindArray {
			for i, name := range n.Names {
				if i < len(val.ArrVal) {
					env.Set(name, val.ArrVal[i])
				} else {
					env.Set(name, nilVal())
				}
			}
		} else {
			for _, name := range n.Names {
				env.Set(name, val)
			}
		}
		return nilVal(), nil

	case *ast.AssignStmt:
		val, err := interp.evalExpr(n.Value, env)
		if err != nil {
			return nil, fmt.Errorf("%s%v", posStr(n.NodePos), err)
		}
		return nil, interp.doAssign(n.Target, n.Op, val, env, n.NodePos)

	case *ast.ReturnStmt:
		if len(n.Values) == 0 {
			return nil, &returnSignal{values: []*Value{nilVal()}}
		}
		if len(n.Values) == 1 {
			val, err := interp.evalExpr(n.Values[0], env)
			if err != nil {
				return nil, fmt.Errorf("%s%v", posStr(n.NodePos), err)
			}
			return nil, &returnSignal{values: []*Value{val}}
		}
		vals := make([]*Value, len(n.Values))
		for i, rv := range n.Values {
			v, err := interp.evalExpr(rv, env)
			if err != nil {
				return nil, fmt.Errorf("%s%v", posStr(n.NodePos), err)
			}
			vals[i] = v
		}
		tuple := arrVal(vals)
		return nil, &returnSignal{values: []*Value{tuple}}

	case *ast.BreakStmt:
		return nil, &breakSignal{}

	case *ast.ContinueStmt:
		return nil, &continueSignal{}

	case *ast.IfStmt:
		cond, err := interp.evalExpr(n.Condition, env)
		if err != nil {
			return nil, fmt.Errorf("%s%v", posStr(n.NodePos), err)
		}
		if cond.Truthy() {
			return interp.execBlock(n.Then, NewEnv(env))
		} else if n.Else != nil {
			return interp.execNode(n.Else, env)
		}
		return nilVal(), nil

	case *ast.Block:
		return interp.execBlock(n, NewEnv(env))

	case *ast.ForStmt:
		return interp.execFor(n, env)

	case *ast.ExprStmt:
		return interp.evalExpr(n.Expr, env)

	default:
		return interp.evalExpr(node, env)
	}
}

func (interp *Interpreter) doAssign(target ast.Node, op string, val *Value, env *Env, pos ast.Pos) error {
	switch t := target.(type) {
	case *ast.Identifier:
		if env.IsConst(t.Name) {
			return fmt.Errorf("%scannot assign to const %q", posStr(pos), t.Name)
		}
		if op != "=" {
			existing, ok := env.Get(t.Name)
			if !ok {
				return fmt.Errorf("%sundefined variable %q", posStr(pos), t.Name)
			}
			var err error
			switch op {
			case "+=":
				val = interp.add(existing, val)
			case "-=":
				val = interp.sub(existing, val)
			case "*=":
				val = interp.mul(existing, val)
			case "/=":
				val, err = interp.div(existing, val)
				if err != nil {
					return fmt.Errorf("%s%v", posStr(pos), err)
				}
			}
		}
		if !env.Assign(t.Name, val) {
			env.Set(t.Name, val)
		}

	case *ast.IndexExpr:
		obj, err := interp.evalExpr(t.Object, env)
		if err != nil {
			return fmt.Errorf("%s%v", posStr(pos), err)
		}
		idx, err := interp.evalExpr(t.Index, env)
		if err != nil {
			return fmt.Errorf("%s%v", posStr(pos), err)
		}
		if obj.Kind == KindArray {
			i := idx.IntVal
			if i < 0 || i >= int64(len(obj.ArrVal)) {
				return fmt.Errorf("%sindex %d out of bounds (len %d)", posStr(pos), i, len(obj.ArrVal))
			}
			obj.ArrVal[i] = val
		} else if obj.Kind == KindMap || obj.Kind == KindStruct {
			k := idx.String()
			if _, exists := obj.MapVal[k]; !exists {
				obj.MapKeys = append(obj.MapKeys, k)
			}
			obj.MapVal[k] = val
		}

	case *ast.MemberExpr:
		obj, err := interp.evalExpr(t.Object, env)
		if err != nil {
			return fmt.Errorf("%s%v", posStr(pos), err)
		}
		if obj.Kind == KindMap || obj.Kind == KindStruct {
			if _, exists := obj.MapVal[t.Member]; !exists {
				obj.MapKeys = append(obj.MapKeys, t.Member)
			}
			obj.MapVal[t.Member] = val
		}

	default:
		return fmt.Errorf("%scannot assign to expression", posStr(pos))
	}
	return nil
}

func (interp *Interpreter) execFor(n *ast.ForStmt, env *Env) (*Value, error) {
	loopEnv := NewEnv(env)
	if n.Init != nil {
		if _, err := interp.execNode(n.Init, loopEnv); err != nil {
			return nil, err
		}
	}
	for {
		if n.Condition != nil {
			cond, err := interp.evalExpr(n.Condition, loopEnv)
			if err != nil {
				return nil, err
			}
			if !cond.Truthy() {
				break
			}
		}
		_, err := interp.execBlock(n.Body, NewEnv(loopEnv))
		if err != nil {
			if _, ok := err.(*breakSignal); ok {
				break
			}
			if _, ok := err.(*continueSignal); ok {
			} else {
				return nil, err
			}
		}
		if n.Post != nil {
			if _, err := interp.execNode(n.Post, loopEnv); err != nil {
				return nil, err
			}
		}
	}
	return nilVal(), nil
}

func (interp *Interpreter) execImport(n *ast.ImportStmt, env *Env) (*Value, error) {
	path, resolveErr := gipack.ResolveImport(n.Path, interp.baseDir)
	if resolveErr != nil {
		return nil, fmt.Errorf("%simport error: %v", posStr(n.NodePos), resolveErr)
	}

	if cached, ok := interp.importCache[path]; ok {
		for i, name := range n.Names {
			if name == "main" {
				return nil, fmt.Errorf("%simport error: cannot import \"main\" — it is an entry point", posStr(n.NodePos))
			}
			val, ok := cached.Get(name)
			if !ok {
				return nil, fmt.Errorf("%simport error: %q not found in %q", posStr(n.NodePos), name, path)
			}
			bindName := name
			if i < len(n.Aliases) && n.Aliases[i] != "" {
				bindName = n.Aliases[i]
			}
			env.Set(bindName, val)
		}
		return nilVal(), nil
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%simport error: cannot read %q: %v", posStr(n.NodePos), path, err)
	}
	lx := lexer.New(string(src))
	tokens := lx.Tokenize()
	p := parser.New(tokens)
	prog := p.Parse()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("%simport parse error in %q:\n  %s", posStr(n.NodePos), path, strings.Join(p.Errors(), "\n  "))
	}
	moduleEnv := NewEnv(interp.global)
	moduleInterp := &Interpreter{
		global:      moduleEnv,
		baseDir:     filepath.Dir(path),
		structDefs:  interp.structDefs,
		importCache: interp.importCache,
	}
	moduleInterp.registerStdlib()
	for _, stmt := range prog.Statements {
		switch nd := stmt.(type) {
		case *ast.FuncDecl:
			if _, execErr := moduleInterp.execNode(nd, moduleEnv); execErr != nil {
				return nil, execErr
			}
		case *ast.StructDecl:
			if _, execErr := moduleInterp.execNode(nd, moduleEnv); execErr != nil {
				return nil, execErr
			}
		case *ast.ImportStmt:
			if _, execErr := moduleInterp.execNode(nd, moduleEnv); execErr != nil {
				return nil, execErr
			}
		}
	}
	interp.importCache[path] = moduleEnv

	for i, name := range n.Names {
		if name == "main" {
			return nil, fmt.Errorf("%simport error: cannot import \"main\" from %q — it is an entry point", posStr(n.NodePos), path)
		}
		val, ok := moduleEnv.Get(name)
		if !ok {
			return nil, fmt.Errorf("%simport error: %q not found in %q", posStr(n.NodePos), name, path)
		}
		bindName := name
		if i < len(n.Aliases) && n.Aliases[i] != "" {
			bindName = n.Aliases[i]
		}
		env.Set(bindName, val)
	}
	return nilVal(), nil
}

func (interp *Interpreter) evalExpr(node ast.Node, env *Env) (*Value, error) {
	switch n := node.(type) {
	case *ast.IntLiteral:
		return intVal(n.Value), nil
	case *ast.FloatLiteral:
		return fltVal(n.Value), nil
	case *ast.StringLiteral:
		return strVal(n.Value), nil
	case *ast.BoolLiteral:
		return boolVal(n.Value), nil
	case *ast.NilLiteral:
		return nilVal(), nil
	case *ast.Identifier:
		v, ok := env.Get(n.Name)
		if !ok {
			return nil, fmt.Errorf("%sundefined variable: %q", posStr(n.NodePos), n.Name)
		}
		return v, nil
	case *ast.ArrayLiteral:
		elems := make([]*Value, len(n.Elements))
		for i, el := range n.Elements {
			v, err := interp.evalExpr(el, env)
			if err != nil {
				return nil, err
			}
			elems[i] = v
		}
		return arrVal(elems), nil
	case *ast.MapLiteral:
		m := &Value{Kind: KindMap, MapVal: make(map[string]*Value)}
		for _, pair := range n.Pairs {
			k, err := interp.evalExpr(pair.Key, env)
			if err != nil {
				return nil, err
			}
			v, err := interp.evalExpr(pair.Value, env)
			if err != nil {
				return nil, err
			}
			key := k.String()
			if _, exists := m.MapVal[key]; !exists {
				m.MapKeys = append(m.MapKeys, key)
			}
			m.MapVal[key] = v
		}
		return m, nil
	case *ast.StructLiteral:
		m := &Value{Kind: KindStruct, StructType: n.Name, MapVal: make(map[string]*Value)}
		if fields, ok := interp.structDefs[n.Name]; ok {
			for _, f := range fields {
				m.MapVal[f.Name] = nilVal()
				m.MapKeys = append(m.MapKeys, f.Name)
			}
		}
		for _, fv := range n.Fields {
			v, err := interp.evalExpr(fv.Value, env)
			if err != nil {
				return nil, err
			}
			if _, exists := m.MapVal[fv.Name]; !exists {
				m.MapKeys = append(m.MapKeys, fv.Name)
			}
			m.MapVal[fv.Name] = v
		}
		return m, nil
	case *ast.FuncLiteral:
		return &Value{Kind: KindFunc, FnVal: &FuncValue{Params: n.Params, Returns: n.Returns, Body: n.Body, Env: env}}, nil
	case *ast.BinaryExpr:
		return interp.evalBinary(n, env)
	case *ast.UnaryExpr:
		return interp.evalUnary(n, env)
	case *ast.PostfixExpr:
		return interp.evalPostfix(n, env)
	case *ast.CallExpr:
		return interp.evalCall(n, env)
	case *ast.MemberExpr:
		return interp.evalMember(n, env)
	case *ast.IndexExpr:
		return interp.evalIndex(n, env)
	case *ast.MultiVarDecl:
		return nilVal(), nil
	}
	return nilVal(), nil
}

func (interp *Interpreter) evalBinary(n *ast.BinaryExpr, env *Env) (*Value, error) {
	if n.Op == "&&" {
		left, err := interp.evalExpr(n.Left, env)
		if err != nil {
			return nil, err
		}
		if !left.Truthy() {
			return boolVal(false), nil
		}
		right, err := interp.evalExpr(n.Right, env)
		if err != nil {
			return nil, err
		}
		return boolVal(right.Truthy()), nil
	}
	if n.Op == "||" {
		left, err := interp.evalExpr(n.Left, env)
		if err != nil {
			return nil, err
		}
		if left.Truthy() {
			return boolVal(true), nil
		}
		right, err := interp.evalExpr(n.Right, env)
		if err != nil {
			return nil, err
		}
		return boolVal(right.Truthy()), nil
	}
	left, err := interp.evalExpr(n.Left, env)
	if err != nil {
		return nil, err
	}
	right, err := interp.evalExpr(n.Right, env)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case "+":
		return interp.add(left, right), nil
	case "-":
		return interp.sub(left, right), nil
	case "*":
		return interp.mul(left, right), nil
	case "/":
		return interp.div(left, right)
	case "%":
		return interp.mod(left, right)
	case "==":
		return boolVal(interp.equals(left, right)), nil
	case "!=":
		return boolVal(!interp.equals(left, right)), nil
	case "<":
		return boolVal(interp.compare(left, right) < 0), nil
	case ">":
		return boolVal(interp.compare(left, right) > 0), nil
	case "<=":
		return boolVal(interp.compare(left, right) <= 0), nil
	case ">=":
		return boolVal(interp.compare(left, right) >= 0), nil
	}
	return nilVal(), nil
}

func (interp *Interpreter) evalUnary(n *ast.UnaryExpr, env *Env) (*Value, error) {
	v, err := interp.evalExpr(n.Operand, env)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case "!":
		return boolVal(!v.Truthy()), nil
	case "-":
		if v.Kind == KindInt {
			return intVal(-v.IntVal), nil
		}
		if v.Kind == KindFloat {
			return fltVal(-v.FltVal), nil
		}
	}
	return nilVal(), nil
}

func (interp *Interpreter) evalPostfix(n *ast.PostfixExpr, env *Env) (*Value, error) {
	v, err := interp.evalExpr(n.Operand, env)
	if err != nil {
		return nil, err
	}
	var newVal *Value
	switch n.Op {
	case "++":
		if v.Kind == KindFloat {
			newVal = fltVal(v.FltVal + 1)
		} else {
			newVal = intVal(v.IntVal + 1)
		}
	case "--":
		if v.Kind == KindFloat {
			newVal = fltVal(v.FltVal - 1)
		} else {
			newVal = intVal(v.IntVal - 1)
		}
	default:
		return v, nil
	}
	assignErr := interp.doAssign(n.Operand, "=", newVal, env, n.NodePos)
	if assignErr != nil {
		return nil, assignErr
	}
	return v, nil
}

func (interp *Interpreter) evalCall(n *ast.CallExpr, env *Env) (*Value, error) {
	callee, err := interp.evalExpr(n.Callee, env)
	if err != nil {
		return nil, fmt.Errorf("%s%v", posStr(n.NodePos), err)
	}
	args := make([]*Value, len(n.Args))
	for i, arg := range n.Args {
		v, err := interp.evalExpr(arg, env)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	result, callErr := interp.callFunc(callee, args)
	if callErr != nil {
		return nil, fmt.Errorf("%s%v", posStr(n.NodePos), callErr)
	}
	return result, nil
}

func (interp *Interpreter) callFunc(fn *Value, args []*Value) (*Value, error) {
	if fn.Kind == KindBuiltin {
		return fn.BuiltFn(args)
	}
	if fn.Kind == KindFunc {
		fnEnv := NewEnv(fn.FnVal.Env)
		for i, param := range fn.FnVal.Params {
			if i < len(args) {
				fnEnv.Set(param.Name, args[i])
			} else {
				fnEnv.Set(param.Name, nilVal())
			}
		}
		_, err := interp.execBlock(fn.FnVal.Body, fnEnv)
		if err != nil {
			if ret, ok := err.(*returnSignal); ok {
				if len(ret.values) == 0 {
					return nilVal(), nil
				}
				if len(ret.values) == 1 {
					return ret.values[0], nil
				}
				return arrVal(ret.values), nil
			}
			return nil, err
		}
		return nilVal(), nil
	}
	return nil, fmt.Errorf("cannot call non-function value (got %s)", kindName(fn.Kind))
}

func kindName(k ValueKind) string {
	switch k {
	case KindNil:
		return "nil"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	case KindArray:
		return "array"
	case KindMap:
		return "map"
	case KindStruct:
		return "struct"
	}
	return "unknown"
}

func (interp *Interpreter) evalMember(n *ast.MemberExpr, env *Env) (*Value, error) {
	obj, err := interp.evalExpr(n.Object, env)
	if err != nil {
		return nil, err
	}
	if obj.Kind == KindArray {
		switch n.Member {
		case "length", "len":
			return intVal(int64(len(obj.ArrVal))), nil
		case "push":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				obj.ArrVal = append(obj.ArrVal, args...)
				return nilVal(), nil
			}}, nil
		case "pop":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				if len(obj.ArrVal) == 0 {
					return nilVal(), nil
				}
				last := obj.ArrVal[len(obj.ArrVal)-1]
				obj.ArrVal = obj.ArrVal[:len(obj.ArrVal)-1]
				return last, nil
			}}, nil
		case "join":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				sep := ""
				if len(args) > 0 {
					sep = args[0].String()
				}
				parts := make([]string, len(obj.ArrVal))
				for i, el := range obj.ArrVal {
					parts[i] = el.String()
				}
				return strVal(strings.Join(parts, sep)), nil
			}}, nil
		case "slice":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				start, end := int64(0), int64(len(obj.ArrVal))
				if len(args) > 0 {
					start = args[0].IntVal
				}
				if len(args) > 1 {
					end = args[1].IntVal
				}
				if start < 0 {
					start = 0
				}
				if end > int64(len(obj.ArrVal)) {
					end = int64(len(obj.ArrVal))
				}
				return arrVal(obj.ArrVal[start:end]), nil
			}}, nil
		}
	}
	if obj.Kind == KindString {
		switch n.Member {
		case "length", "len":
			return intVal(int64(len(obj.StrVal))), nil
		case "upper":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				return strVal(strings.ToUpper(obj.StrVal)), nil
			}}, nil
		case "lower":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				return strVal(strings.ToLower(obj.StrVal)), nil
			}}, nil
		case "contains":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				if len(args) == 0 {
					return boolVal(false), nil
				}
				return boolVal(strings.Contains(obj.StrVal, args[0].String())), nil
			}}, nil
		case "split":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				sep := ""
				if len(args) > 0 {
					sep = args[0].String()
				}
				parts := strings.Split(obj.StrVal, sep)
				vals := make([]*Value, len(parts))
				for i, p2 := range parts {
					vals[i] = strVal(p2)
				}
				return arrVal(vals), nil
			}}, nil
		case "trim":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				return strVal(strings.TrimSpace(obj.StrVal)), nil
			}}, nil
		case "replace":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				if len(args) < 2 {
					return strVal(obj.StrVal), nil
				}
				return strVal(strings.ReplaceAll(obj.StrVal, args[0].String(), args[1].String())), nil
			}}, nil
		case "startsWith":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				if len(args) == 0 {
					return boolVal(false), nil
				}
				return boolVal(strings.HasPrefix(obj.StrVal, args[0].String())), nil
			}}, nil
		case "endsWith":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				if len(args) == 0 {
					return boolVal(false), nil
				}
				return boolVal(strings.HasSuffix(obj.StrVal, args[0].String())), nil
			}}, nil
		case "indexOf":
			return &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
				if len(args) == 0 {
					return intVal(-1), nil
				}
				return intVal(int64(strings.Index(obj.StrVal, args[0].String()))), nil
			}}, nil
		}
	}
	if obj.Kind == KindMap || obj.Kind == KindStruct {
		if v, ok := obj.MapVal[n.Member]; ok {
			return v, nil
		}
		return nilVal(), nil
	}
	return nil, fmt.Errorf("%sno member %q on %s value", posStr(n.NodePos), n.Member, kindName(obj.Kind))
}

func (interp *Interpreter) evalIndex(n *ast.IndexExpr, env *Env) (*Value, error) {
	obj, err := interp.evalExpr(n.Object, env)
	if err != nil {
		return nil, err
	}
	idx, err := interp.evalExpr(n.Index, env)
	if err != nil {
		return nil, err
	}
	if obj.Kind == KindArray {
		i := idx.IntVal
		if i < 0 || i >= int64(len(obj.ArrVal)) {
			return nilVal(), nil
		}
		return obj.ArrVal[i], nil
	}
	if obj.Kind == KindMap || obj.Kind == KindStruct {
		k := idx.String()
		if v, ok := obj.MapVal[k]; ok {
			return v, nil
		}
		return nilVal(), nil
	}
	if obj.Kind == KindString {
		i := idx.IntVal
		if i < 0 || i >= int64(len(obj.StrVal)) {
			return nilVal(), nil
		}
		return strVal(string(obj.StrVal[i])), nil
	}
	return nilVal(), nil
}

func toFloat(v *Value) float64 {
	if v.Kind == KindInt {
		return float64(v.IntVal)
	}
	return v.FltVal
}

func (interp *Interpreter) add(l, r *Value) *Value {
	if l.Kind == KindString || r.Kind == KindString {
		return strVal(l.String() + r.String())
	}
	if l.Kind == KindFloat || r.Kind == KindFloat {
		return fltVal(toFloat(l) + toFloat(r))
	}
	return intVal(l.IntVal + r.IntVal)
}

func (interp *Interpreter) sub(l, r *Value) *Value {
	if l.Kind == KindFloat || r.Kind == KindFloat {
		return fltVal(toFloat(l) - toFloat(r))
	}
	return intVal(l.IntVal - r.IntVal)
}

func (interp *Interpreter) mul(l, r *Value) *Value {
	if l.Kind == KindFloat || r.Kind == KindFloat {
		return fltVal(toFloat(l) * toFloat(r))
	}
	return intVal(l.IntVal * r.IntVal)
}

func (interp *Interpreter) div(l, r *Value) (*Value, error) {
	if l.Kind == KindFloat || r.Kind == KindFloat {
		if toFloat(r) == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return fltVal(toFloat(l) / toFloat(r)), nil
	}
	if r.IntVal == 0 {
		return nil, fmt.Errorf("division by zero")
	}
	return intVal(l.IntVal / r.IntVal), nil
}

func (interp *Interpreter) mod(l, r *Value) (*Value, error) {
	if r.IntVal == 0 {
		return nil, fmt.Errorf("modulo by zero")
	}
	return intVal(l.IntVal % r.IntVal), nil
}

func (interp *Interpreter) equals(l, r *Value) bool {
	if l.Kind != r.Kind {
		if (l.Kind == KindInt || l.Kind == KindFloat) && (r.Kind == KindInt || r.Kind == KindFloat) {
			return toFloat(l) == toFloat(r)
		}
		return false
	}
	switch l.Kind {
	case KindNil:
		return true
	case KindBool:
		return l.BoolVal == r.BoolVal
	case KindInt:
		return l.IntVal == r.IntVal
	case KindFloat:
		return l.FltVal == r.FltVal
	case KindString:
		return l.StrVal == r.StrVal
	}
	return false
}

func (interp *Interpreter) compare(l, r *Value) int {
	if l.Kind == KindString && r.Kind == KindString {
		if l.StrVal < r.StrVal {
			return -1
		}
		if l.StrVal > r.StrVal {
			return 1
		}
		return 0
	}
	lf, rf := toFloat(l), toFloat(r)
	if lf < rf {
		return -1
	}
	if lf > rf {
		return 1
	}
	return 0
}

func (interp *Interpreter) registerStdlib() {
	env := interp.global

	env.Set("print", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = a.String()
		}
		fmt.Print(strings.Join(parts, " "))
		return nilVal(), nil
	}})

	env.Set("println", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = a.String()
		}
		fmt.Println(strings.Join(parts, " "))
		return nilVal(), nil
	}})

	env.Set("printf", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return nilVal(), nil
		}
		format := args[0].StrVal
		ifaces := make([]interface{}, len(args)-1)
		for i, a := range args[1:] {
			ifaces[i] = a.String()
		}
		fmt.Printf(format, ifaces...)
		return nilVal(), nil
	}})

	env.Set("sprintf", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return strVal(""), nil
		}
		format := args[0].StrVal
		ifaces := make([]interface{}, len(args)-1)
		for i, a := range args[1:] {
			ifaces[i] = a.String()
		}
		return strVal(fmt.Sprintf(format, ifaces...)), nil
	}})

	env.Set("input", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) > 0 {
			fmt.Print(args[0].String())
		}
		var line string
		fmt.Scanln(&line)
		return strVal(line), nil
	}})

	env.Set("readFile", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return strVal(""), fmt.Errorf("readFile: path required")
		}
		data, err := os.ReadFile(args[0].StrVal)
		if err != nil {
			return strVal(""), err
		}
		return strVal(string(data)), nil
	}})

	env.Set("writeFile", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) < 2 {
			return nilVal(), fmt.Errorf("writeFile: path and content required")
		}
		err := os.WriteFile(args[0].StrVal, []byte(args[1].StrVal), 0644)
		if err != nil {
			return nilVal(), err
		}
		return nilVal(), nil
	}})

	env.Set("args", &Value{Kind: KindBuiltin, BuiltFn: func(fnArgs []*Value) (*Value, error) {
		osArgs := os.Args
		vals := make([]*Value, len(osArgs))
		for i, a := range osArgs {
			vals[i] = strVal(a)
		}
		return arrVal(vals), nil
	}})

	env.Set("buildFile", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) < 2 {
			return boolVal(false), fmt.Errorf("buildFile: input and output required")
		}
		input := args[0].StrVal
		output := args[1].StrVal
		cmd := exec.Command(os.Args[0], "build", input, "-o", output)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return boolVal(false), nil
		}
		return boolVal(true), nil
	}})

	env.Set("int", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return intVal(0), nil
		}
		a := args[0]
		switch a.Kind {
		case KindInt:
			return a, nil
		case KindFloat:
			return intVal(int64(a.FltVal)), nil
		case KindString:
			n, err := strconv.ParseInt(strings.TrimSpace(a.StrVal), 10, 64)
			if err != nil {
				f, err2 := strconv.ParseFloat(strings.TrimSpace(a.StrVal), 64)
				if err2 != nil {
					return intVal(0), nil
				}
				return intVal(int64(f)), nil
			}
			return intVal(n), nil
		case KindBool:
			if a.BoolVal {
				return intVal(1), nil
			}
			return intVal(0), nil
		}
		return intVal(0), nil
	}})

	env.Set("float", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return fltVal(0), nil
		}
		a := args[0]
		switch a.Kind {
		case KindFloat:
			return a, nil
		case KindInt:
			return fltVal(float64(a.IntVal)), nil
		case KindString:
			f, _ := strconv.ParseFloat(strings.TrimSpace(a.StrVal), 64)
			return fltVal(f), nil
		}
		return fltVal(0), nil
	}})

	env.Set("string", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return strVal(""), nil
		}
		return strVal(args[0].String()), nil
	}})

	env.Set("bool", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return boolVal(false), nil
		}
		return boolVal(args[0].Truthy()), nil
	}})

	env.Set("len", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return intVal(0), nil
		}
		switch args[0].Kind {
		case KindArray:
			return intVal(int64(len(args[0].ArrVal))), nil
		case KindString:
			return intVal(int64(len(args[0].StrVal))), nil
		case KindMap, KindStruct:
			return intVal(int64(len(args[0].MapVal))), nil
		}
		return intVal(0), nil
	}})

	env.Set("values", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 || (args[0].Kind != KindMap && args[0].Kind != KindStruct) {
			return arrVal(nil), nil
		}
		vals := make([]*Value, 0, len(args[0].MapKeys))
		for _, k := range args[0].MapKeys {
			vals = append(vals, args[0].MapVal[k])
		}
		return arrVal(vals), nil
	}})

	env.Set("push", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) < 2 || args[0].Kind != KindArray {
			return intVal(0), fmt.Errorf("push: first argument must be array")
		}
		args[0].ArrVal = append(args[0].ArrVal, args[1:]...)
		return intVal(int64(len(args[0].ArrVal))), nil
	}})

	env.Set("pop", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 || args[0].Kind != KindArray || len(args[0].ArrVal) == 0 {
			return nilVal(), nil
		}
		last := args[0].ArrVal[len(args[0].ArrVal)-1]
		args[0].ArrVal = args[0].ArrVal[:len(args[0].ArrVal)-1]
		return last, nil
	}})

	env.Set("contains", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) < 2 {
			return boolVal(false), nil
		}
		if args[0].Kind == KindString {
			return boolVal(strings.Contains(args[0].StrVal, args[1].String())), nil
		}
		if args[0].Kind == KindArray {
			for _, v := range args[0].ArrVal {
				if interp.equals(v, args[1]) {
					return boolVal(true), nil
				}
			}
		}
		if args[0].Kind == KindMap || args[0].Kind == KindStruct {
			_, ok := args[0].MapVal[args[1].String()]
			return boolVal(ok), nil
		}
		return boolVal(false), nil
	}})

	env.Set("indexOf", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) < 2 {
			return intVal(-1), nil
		}
		if args[0].Kind == KindString {
			return intVal(int64(strings.Index(args[0].StrVal, args[1].String()))), nil
		}
		if args[0].Kind == KindArray {
			for i, v := range args[0].ArrVal {
				if interp.equals(v, args[1]) {
					return intVal(int64(i)), nil
				}
			}
		}
		return intVal(-1), nil
	}})

	env.Set("split", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return arrVal(nil), nil
		}
		sep := " "
		if len(args) > 1 {
			sep = args[1].String()
		}
		parts := strings.Split(args[0].String(), sep)
		out := make([]*Value, len(parts))
		for i, p := range parts {
			out[i] = strVal(p)
		}
		return arrVal(out), nil
	}})

	env.Set("join", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 || args[0].Kind != KindArray {
			return strVal(""), nil
		}
		sep := ""
		if len(args) > 1 {
			sep = args[1].String()
		}
		parts := make([]string, len(args[0].ArrVal))
		for i, v := range args[0].ArrVal {
			parts[i] = v.String()
		}
		return strVal(strings.Join(parts, sep)), nil
	}})

	env.Set("trim", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return strVal(""), nil
		}
		return strVal(strings.TrimSpace(args[0].String())), nil
	}})

	env.Set("upper", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return strVal(""), nil
		}
		return strVal(strings.ToUpper(args[0].String())), nil
	}})

	env.Set("lower", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return strVal(""), nil
		}
		return strVal(strings.ToLower(args[0].String())), nil
	}})

	if fn, ok := env.Get("int"); ok {
		env.Set("parseInt", fn)
	}
	if fn, ok := env.Get("float"); ok {
		env.Set("parseFloat", fn)
		env.Set("Number", fn)
	}
	if fn, ok := env.Get("string"); ok {
		env.Set("String", fn)
	}
	if fn, ok := env.Get("bool"); ok {
		env.Set("Boolean", fn)
	}
	env.Set("now", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		return intVal(time.Now().UnixMilli()), nil
	}})

	env.Set("append", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) < 2 {
			return nilVal(), nil
		}
		arr := args[0]
		if arr.Kind != KindArray {
			return nilVal(), fmt.Errorf("append: first argument must be array")
		}
		newArr := make([]*Value, len(arr.ArrVal), len(arr.ArrVal)+len(args)-1)
		copy(newArr, arr.ArrVal)
		newArr = append(newArr, args[1:]...)
		return arrVal(newArr), nil
	}})

	env.Set("delete", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) < 2 || (args[0].Kind != KindMap && args[0].Kind != KindStruct) {
			return nilVal(), nil
		}
		key := args[1].String()
		delete(args[0].MapVal, key)
		newKeys := args[0].MapKeys[:0]
		for _, k := range args[0].MapKeys {
			if k != key {
				newKeys = append(newKeys, k)
			}
		}
		args[0].MapKeys = newKeys
		return nilVal(), nil
	}})

	env.Set("keys", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 || (args[0].Kind != KindMap && args[0].Kind != KindStruct) {
			return arrVal(nil), nil
		}
		vals := make([]*Value, len(args[0].MapKeys))
		for i, k := range args[0].MapKeys {
			vals[i] = strVal(k)
		}
		return arrVal(vals), nil
	}})

	env.Set("math", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"sqrt": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Sqrt(toFloat(args[0]))), nil
		}},
		"abs": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Abs(toFloat(args[0]))), nil
		}},
		"pow": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return fltVal(0), nil
			}
			return fltVal(math.Pow(toFloat(args[0]), toFloat(args[1]))), nil
		}},
		"floor": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Floor(toFloat(args[0]))), nil
		}},
		"ceil": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Ceil(toFloat(args[0]))), nil
		}},
		"round": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Round(toFloat(args[0]))), nil
		}},
		"log": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Log(toFloat(args[0]))), nil
		}},
		"sin": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Sin(toFloat(args[0]))), nil
		}},
		"cos": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Cos(toFloat(args[0]))), nil
		}},
		"tan": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			return fltVal(math.Tan(toFloat(args[0]))), nil
		}},
		"min": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return fltVal(0), nil
			}
			return fltVal(math.Min(toFloat(args[0]), toFloat(args[1]))), nil
		}},
		"max": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return fltVal(0), nil
			}
			return fltVal(math.Max(toFloat(args[0]), toFloat(args[1]))), nil
		}},
		"pi": fltVal(math.Pi),
		"e":  fltVal(math.E),
	}, MapKeys: []string{"sqrt", "abs", "pow", "floor", "ceil", "round", "log", "sin", "cos", "tan", "min", "max", "pi", "e"}})

	env.Set("typeof", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		if len(args) == 0 {
			return strVal("nil"), nil
		}
		switch args[0].Kind {
		case KindNil:
			return strVal("nil"), nil
		case KindInt:
			return strVal("int"), nil
		case KindFloat:
			return strVal("float"), nil
		case KindString:
			return strVal("string"), nil
		case KindBool:
			return strVal("bool"), nil
		case KindArray:
			return strVal("array"), nil
		case KindMap:
			return strVal("map"), nil
		case KindStruct:
			name := args[0].StructType
			if name == "" {
				name = "struct"
			}
			return strVal(name), nil
		case KindFunc, KindBuiltin:
			return strVal("func"), nil
		}
		return strVal("unknown"), nil
	}})

	env.Set("exit", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		code := 0
		if len(args) > 0 {
			code = int(args[0].IntVal)
		}
		os.Exit(code)
		return nilVal(), nil
	}})

	env.Set("panic", &Value{Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
		msg := "panic"
		if len(args) > 0 {
			msg = args[0].String()
		}
		return nil, fmt.Errorf("panic: %s", msg)
	}})

	env.Set("http", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"get": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return nilVal(), fmt.Errorf("http.get: url required")
			}
			req, err := http.NewRequest(http.MethodGet, args[0].String(), nil)
			if err != nil {
				return nilVal(), err
			}
			return runHTTP(req, 30000)
		}},
		"post": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return nilVal(), fmt.Errorf("http.post: url and body required")
			}
			contentType := "application/json"
			if len(args) > 2 {
				contentType = args[2].String()
			}
			req, err := http.NewRequest(http.MethodPost, args[0].String(), bytes.NewBufferString(args[1].String()))
			if err != nil {
				return nilVal(), err
			}
			req.Header.Set("Content-Type", contentType)
			return runHTTP(req, 30000)
		}},
		"request": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return nilVal(), fmt.Errorf("http.request: method and url required")
			}
			body := ""
			if len(args) > 2 {
				body = args[2].String()
			}
			timeout := int64(30000)
			if len(args) > 3 {
				timeout = args[3].IntVal
			}
			req, err := http.NewRequest(strings.ToUpper(args[0].String()), args[1].String(), bytes.NewBufferString(body))
			if err != nil {
				return nilVal(), err
			}
			if len(args) > 4 && (args[4].Kind == KindMap || args[4].Kind == KindStruct) {
				for _, k := range args[4].MapKeys {
					req.Header.Set(k, args[4].MapVal[k].String())
				}
			}
			return runHTTP(req, timeout)
		}},
		"encodeQuery": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 || (args[0].Kind != KindMap && args[0].Kind != KindStruct) {
				return strVal(""), nil
			}
			q := neturl.Values{}
			for _, k := range args[0].MapKeys {
				q.Set(k, args[0].MapVal[k].String())
			}
			return strVal(q.Encode()), nil
		}},
	}, MapKeys: []string{"get", "post", "request", "encodeQuery"}})

	env.Set("json", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"stringify": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal("null"), nil
			}
			b, err := json.Marshal(valueToAny(args[0]))
			if err != nil {
				return strVal(""), err
			}
			return strVal(string(b)), nil
		}},
		"parse": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return nilVal(), nil
			}
			var v interface{}
			if err := json.Unmarshal([]byte(args[0].String()), &v); err != nil {
				return nilVal(), err
			}
			return anyToValue(v), nil
		}},
	}, MapKeys: []string{"stringify", "parse"}})

	env.Set("strings", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"contains": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return boolVal(false), nil
			}
			return boolVal(strings.Contains(args[0].String(), args[1].String())), nil
		}},
		"split": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return arrVal(nil), nil
			}
			parts := strings.Split(args[0].String(), args[1].String())
			out := make([]*Value, len(parts))
			for i, p := range parts {
				out[i] = strVal(p)
			}
			return arrVal(out), nil
		}},
		"join": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 || args[0].Kind != KindArray {
				return strVal(""), nil
			}
			parts := make([]string, len(args[0].ArrVal))
			for i, v := range args[0].ArrVal {
				parts[i] = v.String()
			}
			return strVal(strings.Join(parts, args[1].String())), nil
		}},
		"trimSpace": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			return strVal(strings.TrimSpace(args[0].String())), nil
		}},
		"toUpper": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			return strVal(strings.ToUpper(args[0].String())), nil
		}},
		"toLower": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			return strVal(strings.ToLower(args[0].String())), nil
		}},
	}, MapKeys: []string{"contains", "split", "join", "trimSpace", "toUpper", "toLower"}})

	env.Set("strconv", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"atoi": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return intVal(0), nil
			}
			n, err := strconv.ParseInt(strings.TrimSpace(args[0].String()), 10, 64)
			if err != nil {
				return intVal(0), err
			}
			return intVal(n), nil
		}},
		"itoa": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal("0"), nil
			}
			return strVal(strconv.FormatInt(args[0].IntVal, 10)), nil
		}},
		"parseFloat": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return fltVal(0), nil
			}
			f, err := strconv.ParseFloat(strings.TrimSpace(args[0].String()), 64)
			if err != nil {
				return fltVal(0), err
			}
			return fltVal(f), nil
		}},
		"formatFloat": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal("0"), nil
			}
			return strVal(strconv.FormatFloat(toFloat(args[0]), 'f', -1, 64)), nil
		}},
	}, MapKeys: []string{"atoi", "itoa", "parseFloat", "formatFloat"}})

	env.Set("os", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"getenv": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			return strVal(os.Getenv(args[0].String())), nil
		}},
		"setenv": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return nilVal(), fmt.Errorf("os.setenv: key and value required")
			}
			return nilVal(), os.Setenv(args[0].String(), args[1].String())
		}},
		"readFile": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), fmt.Errorf("os.readFile: path required")
			}
			data, err := os.ReadFile(args[0].String())
			if err != nil {
				return strVal(""), err
			}
			return strVal(string(data)), nil
		}},
		"writeFile": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) < 2 {
				return nilVal(), fmt.Errorf("os.writeFile: path and content required")
			}
			return nilVal(), os.WriteFile(args[0].String(), []byte(args[1].String()), 0644)
		}},
		"mkdirAll": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return nilVal(), fmt.Errorf("os.mkdirAll: path required")
			}
			return nilVal(), os.MkdirAll(args[0].String(), 0755)
		}},
		"remove": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return nilVal(), fmt.Errorf("os.remove: path required")
			}
			return nilVal(), os.RemoveAll(args[0].String())
		}},
	}, MapKeys: []string{"getenv", "setenv", "readFile", "writeFile", "mkdirAll", "remove"}})

	env.Set("time", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"nowUnix": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			return intVal(time.Now().Unix()), nil
		}},
		"nowUnixMilli": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			return intVal(time.Now().UnixMilli()), nil
		}},
		"sleepMs": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) > 0 {
				time.Sleep(time.Duration(args[0].IntVal) * time.Millisecond)
			}
			return nilVal(), nil
		}},
	}, MapKeys: []string{"nowUnix", "nowUnixMilli", "sleepMs"}})

	env.Set("rand", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"intn": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			max := int64(100)
			if len(args) > 0 {
				max = args[0].IntVal
			}
			if max <= 0 {
				return intVal(0), nil
			}
			return intVal(mrand.Int63n(max)), nil
		}},
		"float": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			return fltVal(mrand.Float64()), nil
		}},
		"seed": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			seed := time.Now().UnixNano()
			if len(args) > 0 {
				seed = args[0].IntVal
			}
			mrand.Seed(seed)
			return nilVal(), nil
		}},
	}, MapKeys: []string{"intn", "float", "seed"}})

	env.Set("path", &Value{Kind: KindMap, MapVal: map[string]*Value{
		"join": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			parts := make([]string, len(args))
			for i, arg := range args {
				parts[i] = arg.String()
			}
			return strVal(filepath.Join(parts...)), nil
		}},
		"base": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			return strVal(filepath.Base(args[0].String())), nil
		}},
		"dir": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			return strVal(filepath.Dir(args[0].String())), nil
		}},
		"ext": {Kind: KindBuiltin, BuiltFn: func(args []*Value) (*Value, error) {
			if len(args) == 0 {
				return strVal(""), nil
			}
			return strVal(filepath.Ext(args[0].String())), nil
		}},
	}, MapKeys: []string{"join", "base", "dir", "ext"}})
}
