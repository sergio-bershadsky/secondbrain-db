package virtuals

import (
	"fmt"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// CompiledVirtual is a pre-compiled Starlark function for a virtual field.
type CompiledVirtual struct {
	Name      string
	Returns   string
	computeFn starlark.Value // stored after Init — avoids re-Init per evaluation
}

// Runtime manages Starlark execution for virtual fields.
type Runtime struct {
	mu       sync.Mutex
	compiled map[string]*CompiledVirtual
}

// NewRuntime creates a new Starlark runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		compiled: make(map[string]*CompiledVirtual),
	}
}

// Compile pre-compiles a virtual field's Starlark source.
func (r *Runtime) Compile(name, source, returns string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Predeclared names must be known at compile time
	predeclared := starlark.StringDict{
		"re": reModule(),
	}
	_, prog, err := starlark.SourceProgramOptions(
		&syntax.FileOptions{},
		name+".star",
		source,
		predeclared.Has,
	)
	if err != nil {
		return fmt.Errorf("compiling virtual %q: %w", name, err)
	}

	// Init once to extract the compute callable — avoids re-Init per evaluation
	thread := &starlark.Thread{
		Name:  name,
		Print: func(_ *starlark.Thread, _ string) {},
	}
	globals, err := prog.Init(thread, predeclared)
	if err != nil {
		return fmt.Errorf("initializing virtual %q: %w", name, err)
	}
	computeFn, ok := globals["compute"]
	if !ok {
		return fmt.Errorf("virtual %q: missing compute(content, fields) function", name)
	}

	r.compiled[name] = &CompiledVirtual{
		Name:      name,
		Returns:   returns,
		computeFn: computeFn,
	}
	return nil
}

// Evaluate runs a compiled virtual field with the given content and field data.
func (r *Runtime) Evaluate(name, content string, fields map[string]any) (any, error) {
	r.mu.Lock()
	cv, ok := r.compiled[name]
	r.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("virtual %q not compiled", name)
	}

	thread := &starlark.Thread{
		Name:  name,
		Print: func(_ *starlark.Thread, _ string) {},
	}

	// Convert Go map to Starlark dict
	starFields, err := goToStarlark(fields)
	if err != nil {
		return nil, fmt.Errorf("virtual %q: converting fields: %w", name, err)
	}

	result, err := starlark.Call(thread, cv.computeFn, starlark.Tuple{
		starlark.String(content),
		starFields,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("virtual %q: execution failed: %w", name, err)
	}

	return starlarkToGo(result)
}

// EvaluateAll runs all compiled virtuals and returns a map of results.
func (r *Runtime) EvaluateAll(content string, fields map[string]any) (map[string]any, error) {
	r.mu.Lock()
	names := make([]string, 0, len(r.compiled))
	for name := range r.compiled {
		names = append(names, name)
	}
	r.mu.Unlock()

	results := make(map[string]any, len(names))
	for _, name := range names {
		val, err := r.Evaluate(name, content, fields)
		if err != nil {
			return nil, err
		}
		results[name] = val
	}
	return results, nil
}

// reModule returns a minimal re module for Starlark with findall, search, match.
func reModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "re",
		Members: starlark.StringDict{
			"findall": starlark.NewBuiltin("re.findall", reFindall),
		},
	}
}

func reFindall(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("re.findall: expected 2 arguments, got %d", len(args))
	}

	pattern, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("re.findall: pattern must be a string")
	}

	text, ok := starlark.AsString(args[1])
	if !ok {
		return nil, fmt.Errorf("re.findall: text must be a string")
	}

	// Simple pattern matching without Go's regexp to keep it sandboxed
	// We implement a basic subset: literal matches and character classes
	matches := simpleRegexFindall(pattern, text)

	var elems []starlark.Value
	for _, m := range matches {
		elems = append(elems, starlark.String(m))
	}
	return starlark.NewList(elems), nil
}

// simpleRegexFindall implements basic regex-like matching.
// For the common virtual-field patterns (e.g., "[A-Z]+-[0-9]+"),
// we use Go's stdlib regexp under the hood.
func simpleRegexFindall(pattern, text string) []string {
	// Use Go's regexp for real matching
	re, err := compileRegex(pattern)
	if err != nil {
		return nil
	}
	return re.FindAllString(text, -1)
}

// goToStarlark converts a Go value to a Starlark value.
func goToStarlark(v any) (starlark.Value, error) {
	switch val := v.(type) {
	case nil:
		return starlark.None, nil
	case string:
		return starlark.String(val), nil
	case bool:
		return starlark.Bool(val), nil
	case int:
		return starlark.MakeInt(val), nil
	case int64:
		return starlark.MakeInt64(val), nil
	case float64:
		return starlark.Float(val), nil
	case []any:
		var elems []starlark.Value
		for _, item := range val {
			sv, err := goToStarlark(item)
			if err != nil {
				return nil, err
			}
			elems = append(elems, sv)
		}
		return starlark.NewList(elems), nil
	case []string:
		var elems []starlark.Value
		for _, s := range val {
			elems = append(elems, starlark.String(s))
		}
		return starlark.NewList(elems), nil
	case map[string]any:
		d := starlark.NewDict(len(val))
		for k, v := range val {
			sv, err := goToStarlark(v)
			if err != nil {
				return nil, err
			}
			if err := d.SetKey(starlark.String(k), sv); err != nil {
				return nil, err
			}
		}
		return d, nil
	default:
		return starlark.String(fmt.Sprintf("%v", val)), nil
	}
}

// starlarkToGo converts a Starlark value to a Go value.
func starlarkToGo(v starlark.Value) (any, error) {
	switch val := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.String:
		return string(val), nil
	case starlark.Bool:
		return bool(val), nil
	case starlark.Int:
		i, ok := val.Int64()
		if ok {
			return int(i), nil
		}
		return val.String(), nil
	case starlark.Float:
		return float64(val), nil
	case *starlark.List:
		var result []any
		iter := val.Iterate()
		defer iter.Done()
		var item starlark.Value
		for iter.Next(&item) {
			g, err := starlarkToGo(item)
			if err != nil {
				return nil, err
			}
			result = append(result, g)
		}
		return result, nil
	case *starlark.Dict:
		result := make(map[string]any)
		for _, kv := range val.Items() {
			key, ok := starlark.AsString(kv[0])
			if !ok {
				key = kv[0].String()
			}
			g, err := starlarkToGo(kv[1])
			if err != nil {
				return nil, err
			}
			result[key] = g
		}
		return result, nil
	case starlark.Tuple:
		var result []any
		for _, item := range val {
			g, err := starlarkToGo(item)
			if err != nil {
				return nil, err
			}
			result = append(result, g)
		}
		return result, nil
	default:
		return val.String(), nil
	}
}
