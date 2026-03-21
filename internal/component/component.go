package component

import (
	"encoding/json"
	"fmt"
	"log"
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

	// Component tree
	Parent   *Info                 // nil for root component
	Children map[string][]*Info    // forLoop GID → child instances

	// Prop fields (for stateful components)
	PropFields map[string]bool // field names marked with `godom:"prop"`

	// Registry reference (from App) for creating child instances
	Registry map[string]*Reg

	// RefreshFn is set by Start() to broadcast current state to all clients.
	// If fields are given, only those fields' bound nodes are patched (surgical).
	// If no fields, full init is broadcast.
	RefreshFn func()

	// MarkedFields accumulates field names marked for surgical refresh
	// via MarkRefresh(). Refresh() reads and clears this list.
	MarkedFields []string

	// VDOM fields
	VDOMTemplates []*vdom.TemplateNode    // parsed once at Mount()
	Tree      vdom.Node               // last rendered tree (for diffing)
	IDCounter     *vdom.IDCounter         // monotonic node ID allocator (persists across renders)
	Bindings      map[string][]vdom.Binding // field name → node bindings (built during first resolve)

	// Unbound input support
	UnboundValues map[string]any    // stableKey → value (survives tree rebuilds)
	NodeStableIDs map[int]string    // nodeID → stableKey (rebuilt each resolve)
}

// Reg holds the registration info for a stateful component.
type Reg struct {
	Typ   reflect.Type  // the struct type (not pointer)
	Proto reflect.Value // pointer to the prototype instance
}

// PropFieldNames returns the set of fields tagged with `godom:"prop"`.
func PropFieldNames(t reflect.Type) map[string]bool {
	props := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Tag.Get("godom") == "prop" {
			props[f.Name] = true
		}
	}
	return props
}

// SetProps sets prop fields on the component from a prop name→value map.
// Only fields tagged with `godom:"prop"` are written; this prevents parents
// from accidentally overwriting child-owned state.
func (ci *Info) SetProps(propValues map[string]interface{}) {
	v := ci.Value.Elem()
	for name, val := range propValues {
		if ci.PropFields != nil && !ci.PropFields[name] {
			continue
		}
		field := v.FieldByName(name)
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		rv := reflect.ValueOf(val)
		if rv.IsValid() && rv.Type().AssignableTo(field.Type()) {
			field.Set(rv)
		} else if rv.IsValid() && rv.Type().ConvertibleTo(field.Type()) {
			field.Set(rv.Convert(field.Type()))
		} else {
			// Try JSON round-trip for complex types (e.g., map → struct)
			data, err := json.Marshal(val)
			if err == nil {
				ptr := reflect.New(field.Type())
				if json.Unmarshal(data, ptr.Interface()) == nil {
					field.Set(ptr.Elem())
				}
			}
		}
	}
}

// GetState serializes all exported fields (excluding the embedded Component) to JSON.
func (ci *Info) GetState() ([]byte, error) {
	state := make(map[string]interface{})
	v := ci.Value.Elem()
	t := ci.Typ

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		// Skip the embedded Component struct (identified by field name)
		if field.Name == "Component" {
			continue
		}
		state[field.Name] = v.Field(i).Interface()
	}

	return json.Marshal(state)
}

// SnapshotState returns a JSON snapshot of the current state.
func (ci *Info) SnapshotState() []byte {
	data, err := ci.GetState()
	if err != nil {
		log.Fatalf("godom: failed to snapshot state: %v", err)
	}
	return data
}

// ChangedFields compares two state snapshots and returns the names of changed top-level fields.
func (ci *Info) ChangedFields(oldJSON, newJSON []byte) []string {
	var oldState, newState map[string]json.RawMessage
	if err := json.Unmarshal(oldJSON, &oldState); err != nil {
		return nil
	}
	if err := json.Unmarshal(newJSON, &newState); err != nil {
		return nil
	}

	var changed []string
	for key, newVal := range newState {
		oldVal, exists := oldState[key]
		if !exists || string(oldVal) != string(newVal) {
			changed = append(changed, key)
		}
	}
	for key := range oldState {
		if _, exists := newState[key]; !exists {
			changed = append(changed, key)
		}
	}
	return changed
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

	field := v.FieldByName(path)
	if !field.IsValid() || !field.CanSet() {
		return fmt.Errorf("field %q not found or not settable", path)
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

// AllExportedFieldNames returns the names of all exported fields except Component.
func AllExportedFieldNames(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.IsExported() && f.Name != "Component" {
			names = append(names, f.Name)
		}
	}
	return names
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
