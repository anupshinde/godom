package godom

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Component is embedded in user structs to make them godom components.
type Component struct {
	ci *componentInfo // internal: set by godom when creating instances
}

// Refresh triggers a re-render and broadcasts the current state to all
// connected browsers. Call this from a background goroutine after mutating
// fields to push updates without user interaction.
func (c Component) Refresh() {
	if c.ci == nil {
		return
	}
	if c.ci.refreshFn != nil {
		c.ci.refreshFn()
	}
}

// Emit sends a named event up the component tree. Each ancestor with a matching
// method name gets called, bottom-up. Arguments are passed to the method.
func (c Component) Emit(method string, args ...interface{}) {
	if c.ci == nil {
		return
	}
	current := c.ci.parent
	for current != nil {
		if current.hasMethod(method) {
			// Convert args to json.RawMessage for callMethod
			jsonArgs := make([]json.RawMessage, len(args))
			for i, arg := range args {
				data, _ := json.Marshal(arg)
				jsonArgs[i] = data
			}
			if err := current.callMethod(method, jsonArgs); err != nil {
				log.Printf("godom: Emit %q: %v", method, err)
			}
		}
		current = current.parent
	}
}

// componentInfo holds reflection data about a mounted component.
type componentInfo struct {
	mu       sync.Mutex
	value    reflect.Value  // pointer to the user's struct
	typ      reflect.Type   // the struct type (not pointer)
	htmlBody string
	pb       *pageBindings  // parsed bindings (nil until Mount parses HTML)

	// Per-item diffing: previous list state keyed by list field name.
	// Each entry is the JSON-encoded items from the last render.
	prevLists map[string][]string

	// Component tree
	parent   *componentInfo                 // nil for root component
	children map[string][]*componentInfo    // forLoop GID → child instances

	// Prop fields (for stateful components)
	propFields map[string]bool // field names marked with `godom:"prop"`

	// Registry reference (from App) for creating child instances
	registry map[string]*componentReg

	// refreshFn is set by Start() to broadcast current state to all clients.
	refreshFn func()
}

// propFieldNames returns the set of fields tagged with `godom:"prop"`.
func propFieldNames(t reflect.Type) map[string]bool {
	props := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Tag.Get("godom") == "prop" {
			props[f.Name] = true
		}
	}
	return props
}

// setProps sets prop fields on the component from a prop name→value map.
func (ci *componentInfo) setProps(propValues map[string]interface{}) {
	v := ci.value.Elem()
	for name, val := range propValues {
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

// getState serializes all exported fields (excluding Component) to JSON.
func (ci *componentInfo) getState() ([]byte, error) {
	state := make(map[string]interface{})
	v := ci.value.Elem()
	t := ci.typ

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		if field.Type == reflect.TypeOf(Component{}) {
			continue
		}
		state[field.Name] = v.Field(i).Interface()
	}

	return json.Marshal(state)
}

// snapshotState returns a JSON snapshot of the current state.
func (ci *componentInfo) snapshotState() []byte {
	data, err := ci.getState()
	if err != nil {
		log.Fatalf("godom: failed to snapshot state: %v", err)
	}
	return data
}

// changedFields compares two state snapshots and returns the names of changed top-level fields.
func (ci *componentInfo) changedFields(oldJSON, newJSON []byte) []string {
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

// callMethod calls an exported method on the component by name with the given arguments.
func (ci *componentInfo) callMethod(name string, args []json.RawMessage) error {
	method := ci.value.MethodByName(name)
	if !method.IsValid() {
		return fmt.Errorf("method %q not found", name)
	}

	mt := method.Type()
	numIn := mt.NumIn()

	if len(args) != numIn {
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

// setField sets an exported field on the component (used for g-bind).
func (ci *componentInfo) setField(path string, rawValue json.RawMessage) error {
	v := ci.value.Elem()
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
			if n, convErr := strconv.ParseInt(s, 10, 64); convErr == nil {
				rv := reflect.ValueOf(n).Convert(field.Type())
				field.Set(rv)
				return nil
			}
			if f, convErr := strconv.ParseFloat(s, 64); convErr == nil {
				rv := reflect.ValueOf(f).Convert(field.Type())
				field.Set(rv)
				return nil
			}
		}
		return fmt.Errorf("field %q: %w", path, err)
	}
	field.Set(ptr.Elem())
	return nil
}

// allExportedFieldNames returns the names of all exported fields except Component.
func allExportedFieldNames(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.IsExported() && f.Type != reflect.TypeOf(Component{}) {
			names = append(names, f.Name)
		}
	}
	return names
}

// parseCallExpr parses "MethodName" or "MethodName(arg1, arg2)" into method name and arg strings.
func parseCallExpr(expr string) (string, []string) {
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

