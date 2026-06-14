package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
)

// Context holds the variable store and provides expression evaluation.
type Context struct {
	vars map[string]interface{}
}

// NewContext creates an empty variable context.
func NewContext() *Context {
	return &Context{vars: make(map[string]interface{})}
}

var flowVarRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_.]*)\}`)

// Set stores a value in the context. Supports dot notation for nested paths.
func (c *Context) Set(key string, value interface{}) {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		c.vars[key] = value
		return
	}

	// Build nested map for dot-notation keys
	current := c.vars
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			next = make(map[string]interface{})
			current[part] = next
		}
		if m, ok := next.(map[string]interface{}); ok {
			current = m
		} else {
			// Can't nest further into a non-map value
			c.vars[key] = value
			return
		}
	}
	current[parts[len(parts)-1]] = value
}

// Get retrieves a value by key. Supports dot notation.
func (c *Context) Get(key string) (interface{}, bool) {
	parts := strings.Split(key, ".")
	var current interface{} = c.vars

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// All returns a copy of all variables for use in expression environments.
func (c *Context) All() map[string]interface{} {
	cp := make(map[string]interface{}, len(c.vars))
	for k, v := range c.vars {
		cp[k] = v
	}
	return cp
}

// ResolveString replaces ${var} placeholders in a string with context values.
func (c *Context) ResolveString(input string) string {
	return flowVarRegex.ReplaceAllStringFunc(input, func(match string) string {
		key := flowVarRegex.FindStringSubmatch(match)[1]
		val, ok := c.Get(key)
		if !ok {
			return match
		}
		return fmt.Sprintf("%v", val)
	})
}

// ResolveInterface recursively resolves ${var} placeholders in any value.
func (c *Context) ResolveInterface(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return c.ResolveString(val)
	case map[string]interface{}:
		resolved := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			resolved[c.ResolveString(k)] = c.ResolveInterface(v2)
		}
		return resolved
	case []interface{}:
		resolved := make([]interface{}, len(val))
		for i, v2 := range val {
			resolved[i] = c.ResolveInterface(v2)
		}
		return resolved
	default:
		return v
	}
}

// EvalBool evaluates an expression string and returns a boolean result.
func (c *Context) EvalBool(expression string) (bool, error) {
	program, err := expr.Compile(expression, expr.AsBool())
	if err != nil {
		return false, fmt.Errorf("compiling expression %q: %w", expression, err)
	}

	result, err := expr.Run(program, c.All())
	if err != nil {
		return false, fmt.Errorf("evaluating expression %q: %w", expression, err)
	}

	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expression %q did not return a boolean, got %T", expression, result)
	}
	return b, nil
}

// EvalExpr evaluates an expression and returns the raw result.
func (c *Context) EvalExpr(expression string) (interface{}, error) {
	program, err := expr.Compile(expression)
	if err != nil {
		return nil, fmt.Errorf("compiling expression %q: %w", expression, err)
	}

	result, err := expr.Run(program, c.All())
	if err != nil {
		return nil, fmt.Errorf("evaluating expression %q: %w", expression, err)
	}
	return result, nil
}

// SaveFromResult extracts values from a driver result using save mappings.
// Each save mapping is like: token -> response.body.token
// The expression is evaluated against the driver result merged into context.
func (c *Context) SaveFromResult(saveMap map[string]string, result map[string]interface{}) error {
	// Merge driver result into a temp env for evaluation
	env := c.All()
	for k, v := range result {
		env[k] = v
	}

	for varName, expression := range saveMap {
		program, err := expr.Compile(expression)
		if err != nil {
			return fmt.Errorf("compiling save expression %q for %q: %w", expression, varName, err)
		}
		val, err := expr.Run(program, env)
		if err != nil {
			return fmt.Errorf("evaluating save expression %q for %q: %w", expression, varName, err)
		}
		c.Set(varName, val)
	}
	return nil
}

// MarshalResult converts a driver's raw result into a JSON-friendly map.
func MarshalResult(v interface{}) map[string]interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
