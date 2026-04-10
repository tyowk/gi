package ast

type Pos struct {
	Line int
	Col  int
}

type Node interface {
	nodeType() string
	Position() Pos
}

type (
	Program struct {
		Statements []Node
		NodePos    Pos
	}

	ImportStmt struct {
		Names   []string
		Path    string
		NodePos Pos
		Aliases []string
	}

	FuncDecl struct {
		Name    string
		Params  []Param
		Returns []string
		Body    *Block
		NodePos Pos
	}

	Param struct {
		Name     string
		Type     string
		Default  Node
		Required bool
		NodePos  Pos
	}

	VarDecl struct {
		Name    string
		Value   Node
		Short   bool
		IsConst bool
		NodePos Pos
	}

	MultiVarDecl struct {
		Names   []string
		Value   Node
		Short   bool
		NodePos Pos
	}

	AssignStmt struct {
		Target  Node
		Op      string
		Value   Node
		NodePos Pos
	}

	ReturnStmt struct {
		Values  []Node
		NodePos Pos
	}

	BreakStmt struct {
		NodePos Pos
	}

	ContinueStmt struct {
		NodePos Pos
	}

	IfStmt struct {
		Condition Node
		Then      *Block
		Else      Node
		NodePos   Pos
	}

	ForStmt struct {
		Init      Node
		Condition Node
		Post      Node
		Body      *Block
		NodePos   Pos
	}

	Block struct {
		Statements []Node
		NodePos    Pos
	}

	ExprStmt struct {
		Expr    Node
		NodePos Pos
	}

	StructDecl struct {
		Name    string
		Fields  []StructField
		NodePos Pos
	}

	StructField struct {
		Name string
		Type string
	}
)

type (
	Identifier struct {
		Name    string
		NodePos Pos
	}

	IntLiteral struct {
		Value   int64
		NodePos Pos
	}

	FloatLiteral struct {
		Value   float64
		NodePos Pos
	}

	StringLiteral struct {
		Value   string
		NodePos Pos
	}

	BoolLiteral struct {
		Value   bool
		NodePos Pos
	}

	NilLiteral struct {
		NodePos Pos
	}

	ArrayLiteral struct {
		Elements []Node
		NodePos  Pos
	}

	MapLiteral struct {
		Pairs   []MapPair
		NodePos Pos
	}

	MapPair struct {
		Key   Node
		Value Node
	}

	StructLiteral struct {
		Name    string
		Fields  []StructFieldVal
		NodePos Pos
	}

	StructFieldVal struct {
		Name  string
		Value Node
	}

	BinaryExpr struct {
		Left    Node
		Op      string
		Right   Node
		NodePos Pos
	}

	UnaryExpr struct {
		Op      string
		Operand Node
		NodePos Pos
	}

	PostfixExpr struct {
		Op      string
		Operand Node
		NodePos Pos
	}

	CallExpr struct {
		Callee  Node
		Args    []Node
		NodePos Pos
	}

	IndexExpr struct {
		Object  Node
		Index   Node
		NodePos Pos
	}

	MemberExpr struct {
		Object  Node
		Member  string
		NodePos Pos
	}

	FuncLiteral struct {
		Params  []Param
		Returns []string
		Body    *Block
		NodePos Pos
	}
)

func (n *Program) nodeType() string      { return "Program" }
func (n *ImportStmt) nodeType() string   { return "ImportStmt" }
func (n *FuncDecl) nodeType() string     { return "FuncDecl" }
func (n *VarDecl) nodeType() string      { return "VarDecl" }
func (n *MultiVarDecl) nodeType() string { return "MultiVarDecl" }
func (n *AssignStmt) nodeType() string   { return "AssignStmt" }
func (n *ReturnStmt) nodeType() string   { return "ReturnStmt" }
func (n *BreakStmt) nodeType() string    { return "BreakStmt" }
func (n *ContinueStmt) nodeType() string { return "ContinueStmt" }
func (n *IfStmt) nodeType() string       { return "IfStmt" }
func (n *ForStmt) nodeType() string      { return "ForStmt" }
func (n *Block) nodeType() string        { return "Block" }
func (n *ExprStmt) nodeType() string     { return "ExprStmt" }
func (n *StructDecl) nodeType() string   { return "StructDecl" }

func (n *Identifier) nodeType() string    { return "Identifier" }
func (n *IntLiteral) nodeType() string    { return "IntLiteral" }
func (n *FloatLiteral) nodeType() string  { return "FloatLiteral" }
func (n *StringLiteral) nodeType() string { return "StringLiteral" }
func (n *BoolLiteral) nodeType() string   { return "BoolLiteral" }
func (n *NilLiteral) nodeType() string    { return "NilLiteral" }
func (n *ArrayLiteral) nodeType() string  { return "ArrayLiteral" }
func (n *MapLiteral) nodeType() string    { return "MapLiteral" }
func (n *StructLiteral) nodeType() string { return "StructLiteral" }
func (n *BinaryExpr) nodeType() string    { return "BinaryExpr" }
func (n *UnaryExpr) nodeType() string     { return "UnaryExpr" }
func (n *PostfixExpr) nodeType() string   { return "PostfixExpr" }
func (n *CallExpr) nodeType() string      { return "CallExpr" }
func (n *IndexExpr) nodeType() string     { return "IndexExpr" }
func (n *MemberExpr) nodeType() string    { return "MemberExpr" }
func (n *FuncLiteral) nodeType() string   { return "FuncLiteral" }

func (n *Program) Position() Pos      { return n.NodePos }
func (n *ImportStmt) Position() Pos   { return n.NodePos }
func (n *FuncDecl) Position() Pos     { return n.NodePos }
func (n *VarDecl) Position() Pos      { return n.NodePos }
func (n *MultiVarDecl) Position() Pos { return n.NodePos }
func (n *AssignStmt) Position() Pos   { return n.NodePos }
func (n *ReturnStmt) Position() Pos   { return n.NodePos }
func (n *BreakStmt) Position() Pos    { return n.NodePos }
func (n *ContinueStmt) Position() Pos { return n.NodePos }
func (n *IfStmt) Position() Pos       { return n.NodePos }
func (n *ForStmt) Position() Pos      { return n.NodePos }
func (n *Block) Position() Pos        { return n.NodePos }
func (n *ExprStmt) Position() Pos     { return n.NodePos }
func (n *StructDecl) Position() Pos   { return n.NodePos }

func (n *Identifier) Position() Pos    { return n.NodePos }
func (n *IntLiteral) Position() Pos    { return n.NodePos }
func (n *FloatLiteral) Position() Pos  { return n.NodePos }
func (n *StringLiteral) Position() Pos { return n.NodePos }
func (n *BoolLiteral) Position() Pos   { return n.NodePos }
func (n *NilLiteral) Position() Pos    { return n.NodePos }
func (n *ArrayLiteral) Position() Pos  { return n.NodePos }
func (n *MapLiteral) Position() Pos    { return n.NodePos }
func (n *StructLiteral) Position() Pos { return n.NodePos }
func (n *BinaryExpr) Position() Pos    { return n.NodePos }
func (n *UnaryExpr) Position() Pos     { return n.NodePos }
func (n *PostfixExpr) Position() Pos   { return n.NodePos }
func (n *CallExpr) Position() Pos      { return n.NodePos }
func (n *IndexExpr) Position() Pos     { return n.NodePos }
func (n *MemberExpr) Position() Pos    { return n.NodePos }
func (n *FuncLiteral) Position() Pos   { return n.NodePos }
