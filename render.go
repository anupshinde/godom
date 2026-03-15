package godom

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// --- Wire types: sent over WebSocket to the bridge ---

type command struct {
	Op    string      `json:"op"`              // "text","value","checked","display","class","list"
	ID    string      `json:"id"`              // data-gid of target element
	Val   interface{} `json:"val,omitempty"`    // value to set
	Name  string      `json:"name,omitempty"`   // class name (for "class" op)
	Items []listItem  `json:"items"` // for "list" op
}

type listItem struct {
	HTML   string         `json:"html"`
	Cmds   []command      `json:"cmds"`
	Evts   []eventCommand `json:"evts"`
}

type eventCommand struct {
	ID   string          `json:"id"`             // data-gid
	On   string          `json:"on"`             // "click","keydown","input"
	Key  string          `json:"key,omitempty"`  // key filter for keydown
	Msg  json.RawMessage `json:"msg"`            // pre-built message bridge sends back
}

// --- Expression resolution ---

// resolveExprVal resolves an expression string against component state and loop context.
// Supports field access ("Name"), dotted paths ("todo.Text"), loop variables,
// and literals (true, false, integers, quoted strings).
func resolveExprVal(expr string, state map[string]interface{}, ctx map[string]interface{}) interface{} {
	expr = strings.TrimSpace(expr)

	// Literals
	if expr == "true" {
		return true
	}
	if expr == "false" {
		return false
	}
	if n, err := strconv.Atoi(expr); err == nil {
		return n
	}
	if len(expr) >= 2 {
		if (expr[0] == '"' && expr[len(expr)-1] == '"') ||
			(expr[0] == '\'' && expr[len(expr)-1] == '\'') {
			return expr[1 : len(expr)-1]
		}
	}

	parts := strings.Split(expr, ".")
	root := parts[0]

	// Try loop context first
	if ctx != nil {
		if val, ok := ctx[root]; ok {
			return resolvePath(val, parts[1:])
		}
	}

	// Then component state
	if val, ok := state[root]; ok {
		return resolvePath(val, parts[1:])
	}

	return nil
}

// resolvePath walks a dotted path through nested maps.
func resolvePath(val interface{}, parts []string) interface{} {
	for _, part := range parts {
		m, ok := val.(map[string]interface{})
		if !ok {
			return nil
		}
		val = m[part]
	}
	return val
}

// toBool converts a value to a boolean for conditional directives.
func toBool(val interface{}) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case string:
		return v != ""
	default:
		return true
	}
}

// --- Init message: full state → all commands + events ---

func computeInitMessage(pb *pageBindings, ci *componentInfo) (map[string]interface{}, error) {
	state := stateMap(ci)

	cmds := computeBindingCmds(pb.Bindings, state, nil)
	evts := computeEventCmds(pb.Events, state, nil)

	// g-for lists — full render + seed prevLists for future diffs
	if ci.prevLists == nil {
		ci.prevLists = make(map[string][]string)
	}
	for _, fl := range pb.ForLoops {
		cmds = append(cmds, computeListCmd(fl, state, ci))
		// Seed previous list state
		listVal := state[fl.ListField]
		items, _ := listVal.([]interface{})
		snapshots := make([]string, len(items))
		for i, item := range items {
			data, _ := json.Marshal(item)
			snapshots[i] = string(data)
		}
		ci.prevLists[fl.ListField] = snapshots
	}

	return map[string]interface{}{
		"type":     "init",
		"commands": cmds,
		"events":   evts,
	}, nil
}

// --- Update message: only changed fields → affected commands ---

func computeUpdateMessage(pb *pageBindings, ci *componentInfo, changed []string) map[string]interface{} {
	state := stateMap(ci)

	var cmds []command
	for _, field := range changed {
		if indices, ok := pb.FieldToBindings[field]; ok {
			for _, idx := range indices {
				b := pb.Bindings[idx]
				cmds = append(cmds, singleCmd(b, state, nil))
			}
		}
		if indices, ok := pb.FieldToForLoops[field]; ok {
			for _, idx := range indices {
				diffCmds := computeListDiff(pb.ForLoops[idx], state, ci)
				cmds = append(cmds, diffCmds...)
			}
		}
	}

	if len(cmds) == 0 {
		return nil
	}

	return map[string]interface{}{
		"type":     "update",
		"commands": cmds,
	}
}

// --- Binding → command conversion ---

func computeBindingCmds(bindings []binding, state map[string]interface{}, ctx map[string]interface{}) []command {
	cmds := make([]command, 0, len(bindings))
	for _, b := range bindings {
		cmds = append(cmds, singleCmd(b, state, ctx))
	}
	return cmds
}

func singleCmd(b binding, state map[string]interface{}, ctx map[string]interface{}) command {
	val := resolveExprVal(b.Expr, state, ctx)

	switch {
	case b.Dir == "text":
		return command{Op: "text", ID: b.GID, Val: val}
	case b.Dir == "bind":
		return command{Op: "value", ID: b.GID, Val: val}
	case b.Dir == "checked":
		return command{Op: "checked", ID: b.GID, Val: toBool(val)}
	case b.Dir == "if" || b.Dir == "show":
		return command{Op: "display", ID: b.GID, Val: toBool(val)}
	default:
		if strings.HasPrefix(b.Dir, "class:") {
			return command{Op: "class", ID: b.GID, Name: b.Dir[6:], Val: toBool(val)}
		}
		if strings.HasPrefix(b.Dir, "attr:") {
			return command{Op: "attr", ID: b.GID, Name: b.Dir[5:], Val: val}
		}
		return command{Op: "text", ID: b.GID, Val: val}
	}
}

// --- Event → eventCommand conversion ---

func computeEventCmds(events []eventBinding, state map[string]interface{}, ctx map[string]interface{}) []eventCommand {
	cmds := make([]eventCommand, 0, len(events))
	for _, e := range events {
		cmds = append(cmds, singleEventCmd(e, state, ctx))
	}
	return cmds
}

func singleEventCmd(e eventBinding, state map[string]interface{}, ctx map[string]interface{}) eventCommand {
	if e.Method == "__bind" {
		// Two-way binding: bridge sends field + value back
		msg, _ := json.Marshal(map[string]interface{}{
			"type":  "bind",
			"field": e.Args[0],
		})
		return eventCommand{ID: e.GID, On: e.Event, Msg: msg}
	}

	// Resolve arguments now (Go-side)
	resolved := make([]interface{}, len(e.Args))
	for i, arg := range e.Args {
		resolved[i] = resolveExprVal(arg, state, ctx)
	}

	msg, _ := json.Marshal(map[string]interface{}{
		"type":   "call",
		"method": e.Method,
		"args":   resolved,
	})
	return eventCommand{ID: e.GID, On: e.Event, Key: e.Key, Msg: msg}
}

// computeChildUpdateMessage re-renders a single child component instance
// identified by scope (e.g., "g3:0"). Used when a child method changes child
// state but not parent state.
func computeChildUpdateMessage(pb *pageBindings, ci *componentInfo, scope string) map[string]interface{} {
	parts := strings.SplitN(scope, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	forGID := parts[0]
	idx, _ := strconv.Atoi(parts[1])

	// Find the forTemplate for this GID
	var ft *forTemplate
	for _, fl := range pb.ForLoops {
		if fl.GID == forGID {
			ft = fl
			break
		}
	}
	if ft == nil || ft.ComponentTag == "" {
		return nil
	}

	children := ci.children[forGID]
	if idx < 0 || idx >= len(children) {
		return nil
	}

	child := children[idx]
	childState := stateMap(child)
	idxStr := strconv.Itoa(idx)

	var cmds []command
	for _, b := range ft.Bindings {
		resolved := binding{
			GID:  strings.ReplaceAll(b.GID, "__IDX__", idxStr),
			Dir:  b.Dir,
			Expr: b.Expr,
		}
		cmds = append(cmds, singleCmd(resolved, childState, nil))
	}

	if len(cmds) == 0 {
		return nil
	}

	return map[string]interface{}{
		"type":     "update",
		"commands": cmds,
	}
}

// buildItemCtx creates the per-item context for a g-for iteration,
// including loop variables and resolved prop aliases.
func buildItemCtx(ft *forTemplate, state map[string]interface{}, item interface{}, idx int) map[string]interface{} {
	ctx := map[string]interface{}{
		ft.ItemVar: item,
	}
	if ft.IndexVar != "" {
		ctx[ft.IndexVar] = idx
	}
	// Add prop aliases: e.g. "index" → value of "i", "todo" → value of "todo"
	for propName, parentExpr := range ft.Props {
		ctx[propName] = resolveExprVal(parentExpr, state, ctx)
	}
	return ctx
}

// resolveItemBindings resolves bindings and events for a single g-for item.
// For stateful components, it sets child props and resolves against child state.
// For presentational components, it resolves against parent state + loop context.
func resolveItemBindings(ft *forTemplate, ci *componentInfo, state map[string]interface{}, item interface{}, idx int) ([]command, []eventCommand) {
	idxStr := strconv.Itoa(idx)
	var cmds []command
	var evts []eventCommand

	if ft.ComponentTag != "" && ci.children[ft.GID] != nil && idx < len(ci.children[ft.GID]) {
		child := ci.children[ft.GID][idx]
		parentCtx := buildItemCtx(ft, state, item, idx)
		setChildProps(child, ft.Props, state, parentCtx)

		childState := stateMap(child)
		for _, b := range ft.Bindings {
			resolved := binding{
				GID:  strings.ReplaceAll(b.GID, "__IDX__", idxStr),
				Dir:  b.Dir,
				Expr: b.Expr,
			}
			cmds = append(cmds, singleCmd(resolved, childState, nil))
		}
		for _, e := range ft.Events {
			resolved := eventBinding{
				GID:    strings.ReplaceAll(e.GID, "__IDX__", idxStr),
				Event:  e.Event,
				Key:    e.Key,
				Method: e.Method,
				Args:   e.Args,
			}
			evts = append(evts, singleScopedEventCmd(resolved, childState, nil, ft.GID, idx))
		}
	} else {
		ctx := buildItemCtx(ft, state, item, idx)
		for _, b := range ft.Bindings {
			resolved := binding{
				GID:  strings.ReplaceAll(b.GID, "__IDX__", idxStr),
				Dir:  b.Dir,
				Expr: b.Expr,
			}
			cmds = append(cmds, singleCmd(resolved, state, ctx))
		}
		for _, e := range ft.Events {
			resolved := eventBinding{
				GID:    strings.ReplaceAll(e.GID, "__IDX__", idxStr),
				Event:  e.Event,
				Key:    e.Key,
				Method: e.Method,
				Args:   e.Args,
			}
			evts = append(evts, singleEventCmd(resolved, state, ctx))
		}
	}

	return cmds, evts
}

// --- g-for list rendering ---

// computeListDiff compares previous and current list items and returns targeted
// commands instead of a full re-render. Returns nil if nothing changed.
func computeListDiff(ft *forTemplate, state map[string]interface{}, ci *componentInfo) []command {
	listVal := state[ft.ListField]
	items, _ := listVal.([]interface{})
	if items == nil {
		items = []interface{}{}
	}

	// Get previous item snapshots
	if ci.prevLists == nil {
		ci.prevLists = make(map[string][]string)
	}
	prevItems := ci.prevLists[ft.ListField]
	oldLen := len(prevItems)
	newLen := len(items)

	// Snapshot current items as JSON strings for comparison
	currentSnapshots := make([]string, newLen)
	for i, item := range items {
		data, _ := json.Marshal(item)
		currentSnapshots[i] = string(data)
	}

	// Store for next diff
	ci.prevLists[ft.ListField] = currentSnapshots

	// If both empty, nothing to do
	if oldLen == 0 && newLen == 0 {
		return nil
	}

	// First render or list was empty before — full render
	if oldLen == 0 {
		return []command{computeListCmd(ft, state, ci)}
	}

	// List cleared — full render (empty)
	if newLen == 0 {
		return []command{computeListCmd(ft, state, ci)}
	}

	var cmds []command
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}

	// For stateful components, ensure child instances
	if ft.ComponentTag != "" {
		ensureChildInstances(ft, ci, newLen)
	}

	// Send update commands for existing items that changed
	for idx := 0; idx < minLen; idx++ {
		if currentSnapshots[idx] == prevItems[idx] {
			continue // unchanged
		}
		itemCmds, itemEvts := resolveItemBindings(ft, ci, state, items[idx], idx)
		cmds = append(cmds, itemCmds...)
		for _, evt := range itemEvts {
			cmds = append(cmds, command{Op: "re-event", ID: evt.ID, Val: evt})
		}
	}

	// Items removed from the end
	if newLen < oldLen {
		// Trim child instances too
		if ft.ComponentTag != "" && ci.children[ft.GID] != nil {
			if len(ci.children[ft.GID]) > newLen {
				ci.children[ft.GID] = ci.children[ft.GID][:newLen]
			}
		}
		cmds = append(cmds, command{
			Op:  "list-truncate",
			ID:  ft.GID,
			Val: oldLen - newLen,
		})
	}

	// Items appended at the end
	if newLen > oldLen {
		appendItems := make([]listItem, 0, newLen-oldLen)
		for idx := oldLen; idx < newLen; idx++ {
			idxStr := strconv.Itoa(idx)
			itemHTML := strings.ReplaceAll(ft.TemplateHTML, "__IDX__", idxStr)
			itemCmds, evts := resolveItemBindings(ft, ci, state, items[idx], idx)
			appendItems = append(appendItems, listItem{
				HTML: itemHTML,
				Cmds: itemCmds,
				Evts: evts,
			})
		}
		cmds = append(cmds, command{
			Op:    "list-append",
			ID:    ft.GID,
			Items: appendItems,
		})
	}

	return cmds
}

func computeListCmd(ft *forTemplate, state map[string]interface{}, ci *componentInfo) command {
	listVal := state[ft.ListField]
	items, _ := listVal.([]interface{})
	if items == nil {
		items = []interface{}{}
	}

	// For stateful components, ensure child instances exist
	if ft.ComponentTag != "" {
		ensureChildInstances(ft, ci, len(items))
	}

	listItems := make([]listItem, 0, len(items))
	for idx, item := range items {
		idxStr := strconv.Itoa(idx)
		itemHTML := strings.ReplaceAll(ft.TemplateHTML, "__IDX__", idxStr)
		itemCmds, evts := resolveItemBindings(ft, ci, state, item, idx)
		listItems = append(listItems, listItem{
			HTML: itemHTML,
			Cmds: itemCmds,
			Evts: evts,
		})
	}

	return command{Op: "list", ID: ft.GID, Items: listItems}
}

// ensureChildInstances creates or trims child componentInfo instances for a stateful g-for.
func ensureChildInstances(ft *forTemplate, parent *componentInfo, count int) {
	reg := parent.registry[ft.ComponentTag]
	if reg == nil {
		return
	}

	existing := parent.children[ft.GID]

	// Trim if we have too many
	if len(existing) > count {
		parent.children[ft.GID] = existing[:count]
		existing = parent.children[ft.GID]
	}

	// Create new instances if we need more
	for len(existing) < count {
		newVal := reflect.New(reg.typ)
		child := &componentInfo{
			value:      newVal,
			typ:        reg.typ,
			parent:     parent,
			children:   make(map[string][]*componentInfo),
			propFields: propFieldNames(reg.typ),
			registry:   parent.registry,
		}
		// Set the Component.ci pointer so Emit works
		compField := newVal.Elem().FieldByName("Component")
		if compField.IsValid() && compField.CanSet() {
			compField.Set(reflect.ValueOf(Component{ci: child}))
		}
		existing = append(existing, child)
	}
	parent.children[ft.GID] = existing
}

// setChildProps resolves prop expressions in parent context and sets them on the child.
func setChildProps(child *componentInfo, props map[string]string, parentState map[string]interface{}, parentCtx map[string]interface{}) {
	propValues := make(map[string]interface{})
	for propName, parentExpr := range props {
		val := resolveExprVal(parentExpr, parentState, parentCtx)
		propValues[propName] = val
	}
	child.setProps(propValues)
}

// singleScopedEventCmd builds an event command scoped to a child component instance.
func singleScopedEventCmd(e eventBinding, state map[string]interface{}, ctx map[string]interface{}, forGID string, idx int) eventCommand {
	if e.Method == "__bind" {
		msg, _ := json.Marshal(map[string]interface{}{
			"type":  "bind",
			"field": e.Args[0],
			"scope": forGID + ":" + strconv.Itoa(idx),
		})
		return eventCommand{ID: e.GID, On: e.Event, Msg: msg}
	}

	resolved := make([]interface{}, len(e.Args))
	for i, arg := range e.Args {
		resolved[i] = resolveExprVal(arg, state, ctx)
	}

	msg, _ := json.Marshal(map[string]interface{}{
		"type":   "call",
		"method": e.Method,
		"args":   resolved,
		"scope":  forGID + ":" + strconv.Itoa(idx),
	})
	return eventCommand{ID: e.GID, On: e.Event, Key: e.Key, Msg: msg}
}

// stateMap returns the component state as map[string]interface{}.
func stateMap(ci *componentInfo) map[string]interface{} {
	data, _ := ci.getState()
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if m == nil {
		m = make(map[string]interface{})
	}
	return m
}
