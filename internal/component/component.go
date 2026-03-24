package component

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/anupshinde/godom/internal/vdom"
)

// Info holds reflection data about a mounted component.
type Info struct {
	Mu    sync.Mutex
	Value reflect.Value // pointer to the user's struct
	Typ   reflect.Type  // the struct type (not pointer)

	HTMLBody string

	// RefreshFn is set by Start() to broadcast current state to all clients.
	// If fields are given, only those fields' bound nodes are patched (surgical).
	// If no fields, full init is broadcast.
	RefreshFn func()

	// MarkedFields accumulates field names marked for surgical refresh
	// via MarkRefresh(). Refresh() reads and clears this list.
	MarkedFields []string

	// VDOM fields
	VDOMTemplates []*vdom.TemplateNode      // parsed once at Mount()
	Tree          vdom.Node                 // last rendered tree (for diffing)
	IDCounter     *vdom.IDCounter           // monotonic node ID allocator (persists across renders)
	Bindings      map[string][]vdom.Binding // field name → node bindings (built during first resolve)
	InputBindings map[int]vdom.InputBinding // nodeID → field info for input bindings (reverse lookup)

	// Unbound input support
	UnboundValues map[string]any // stableKey → value (survives tree rebuilds)
	NodeStableIDs map[int]string // nodeID → stableKey (rebuilt each resolve)
}

// CallMethod calls an exported method on the component by name with the given arguments.
func (ci *Info) CallMethod(name string, args []json.RawMessage) error {
	method := ci.Value.MethodByName(name)
	if !method.IsValid() {
		return fmt.Errorf("method %q not found", name)
	}

	mt := method.Type()
	numIn := mt.NumIn()

	if len(args) < numIn {
		return fmt.Errorf("method %q expects %d args, got %d", name, numIn, len(args))
	}

	in := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		paramType := mt.In(i)
		param := reflect.New(paramType)

		if err := json.Unmarshal(args[i], param.Interface()); err != nil {
			return fmt.Errorf("method %q arg %d: %w", name, i, err)
		}
		in[i] = param.Elem()
	}

	method.Call(in)
	return nil
}

// SetField sets an exported field on the component (used for g-bind).
// Supports bracket syntax for map access: "Inputs[first]" sets map key "first" on field Inputs.
func (ci *Info) SetField(path string, rawValue json.RawMessage) error {
	v := ci.Value.Elem()

	// Handle bracket map access: "Inputs[first]"
	if bracketIdx := strings.Index(path, "["); bracketIdx != -1 && strings.HasSuffix(path, "]") {
		fieldName := path[:bracketIdx]
		mapKey := path[bracketIdx+1 : len(path)-1]
		field := v.FieldByName(fieldName)
		if !field.IsValid() || !field.CanSet() {
			return fmt.Errorf("field %q not found or not settable", fieldName)
		}
		if field.Kind() != reflect.Map {
			return fmt.Errorf("field %q is not a map", fieldName)
		}
		if field.IsNil() {
			field.Set(reflect.MakeMap(field.Type()))
		}
		// Unmarshal the value to the map's value type
		valType := field.Type().Elem()
		ptr := reflect.New(valType)
		if err := json.Unmarshal(rawValue, ptr.Interface()); err != nil {
			return fmt.Errorf("field %q key %q: %w", fieldName, mapKey, err)
		}
		field.SetMapIndex(reflect.ValueOf(mapKey), ptr.Elem())
		return nil
	}

	field, err := walkFieldPath(v, path)
	if err != nil {
		return err
	}

	ptr := reflect.New(field.Type())
	if err := json.Unmarshal(rawValue, ptr.Interface()); err != nil {
		// HTML inputs always send strings. If the target field is numeric,
		// unwrap the JSON string and parse the number.
		var s string
		if json.Unmarshal(rawValue, &s) == nil {
			// Empty string → zero value for numeric fields
			if s == "" {
				field.Set(reflect.Zero(field.Type()))
				return nil
			}
			switch field.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if n, convErr := strconv.ParseInt(s, 10, 64); convErr == nil {
					field.SetInt(n)
					return nil
				}
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if n, convErr := strconv.ParseUint(s, 10, 64); convErr == nil {
					field.SetUint(n)
					return nil
				}
			case reflect.Float32, reflect.Float64:
				if f, convErr := strconv.ParseFloat(s, 64); convErr == nil {
					field.SetFloat(f)
					return nil
				}
			}
		}
		return fmt.Errorf("field %q: %w", path, err)
	}
	field.Set(ptr.Elem())
	return nil
}

// walkFieldPath walks a dotted path like "Box.Top" and returns the final settable field.
func walkFieldPath(v reflect.Value, path string) (reflect.Value, error) {
	parts := strings.Split(path, ".")
	for _, part := range parts {
		for v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return reflect.Value{}, fmt.Errorf("field %q: nil pointer in path", path)
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, fmt.Errorf("field %q: not a struct at %q", path, part)
		}
		v = v.FieldByName(part)
		if !v.IsValid() || !v.CanSet() {
			return reflect.Value{}, fmt.Errorf("field %q not found or not settable", path)
		}
	}
	return v, nil
}

// HasField checks if the component struct has an exported field with the given name.
func (ci *Info) HasField(name string) bool {
	f, ok := ci.Typ.FieldByName(name)
	if !ok {
		return false
	}
	return f.IsExported() && f.Name != "Component"
}

// HasMethod checks if the component has an exported method with the given name.
func (ci *Info) HasMethod(name string) bool {
	return ci.Value.MethodByName(name).IsValid()
}

// ParseCallExpr parses "MethodName" or "MethodName(arg1, arg2)" into method name and arg strings.
func ParseCallExpr(expr string) (string, []string) {
	expr = strings.TrimSpace(expr)
	parenIdx := strings.Index(expr, "(")
	if parenIdx == -1 {
		return expr, nil
	}

	name := expr[:parenIdx]
	argsStr := strings.TrimSuffix(expr[parenIdx+1:], ")")
	argsStr = strings.TrimSpace(argsStr)
	if argsStr == "" {
		return name, nil
	}

	args := strings.Split(argsStr, ",")
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}
	return name, args
}
