package godom

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// directiveRe matches g-* attributes in HTML.
var directiveRe = regexp.MustCompile(`g-(text|bind|click|keydown|mousedown|mousemove|mouseup|wheel|for|if|show|checked|class:[a-zA-Z0-9_-]+|attr:[a-zA-Z0-9_-]+|style:[a-zA-Z0-9_-]+|plugin:[a-zA-Z0-9_-]+|draggable(?:\.[a-zA-Z0-9_-]+)?|dropzone|drop(?:\.[a-zA-Z0-9_-]+)?)\s*=\s*"([^"]*)"`)

// gForRe matches g-for attributes to extract loop variable names.
var gForRe = regexp.MustCompile(`g-for\s*=\s*"([^"]*)"`)

// gPropsRe matches g-props attributes to extract prop mappings.
var gPropsRe = regexp.MustCompile(`g-props\s*=\s*"([^"]*)"`)

// loopVarInfo holds the type info for a loop variable.
type loopVarInfo struct {
	itemType reflect.Type // element type of the array/slice
}

// validateDirectives parses HTML for g-* directives and validates them
// against the component's struct fields and methods. Called at Mount() time.
// For directives inside registered component subtrees, validation falls through
// to the child component's struct.
func validateDirectives(htmlStr string, ci *componentInfo) error {
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
			if err := validateForExpr(expr, ci); err != nil {
				return err
			}
		case "click":
			if err := validateMethodRef("g-click", expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(dirType, expr, childCIs) {
					return err
				}
			}
		case "keydown":
			if err := validateKeydownExpr(expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(dirType, expr, childCIs) {
					return err
				}
			}
		case "mousedown", "mousemove", "mouseup", "wheel":
			if err := validateMethodRef("g-"+dirType, expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(dirType, expr, childCIs) {
					return err
				}
			}
		case "drop":
			if err := validateMethodRef("g-drop", expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(dirType, expr, childCIs) {
					return err
				}
			}
		default:
			if err := validateFieldExpr(expr, ci, loopVars); err != nil {
				if !validateAgainstChildren(dirType, expr, childCIs) {
					return err
				}
			}
		}
	}

	return nil
}

// buildChildCIs creates temporary componentInfos for each registered component
// in the registry, used for validation fallback.
func buildChildCIs(parentCI *componentInfo) []*componentInfo {
	if parentCI.registry == nil {
		return nil
	}
	var children []*componentInfo
	for _, reg := range parentCI.registry {
		children = append(children, &componentInfo{
			value:    reflect.New(reg.typ),
			typ:      reg.typ,
			registry: parentCI.registry,
		})
	}
	return children
}

// validateAgainstChildren tries validating a directive against any registered
// child component struct. Returns true if any child validates successfully.
func validateAgainstChildren(dirType, expr string, childCIs []*componentInfo) bool {
	for _, childCI := range childCIs {
		childLoopVars := map[string]*loopVarInfo{}
		switch dirType {
		case "click", "drop":
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
// e.g., g-for="todo, i in Todos" where Todos is []Todo
// → loopVars["todo"] = {itemType: Todo}, loopVars["i"] = {itemType: int}
func collectLoopVars(htmlStr string, ci *componentInfo) map[string]*loopVarInfo {
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
		f, ok := ci.typ.FieldByName(listField)
		if ok && (f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Array) {
			elemType = f.Type.Elem()
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
		// e.g., g-props="index:i,todo:todo" → "index" gets same type as "i"
		propMatches := gPropsRe.FindAllStringSubmatch(htmlStr, -1)
		for _, pm := range propMatches {
			props := parsePropsAttr(pm[1])
			for propName, parentExpr := range props {
				if parentExpr == itemVar {
					vars[propName] = &loopVarInfo{itemType: elemType}
				} else if parentExpr == indexVar {
					vars[propName] = &loopVarInfo{itemType: reflect.TypeOf(0)}
				} else if ci.hasField(parentExpr) {
					// Prop references a top-level field
					pf, _ := ci.typ.FieldByName(parentExpr)
					vars[propName] = &loopVarInfo{itemType: pf.Type}
				}
			}
		}
	}
	return vars
}

func validateForExpr(expr string, ci *componentInfo) error {
	parts := strings.SplitN(expr, " in ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid g-for syntax: %q (expected 'item in List' or 'item, index in List')", expr)
	}
	listField := strings.TrimSpace(parts[1])
	if !ci.hasField(listField) {
		return fmt.Errorf("g-for references unknown field %q on %s", listField, ci.typ.Name())
	}
	return nil
}

func validateMethodRef(dirName, expr string, ci *componentInfo, loopVars map[string]*loopVarInfo) error {
	name, args := parseCallExpr(expr)
	if !ci.hasMethod(name) {
		return fmt.Errorf("%s references unknown method %q on %s", dirName, name, ci.typ.Name())
	}
	for _, arg := range args {
		if err := validateArgExpr(arg, ci, loopVars); err != nil {
			return fmt.Errorf("%s %q: %w", dirName, expr, err)
		}
	}
	return nil
}

func validateKeydownExpr(expr string, ci *componentInfo, loopVars map[string]*loopVarInfo) error {
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

func validateFieldExpr(expr string, ci *componentInfo, loopVars map[string]*loopVarInfo) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fmt.Errorf("empty directive expression")
	}
	if isLiteral(expr) {
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
	if ci.hasField(root) {
		// Validate remaining path against the field's type
		if len(parts) > 1 {
			f, _ := ci.typ.FieldByName(root)
			return validateTypePath(f.Type, parts[1:], expr)
		}
		return nil
	}

	return fmt.Errorf("directive references unknown field %q (expression: %q) on %s", root, expr, ci.typ.Name())
}

// validateTypePath walks a dotted path through struct types.
// e.g., for type Todo with field Text, validateTypePath(Todo, ["Text"], ...) succeeds.
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

func validateArgExpr(arg string, ci *componentInfo, loopVars map[string]*loopVarInfo) error {
	arg = strings.TrimSpace(arg)
	if isLiteral(arg) {
		return nil
	}

	root := arg
	if idx := strings.Index(arg, "."); idx != -1 {
		root = arg[:idx]
	}

	if _, ok := loopVars[root]; ok {
		return nil
	}
	if ci.hasField(root) {
		return nil
	}

	return fmt.Errorf("unknown argument %q", arg)
}

func isLiteral(s string) bool {
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

// hasField checks if the component struct has an exported field with the given name.
func (ci *componentInfo) hasField(name string) bool {
	f, ok := ci.typ.FieldByName(name)
	if !ok {
		return false
	}
	return f.IsExported() && f.Name != "Component"
}

// hasMethod checks if the component has an exported method with the given name.
func (ci *componentInfo) hasMethod(name string) bool {
	return ci.value.MethodByName(name).IsValid()
}
