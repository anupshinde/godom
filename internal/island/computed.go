package island

import (
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
)

// ComputedDef declares a computed field. Name is an exported struct field whose
// value the engine maintains by calling Fn — a pure derivation of the island's
// state — whenever any of Deps changes. Deps are other exported fields (plain or
// themselves computed). The engine assigns Fn's result to the field; Fn itself
// must not mutate island state.
type ComputedDef struct {
	Name string
	Fn   func() any
	Deps []string
}

type computed struct {
	name string
	fn   func() any
	deps []string
}

// RegisterComputed validates and installs computed-field definitions, then
// computes their initial values. It returns (rather than fatals) so the caller
// can surface the error at Register time. Fatal conditions: a name or dependency
// that is not an exported struct field, a duplicate computed name, or a
// dependency cycle among computeds.
func (ci *Info) RegisterComputed(defs []ComputedDef) error {
	if len(defs) == 0 {
		return nil
	}
	st := ci.Value.Elem()
	if ci.computeds == nil {
		ci.computeds = make(map[string]*computed)
	}

	// Names: must be exported settable fields; no duplicates.
	for _, d := range defs {
		if d.Name == "" || d.Fn == nil {
			return fmt.Errorf("computed: each definition needs a name and a function")
		}
		f := st.FieldByName(d.Name)
		if !f.IsValid() || !f.CanSet() {
			return fmt.Errorf("computed %q: not an exported, settable struct field", d.Name)
		}
		if _, dup := ci.computeds[d.Name]; dup {
			return fmt.Errorf("computed %q: declared more than once", d.Name)
		}
		ci.computeds[d.Name] = &computed{name: d.Name, fn: d.Fn, deps: d.Deps}
	}

	// Dependencies: must be exported struct fields (a plain field, or another
	// computed — which is also a field).
	for _, d := range defs {
		for _, dep := range d.Deps {
			if !st.FieldByName(dep).IsValid() {
				return fmt.Errorf("computed %q: dependency %q is not an exported struct field", d.Name, dep)
			}
		}
	}

	// Reverse graph for mark-expansion: dep name → computeds that depend on it.
	ci.computedDeps = make(map[string][]string)
	for _, c := range ci.computeds {
		for _, dep := range c.deps {
			ci.computedDeps[dep] = append(ci.computedDeps[dep], c.name)
		}
	}

	// Dependency order (deps before dependents) + cycle detection.
	order, err := ci.topoSortComputeds()
	if err != nil {
		return err
	}
	ci.computedOrder = order

	// Seed initial values so the first render reflects them.
	ci.RecomputeAll()
	return nil
}

// topoSortComputeds returns the computed names in dependency order (a computed's
// computed-dependencies appear before it) and reports a cycle if one exists.
func (ci *Info) topoSortComputeds() ([]string, error) {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS path
		black = 2 // done
	)
	color := make(map[string]int, len(ci.computeds))
	var order, path []string

	var visit func(name string) error
	visit = func(name string) error {
		color[name] = gray
		path = append(path, name)
		for _, dep := range ci.computeds[name].deps {
			if _, isComputed := ci.computeds[dep]; !isComputed {
				continue // only computed→computed edges constrain ordering
			}
			switch color[dep] {
			case gray:
				return fmt.Errorf("computed dependency cycle: %s -> %s", strings.Join(append(path, dep), " -> "), dep)
			case white:
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		path = path[:len(path)-1]
		color[name] = black
		order = append(order, name) // post-order: dependencies first
		return nil
	}

	names := make([]string, 0, len(ci.computeds))
	for n := range ci.computeds {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic order
	for _, n := range names {
		if color[n] == white {
			if err := visit(n); err != nil {
				return nil, err
			}
		}
	}
	return order, nil
}

// recompute evaluates one computed and assigns the result to its struct field.
func (ci *Info) recompute(name string) {
	c := ci.computeds[name]
	if c == nil {
		return
	}
	f := ci.Value.Elem().FieldByName(name)
	if !f.IsValid() || !f.CanSet() {
		return
	}
	val := c.fn()
	if val == nil {
		f.Set(reflect.Zero(f.Type()))
		return
	}
	rv := reflect.ValueOf(val)
	switch {
	case rv.Type().AssignableTo(f.Type()):
		f.Set(rv)
	case rv.Type().ConvertibleTo(f.Type()):
		f.Set(rv.Convert(f.Type()))
	default:
		log.Printf("godom: computed %q returned %s, not assignable to field type %s",
			name, rv.Type(), f.Type())
	}
}

// RecomputeAll recomputes every computed field in dependency order. Used on a
// full refresh and to seed initial values.
func (ci *Info) RecomputeAll() {
	for _, name := range ci.computedOrder {
		ci.recompute(name)
	}
}

// HasComputeds reports whether the island has any computed fields.
func (ci *Info) HasComputeds() bool { return len(ci.computeds) > 0 }

// ExpandAndRecompute expands marked field names to include every computed
// transitively reachable from them, recomputes those computeds in dependency
// order (assigning to their fields), and returns the expanded set so the
// surgical-patch path patches the computeds' bound nodes too.
func (ci *Info) ExpandAndRecompute(marked []string) []string {
	if len(ci.computeds) == 0 {
		return marked
	}
	seen := make(map[string]bool, len(marked))
	for _, m := range marked {
		seen[m] = true
	}
	expanded := append([]string(nil), marked...)
	reachable := make(map[string]bool)
	queue := append([]string(nil), marked...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dep := range ci.computedDeps[cur] {
			if seen[dep] {
				continue
			}
			seen[dep] = true
			reachable[dep] = true
			expanded = append(expanded, dep)
			queue = append(queue, dep) // computeds can have dependents too
		}
	}
	for _, name := range ci.computedOrder {
		if reachable[name] {
			ci.recompute(name)
		}
	}
	return expanded
}
