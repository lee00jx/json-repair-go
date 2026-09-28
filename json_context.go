package json_repair

// ContextValue models the Python ContextValues enum: which object/array role
// the parser is currently inside. It drives the heuristic branches in the
// string/object/array parsers.
type ContextValue int

const (
	ContextObjectKey ContextValue = iota
	ContextObjectValue
	ContextArray
)

// JsonContext is a stack of ContextValue. It mirrors the Python JsonContext:
// `current` tracks the innermost context, `empty` is true when the stack is
// empty. Parsers enter/leave contexts around keys, values and arrays.
type JsonContext struct {
	stack   []ContextValue
	current ContextValue
	empty   bool
}

func newJsonContext() *JsonContext {
	return &JsonContext{empty: true}
}

func (c *JsonContext) set(value ContextValue) {
	c.stack = append(c.stack, value)
	c.current = value
	c.empty = false
}

func (c *JsonContext) reset() {
	n := len(c.stack)
	switch {
	case n == 0:
		return
	case n == 1:
		// popping the last element empties the stack
		c.stack = nil
		c.current = 0
		c.empty = true
	default:
		c.current = c.stack[n-2]
		c.stack = c.stack[:n-1]
	}
}

func (c *JsonContext) clear() {
	c.stack = nil
	c.current = 0
	c.empty = true
}

// enter sets the context and returns a closer that restores the previous one.
// Use with defer: `defer p.context.enter(ContextArray)()`.
func (c *JsonContext) enter(value ContextValue) func() {
	c.set(value)
	return c.reset
}

// enterValues pushes every value and returns a single closer that pops them all,
// mirroring Python's `ExitStack` + `context.enter(...)` used to replay
// deferred contexts.
func (c *JsonContext) enterValues(values []ContextValue) func() {
	for _, v := range values {
		c.set(v)
	}
	return func() {
		for range values {
			c.reset()
		}
	}
}

// currentIs reports whether the innermost context equals value.
// Returns false when the context is empty (mirrors `current is None`).
func (c *JsonContext) currentIs(value ContextValue) bool {
	return !c.empty && c.current == value
}

// in reports whether value appears anywhere in the context stack.
func (c *JsonContext) in(value ContextValue) bool {
	for _, v := range c.stack {
		if v == value {
			return true
		}
	}
	return false
}
