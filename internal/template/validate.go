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
var directiveRe = regexp.MustCompile(`g-(text|html|bind|value|click|keydown|mousedown|mousemove|mouseup|wheel|scroll|for|if|show|hide|checked|class:[a-zA-Z0-9_-]+|attr:[a-zA-Z0-9_-]+|style:[a-zA-Z0-9_-]+|prop:[a-zA-Z0-9_-]+|plugin:[a-zA-Z0-9_-]+|draggable(?::[a-zA-Z0-9_-]+)?|dropzone|drop(?::[a-zA-Z0-9_-]+)?)\s*=\s*"([^"]*)"`)

// gForRe matches g-for attributes to extract loop variable names.
var gForRe = regexp.MustCompile(`g-for\s*=\s*"([^"]*)"`)

// loopVarInfo holds the type info for a loop variable.
type loopVarInfo struct {
	itemType reflect.Type // element type of the array/slice
}

// ValidateDirectives parses HTML for g-* directives and validates them
// against the component's struct fields and methods. Called at Mount() time.
func ValidateDirectives(htmlStr string, ci *component.Info) error {
	loopVars := collectLoopVars(htmlStr, ci)

	matches := directiveRe.FindAllStringSubmatch(htmlStr, -1)
	for _, m := range matches {
		dirType := m[1]
		expr := m[2]

		// Normalize group variants: "drop:canvas" → "drop", "draggable:palette" → "draggable"
		baseDirType := dirType
		if strings.HasPrefix(dirType, "drop:") {
			baseDirType = "drop"
		} else if strings.HasPrefix(dirType, "draggable:") {
			baseDirType = "draggable"
		}

		switch baseDirType {
		case "for":
			if err := validateForExpr(expr, ci, loopVars); err != nil {
				return err
			}
		case "bind":
			if err := validateBindExpr(expr, ci, loopVars); err != nil {
				return err
			}
		case "click":
			if err := validateMethodRef("g-click", expr, ci, loopVars); err != nil {
				return err
			}
		case "keydown":
			if err := validateKeydownExpr(expr, ci, loopVars); err != nil {
				return err
			}
		case "mousedown", "mousemove", "mouseup", "wheel", "scroll":
			if err := validateMethodRef("g-"+dirType, expr, ci, loopVars); err != nil {
				return err
			}
		case "drop":
			if err := validateMethodRef("g-drop", expr, ci, loopVars); err != nil {
				return err
			}
		default:
			if err := validateFieldExpr(expr, ci, loopVars); err != nil {
				return err
			}
		}
	}

	return nil
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
	// Handle bracket syntax: "Inputs[first]" → root is "Inputs"
	if idx := strings.Index(root, "["); idx != -1 {
		root = root[:idx]
	}

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

	// Expressions containing operators are handled by expr-lang at runtime.
	// Skip validation here — expr-lang will report errors if invalid.
	if containsOperator(expr) {
		return nil
	}

	// Expressions with parentheses are method calls — validate the method name.
	// Handles "Summary()" (zero-arg) and "Add(3, 4)" (with args).
	if parenIdx := strings.Index(expr, "("); parenIdx != -1 {
		methodName := expr[:parenIdx]
		if ci.HasMethod(methodName) {
			return nil
		}
		return fmt.Errorf("directive references unknown method %q (expression: %q) on %s", methodName, expr, ci.Typ.Name())
	}

	parts := strings.Split(expr, ".")
	root := parts[0]

	// Check if it's a loop variable (loop vars never use bracket syntax)
	if lv, ok := loopVars[root]; ok {
		// Validate remaining path against the loop variable's type
		if len(parts) > 1 && lv.itemType != nil && lv.itemType.Kind() == reflect.Struct {
			return validateTypePath(lv.itemType, parts[1:], expr)
		}
		return nil
	}

	// Handle bracket syntax: "Inputs[first]" → field root is "Inputs"
	fieldRoot := root
	if idx := strings.Index(root, "["); idx != -1 {
		fieldRoot = root[:idx]
	}

	// Check if it's a component field
	if ci.HasField(fieldRoot) {
		// Validate remaining path against the field's type
		if len(parts) > 1 && fieldRoot == root {
			f, _ := ci.Typ.FieldByName(fieldRoot)
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

// containsOperator returns true if the expression contains comparison or
// logical operators, indicating it should be handled by expr-lang.
func containsOperator(expr string) bool {
	// Check for comparison operators
	for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		if strings.Contains(expr, op) {
			return true
		}
	}
	// Check for logical keyword operators (word boundaries via spaces)
	for _, kw := range []string{" and ", " or ", " not "} {
		if strings.Contains(expr, kw) {
			return true
		}
	}
	// "not " at the start
	if strings.HasPrefix(expr, "not ") {
		return true
	}
	return false
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
