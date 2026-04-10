package interpreter

type Env struct {
	vars   map[string]*Value
	consts map[string]bool
	parent *Env
}

func NewEnv(parent *Env) *Env {
	return &Env{vars: make(map[string]*Value), consts: make(map[string]bool), parent: parent}
}

func (e *Env) Get(name string) (*Value, bool) {
	if v, ok := e.vars[name]; ok {
		return v, true
	}
	if e.parent != nil {
		return e.parent.Get(name)
	}
	return nil, false
}

func (e *Env) Set(name string, v *Value) {
	e.vars[name] = v
}

func (e *Env) SetConst(name string, v *Value) {
	e.vars[name] = v
	e.consts[name] = true
}

func (e *Env) IsConst(name string) bool {
	if e.consts[name] {
		return true
	}
	if e.parent != nil {
		return e.parent.IsConst(name)
	}
	return false
}

func (e *Env) Assign(name string, v *Value) bool {
	if _, ok := e.vars[name]; ok {
		e.vars[name] = v
		return true
	}
	if e.parent != nil {
		return e.parent.Assign(name, v)
	}
	return false
}
