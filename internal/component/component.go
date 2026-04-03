package component

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/anupshinde/godom/internal/vdom"

	gproto "github.com/anupshinde/godom/internal/proto"
)

// EventKind identifies the type of event sent to a component's event queue.
type EventKind int

const (
	NodeEventKind   EventKind = iota // browser input changed
	MethodCallKind                   // browser event handler (g-click, etc.)
	RefreshKind                      // background goroutine refresh
)

// Event is a unit of work sent to a component's event queue.
type Event struct {
	Kind   EventKind
	NodeID int32
	Value  string
	Call   *gproto.MethodCall
}

// Info holds reflection data about a mounted component.
type Info struct {
	Mu       sync.Mutex
	Value    reflect.Value // pointer to the user's struct
	Typ      reflect.Type  // the struct type (not pointer)
	SlotName string        // registered instance name (empty = root, renders into body)

	HTMLBody string

	// Removed is set when the component is unloaded (not used today, but
	// needed for dynamic mount/unmount and navigation view switching).
	Removed bool

	// RefreshFn is set by Start() to broadcast current state to all clients.
	// If fields are given, only those fields' bound nodes are patched (surgical).
	// If no fields, full init is broadcast.
	RefreshFn func()

	// markedFields accumulates field names marked for surgical refresh
	// via AddMarkedFields(). DrainMarkedFields() reads and clears this list.
	// Access only through AddMarkedFields/DrainMarkedFields.
	// Protected by markedMu (separate from Mu to avoid contention with tree ops).
	markedMu     sync.Mutex
	markedFields []string

	// LastChangedFields is populated by wireRefresh after a BuildUpdate,
	// containing the field names that produced patches. Used by
	// handleMethodCall to surgically refresh sibling components that
	// share embedded pointer state.
	LastChangedFields []string

	// EventCh is the event queue for this component. All events (browser
	// events, background refreshes) are sent here and processed sequentially
	// by a single goroutine. This eliminates race conditions between
	// concurrent event sources.
	EventCh chan Event

	// VDOM fields
	VDOMTemplates []*vdom.TemplateNode      // parsed once at Register()
	Tree          vdom.Node                 // last rendered tree (for diffing)
	IDCounter     *vdom.IDCounter           // monotonic node ID allocator (persists across renders)
	Bindings      map[string][]vdom.Binding // field name → node bindings (built during first resolve)
	InputBindings map[int]vdom.InputBinding // nodeID → field info for input bindings (reverse lookup)

	// Unbound input support
	UnboundValues map[string]any // stableKey → value (survives tree rebuilds)
	NodeStableIDs map[int]string // nodeID → stableKey (rebuilt each resolve)

	// ExecJS support
	ExecJSFn       func(id int32, expr string)                        // broadcast JSCall to all browsers (set by server)
	ExecJSDisabled bool                                                // when true, ExecJS calls are silently dropped
	JSCallbacks    map[int32]func(result []byte, err string)           // pending callbacks by request ID
	JSCallbackMu   sync.Mutex
	jsCallID       int32                                               // monotonic ID counter
}

// ExecJS sends a JavaScript expression to all connected browsers and calls the
// callback for each response. The callback receives JSON-encoded result bytes
// and an error string (empty on success).
func (ci *Info) ExecJS(expr string, cb func(result []byte, err string)) {
	if ci.ExecJSDisabled {
		if cb != nil {
			cb(nil, "ExecJS is disabled")
		}
		return
	}
	ci.JSCallbackMu.Lock()
	ci.jsCallID++
	id := ci.jsCallID
	if ci.JSCallbacks == nil {
		ci.JSCallbacks = make(map[int32]func(result []byte, err string))
	}
	ci.JSCallbacks[id] = cb
	ci.JSCallbackMu.Unlock()

	if ci.ExecJSFn != nil {
		ci.ExecJSFn(id, expr)
	}
}

// HandleJSResult dispatches a JSResult to the registered callback.
func (ci *Info) HandleJSResult(id int32, result []byte, errMsg string) {
	ci.JSCallbackMu.Lock()
	cb, ok := ci.JSCallbacks[id]
	// Don't delete — multiple browsers may respond with the same ID.
	// Cleanup happens via timeout or explicit clear.
	ci.JSCallbackMu.Unlock()

	if ok && cb != nil {
		cb(result, errMsg)
	}
}

// HasMethod returns true if the component has an exported method with the given name.
func (ci *Info) HasMethod(name string) bool {
	return ci.Value.MethodByName(name).IsValid()
}

// AddMarkedFields appends field names for surgical refresh. Thread-safe.
func (ci *Info) AddMarkedFields(fields ...string) {
	ci.markedMu.Lock()
	ci.markedFields = append(ci.markedFields, fields...)
	ci.markedMu.Unlock()
}

// DrainMarkedFields returns and clears the accumulated marked fields. Thread-safe.
func (ci *Info) DrainMarkedFields() []string {
	ci.markedMu.Lock()
	fields := ci.markedFields
	ci.markedFields = nil
	ci.markedMu.Unlock()
	return fields
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
