package template

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/anupshinde/godom/internal/component"
)

// directiveRe matches g-* attributes in HTML.
var directiveRe = regexp.MustCompile(`g-(text|bind|value|click|keydown|mousedown|mousemove|mouseup|wheel|for|if|show|hide|checked|class:[a-zA-Z0-9_-]+|attr:[a-zA-Z0-9_-]+|style:[a-zA-Z0-9_-]+|plugin:[a-zA-Z0-9_-]+|draggable(?:\.[a-zA-Z0-9_-]+)?|dropzone|drop(?:\.[a-zA-Z0-9_-]+)?)\s*=\s*"([^"]*)"`)

// gForRe matches g-for attributes to extract loop variable names.
var gForRe = regexp.MustCompile(`g-for\s*=\s*"([^"]*)"`)

// gPropsRe matches g-props attributes to extract prop mappings.
var gPropsRe = regexp.MustCompile(`g-props\s*=\s*"([^"]*)"`)

// loopVarInfo holds the type info for a loop variable.
type loopVarInfo struct {
	itemType reflect.Type // element type of the array/slice
}

// ValidateDirectives parses HTML for g-* directives and validates them
// against the component's struct fields and methods. Called at Mount() time.
// For directives inside registered component subtrees, validation falls through
// to the child component's struct.
func ValidateDirectives(htmlStr string, ci *component.Info) error {
	loopVars := collectLoopVars(htmlStr, ci)

	// Build child CIs for registered components (for fallback validation)
	childCIs := buildChildCIs(ci)

	matches := directiveRe.FindAllStringSubmatch(htmlStr, -1)
	for _, m := range matches {
		dirType := m[1]
		expr := m[2]

		// Normalize group variants: "drop.canvas" → "drop", "draggable.palette" → "draggable"
		baseDirType := dirType
		if strings.HasPrefix(dirType, "drop.") {
			baseDirType = "drop"
		} else if strings.HasPrefix(dirType, "draggable.") {
			baseDirType = "draggable"
		}

		switch baseDirType {
		case "for":
			if err := validateForExpr(expr, ci, loopVars); err != nil {
				return err
			}
		case "bind":
			if err := validateBindExpr(expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(baseDirType, expr, childCIs) {
					return err
				}
			}
		case "click":
			if err := validateMethodRef("g-click", expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(baseDirType, expr, childCIs) {
					return err
				}
			}
		case "keydown":
			if err := validateKeydownExpr(expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(baseDirType, expr, childCIs) {
					return err
				}
			}
		case "mousedown", "mousemove", "mouseup", "wheel":
			if err := validateMethodRef("g-"+dirType, expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(baseDirType, expr, childCIs) {
					return err
				}
			}
		case "drop":
			if err := validateMethodRef("g-drop", expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(baseDirType, expr, childCIs) {
					return err
				}
			}
		default:
			if err := validateFieldExpr(expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(baseDirType, expr, childCIs) {
					return err
				}
			}
		}
	}

	return nil
}

// buildChildCIs creates temporary component.Infos for each registered component
// in the registry, used for validation fallback.
func buildChildCIs(parentCI *component.Info) []*component.Info {
	if parentCI.Registry == nil {
		return nil
	}
	var children []*component.Info
	for _, reg := range parentCI.Registry {
		children = append(children, &component.Info{
			Value:    reflect.New(reg.Typ),
			Typ:      reg.Typ,
			Registry: parentCI.Registry,
		})
	}
	return children
}

// validateAgainstChildren tries validating a directive against any registered
// child component struct. Returns true if any child validates successfully.
func validateAgainstChildren(dirType, expr string, childCIs []*component.Info) bool {
	for _, childCI := range childCIs {
		childLoopVars := map[string]*loopVarInfo{}
		switch dirType {
		case "click", "drop", "mousedown", "mousemove", "mouseup", "wheel":
			if validateMethodRef("g-"+dirType, expr, childCI, childLoopVars) == nil {
				return true
			}
		case "keydown":
			if validateKeydownExpr(expr, childCI, childLoopVars) == nil {
				return true
			}
		default:
			if validateFieldExpr(expr, childCI, childLoopVars) == nil {
				return true
			}
		}
	}
	return false
}

// collectLoopVars finds all loop variables declared in g-for expressions
// and resolves their types from the component struct.
func collectLoopVars(htmlStr string, ci *component.Info) map[string]*loopVarInfo {
	vars := map[string]*loopVarInfo{}
	matches := gForRe.FindAllStringSubmatch(htmlStr, -1)
	for _, m := range matches {
		parts := strings.SplitN(m[1], " in ", 2)
		if len(parts) != 2 {
			continue
		}

		listField := strings.TrimSpace(parts[1])
		left := strings.TrimSpace(parts[0])
		vs := strings.Split(left, ",")

		// Resolve the element type of the list field
		var elemType reflect.Type
		pathParts := strings.Split(listField, ".")
		if f, ok := ci.Typ.FieldByName(listField); ok && (f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Array) {
			// Top-level field (e.g., "Todos")
			elemType = f.Type.Elem()
		} else if len(pathParts) > 1 {
			// Dotted path — may start with a loop variable (e.g., "field.Options")
			if lv, ok := vars[pathParts[0]]; ok && lv.itemType != nil {
				resolved := resolveFieldType(lv.itemType, pathParts[1:])
				if resolved != nil && (resolved.Kind() == reflect.Slice || resolved.Kind() == reflect.Array) {
					elemType = resolved.Elem()
				}
			}
		}

		// Item variable gets the element type
		itemVar := strings.TrimSpace(vs[0])
		vars[itemVar] = &loopVarInfo{itemType: elemType}

		// Index variable is always int
		indexVar := ""
		if len(vs) > 1 {
			indexVar = strings.TrimSpace(vs[1])
			vars[indexVar] = &loopVarInfo{itemType: reflect.TypeOf(0)}
		}

		// Collect prop aliases — they map to loop variables
		propMatches := gPropsRe.FindAllStringSubmatch(htmlStr, -1)
		for _, pm := range propMatches {
			props := ParsePropsAttr(pm[1])
			for propName, parentExpr := range props {
				if parentExpr == itemVar {
					vars[propName] = &loopVarInfo{itemType: elemType}
				} else if parentExpr == indexVar {
					vars[propName] = &loopVarInfo{itemType: reflect.TypeOf(0)}
				} else if ci.HasField(parentExpr) {
					// Prop references a top-level field
					pf, _ := ci.Typ.FieldByName(parentExpr)
					vars[propName] = &loopVarInfo{itemType: pf.Type}
				}
			}
		}
	}
	return vars
}

func validateForExpr(expr string, ci *component.Info, loopVars map[string]*loopVarInfo) error {
	parts := strings.SplitN(expr, " in ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid g-for syntax: %q (expected 'item in List' or 'item, index in List')", expr)
	}
	listField := strings.TrimSpace(parts[1])

	// Top-level field (e.g., "Todos")
	if ci.HasField(listField) {
		return nil
	}

	// Dotted path through a loop variable (e.g., "field.Options")
	pathParts := strings.Split(listField, ".")
	if lv, ok := loopVars[pathParts[0]]; ok && lv.itemType != nil {
		return nil // trust the loop variable's type
	}

	return fmt.Errorf("g-for references unknown field %q on %s", listField, ci.Typ.Name())
}

func validateMethodRef(dirName, expr string, ci *component.Info, loopVars map[string]*loopVarInfo) error {
	name, args := component.ParseCallExpr(expr)
	if !ci.HasMethod(name) {
		return fmt.Errorf("%s references unknown method %q on %s", dirName, name, ci.Typ.Name())
	}
	for _, arg := range args {
		if err := validateArgExpr(arg, ci, loopVars); err != nil {
			return fmt.Errorf("%s %q: %w", dirName, expr, err)
		}
	}
	return nil
}

func validateKeydownExpr(expr string, ci *component.Info, loopVars map[string]*loopVarInfo) error {
	for _, part := range strings.Split(expr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		method := part
		if idx := strings.Index(part, ":"); idx != -1 {
			method = part[idx+1:]
		}
		if err := validateMethodRef("g-keydown", method, ci, loopVars); err != nil {
			return err
		}
	}
	return nil
}

// validateBindExpr ensures g-bind references a field, not a method.
// g-bind is two-way — it must be able to write back, which only works with fields.
func validateBindExpr(expr string, ci *component.Info, loopVars map[string]*loopVarInfo) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("empty g-bind expression")
	}

	root := strings.Split(expr, ".")[0]

	// If it's a method, fail loudly
	if ci.HasMethod(root) && !ci.HasField(root) {
		return fmt.Errorf("g-bind=%q references a method, not a field — g-bind requires a field for two-way binding (use g-value for one-way)", expr)
	}

	// Otherwise validate as a normal field expression
	return validateFieldExpr(expr, ci, loopVars)
}

func validateFieldExpr(expr string, ci *component.Info, loopVars map[string]*loopVarInfo) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("empty directive expression")
	}
	// Strip leading ! for negation
	if strings.HasPrefix(expr, "!") {
		expr = strings.TrimSpace(expr[1:])
	}
	if IsLiteral(expr) {
		return nil
	}

	parts := strings.Split(expr, ".")
	root := parts[0]

	// Check if it's a loop variable
	if lv, ok := loopVars[root]; ok {
		// Validate remaining path against the loop variable's type
		if len(parts) > 1 && lv.itemType != nil && lv.itemType.Kind() == reflect.Struct {
			return validateTypePath(lv.itemType, parts[1:], expr)
		}
		return nil
	}

	// Check if it's a component field
	if ci.HasField(root) {
		// Validate remaining path against the field's type
		if len(parts) > 1 {
			f, _ := ci.Typ.FieldByName(root)
			return validateTypePath(f.Type, parts[1:], expr)
		}
		return nil
	}

	// Check if it's a zero-arg, single-return method (computed value)
	if len(parts) == 1 && ci.HasMethod(root) {
		m, _ := ci.Value.Type().MethodByName(root)
		// Method on pointer receiver: first param is the receiver itself
		if m.Type.NumIn() == 1 && m.Type.NumOut() == 1 {
			return nil
		}
	}

	return fmt.Errorf("directive references unknown field or method %q (expression: %q) on %s", root, expr, ci.Typ.Name())
}

// validateTypePath walks a dotted path through struct types.
func validateTypePath(t reflect.Type, path []string, fullExpr string) error {
	current := t
	for _, part := range path {
		// Dereference pointers
		for current.Kind() == reflect.Ptr {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			// Can't validate further (e.g., map, interface)
			return nil
		}
		f, ok := current.FieldByName(part)
		if !ok {
			return fmt.Errorf("field %q not found on %s (expression: %q)", part, current.Name(), fullExpr)
		}
		current = f.Type
	}
	return nil
}

func validateArgExpr(arg string, ci *component.Info, loopVars map[string]*loopVarInfo) error {
	arg = strings.TrimSpace(arg)
	if IsLiteral(arg) {
		return nil
	}

	root := arg
	if idx := strings.Index(arg, "."); idx != -1 {
		root = arg[:idx]
	}

	if _, ok := loopVars[root]; ok {
		return nil
	}
	if ci.HasField(root) {
		return nil
	}

	return fmt.Errorf("unknown argument %q", arg)
}

// resolveFieldType walks a dotted path through a struct type, returning the final type.
func resolveFieldType(t reflect.Type, path []string) reflect.Type {
	current := t
	for _, part := range path {
		for current.Kind() == reflect.Ptr {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return nil
		}
		f, ok := current.FieldByName(part)
		if !ok {
			return nil
		}
		current = f.Type
	}
	return current
}

func IsLiteral(s string) bool {
	if s == "true" || s == "false" {
		return true
	}
	if _, err := strconv.Atoi(s); err == nil {
		return true
	}
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return true
	}
	return false
}
