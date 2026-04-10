package IR

import (
	"fmt"
	"strings"
)

type Opcode int

const (
	OpNop Opcode = iota

	OpConst
	OpNil
	OpTrue
	OpFalse

	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpNeg
	OpNot

	OpEq
	OpNeq
	OpLt
	OpLte
	OpGt
	OpGte
	OpAnd
	OpOr

	OpLoad
	OpStore
	OpStoreIndex
	OpStoreMember
	OpLoadIndex
	OpLoadMember

	OpMakeArray
	OpArrayPush
	OpMakeMap
	OpMapSet

	OpMakeStruct
	OpStructSet

	OpCall
	OpCallBuiltin
	OpCallMethod
	OpReturn

	OpJump
	OpJumpIfFalse
	OpJumpIfTrue

	OpLabel

	OpInc
	OpDec

	OpFuncDef
	OpFuncEnd

	OpPop
	OpDup
)

func (op Opcode) String() string {
	switch op {
	case OpNop:
		return "NOP"
	case OpConst:
		return "CONST"
	case OpNil:
		return "NIL"
	case OpTrue:
		return "TRUE"
	case OpFalse:
		return "FALSE"
	case OpAdd:
		return "ADD"
	case OpSub:
		return "SUB"
	case OpMul:
		return "MUL"
	case OpDiv:
		return "DIV"
	case OpMod:
		return "MOD"
	case OpNeg:
		return "NEG"
	case OpNot:
		return "NOT"
	case OpEq:
		return "EQ"
	case OpNeq:
		return "NEQ"
	case OpLt:
		return "LT"
	case OpLte:
		return "LTE"
	case OpGt:
		return "GT"
	case OpGte:
		return "GTE"
	case OpAnd:
		return "AND"
	case OpOr:
		return "OR"
	case OpLoad:
		return "LOAD"
	case OpStore:
		return "STORE"
	case OpStoreIndex:
		return "STORE_INDEX"
	case OpStoreMember:
		return "STORE_MEMBER"
	case OpLoadIndex:
		return "LOAD_INDEX"
	case OpLoadMember:
		return "LOAD_MEMBER"
	case OpMakeArray:
		return "MAKE_ARRAY"
	case OpArrayPush:
		return "ARRAY_PUSH"
	case OpMakeMap:
		return "MAKE_MAP"
	case OpMapSet:
		return "MAP_SET"
	case OpMakeStruct:
		return "MAKE_STRUCT"
	case OpStructSet:
		return "STRUCT_SET"
	case OpCall:
		return "CALL"
	case OpCallBuiltin:
		return "CALL_BUILTIN"
	case OpCallMethod:
		return "CALL_METHOD"
	case OpReturn:
		return "RETURN"
	case OpJump:
		return "JUMP"
	case OpJumpIfFalse:
		return "JUMP_IF_FALSE"
	case OpJumpIfTrue:
		return "JUMP_IF_TRUE"
	case OpLabel:
		return "LABEL"
	case OpInc:
		return "INC"
	case OpDec:
		return "DEC"
	case OpFuncDef:
		return "FUNC_DEF"
	case OpFuncEnd:
		return "FUNC_END"
	case OpPop:
		return "POP"
	case OpDup:
		return "DUP"
	}
	return fmt.Sprintf("OP(%d)", int(op))
}

type Instr struct {
	Op      Opcode
	Operand interface{}
	Comment string
	Line    int
	Col     int
}

func (ins *Instr) String() string {
	s := ins.Op.String()
	if ins.Operand != nil {
		s += fmt.Sprintf(" %v", ins.Operand)
	}
	if ins.Comment != "" {
		s += "  ; " + ins.Comment
	}
	return s
}

type Function struct {
	Name    string
	Params  []string
	Instrs  []*Instr
	Locals  map[string]int
	nLabels int
}

func NewFunction(name string, params []string) *Function {
	return &Function{
		Name:   name,
		Params: params,
		Locals: make(map[string]int),
	}
}

func (f *Function) Emit(op Opcode, operand interface{}, comment string, line, col int) *Instr {
	ins := &Instr{Op: op, Operand: operand, Comment: comment, Line: line, Col: col}
	f.Instrs = append(f.Instrs, ins)
	return ins
}

func (f *Function) NewLabel() string {
	f.nLabels++
	return fmt.Sprintf(".L%d_%s", f.nLabels, sanitize(f.Name))
}

func (f *Function) EmitLabel(label string) {
	f.Emit(OpLabel, label, "", 0, 0)
}

func (f *Function) Dump() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("func %s(%s):\n", f.Name, strings.Join(f.Params, ", ")))
	for i, ins := range f.Instrs {
		if ins.Op == OpLabel {
			sb.WriteString(fmt.Sprintf("  %s:\n", ins.Operand))
		} else {
			loc := ""
			if ins.Line > 0 {
				loc = fmt.Sprintf("%-6s", fmt.Sprintf("%d:%d", ins.Line, ins.Col))
			} else {
				loc = "      "
			}
			sb.WriteString(fmt.Sprintf("  %4d  %s  %s\n", i, loc, ins.String()))
		}
	}
	return sb.String()
}

type Module struct {
	Functions []*Function
	funcIndex map[string]*Function
}

func NewModule() *Module {
	return &Module{funcIndex: make(map[string]*Function)}
}

func (m *Module) AddFunction(f *Function) {
	m.Functions = append(m.Functions, f)
	m.funcIndex[f.Name] = f
}

func (m *Module) GetFunction(name string) (*Function, bool) {
	f, ok := m.funcIndex[name]
	return f, ok
}

func (m *Module) Dump() string {
	var sb strings.Builder
	sb.WriteString("; gi IR module\n\n")
	for _, f := range m.Functions {
		sb.WriteString(f.Dump())
		sb.WriteString("\n")
	}
	return sb.String()
}

func sanitize(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	if sb.Len() == 0 {
		return "anon"
	}
	return sb.String()
}
