package main

import (
	"embed"
	"encoding/json"
	"log"
	"strings"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type FormField struct {
	Type        string
	Label       string
	Placeholder string
	Required    bool
	Options     []string
	HelpText    string
	Selected    bool

	// Type flags for conditional rendering in templates
	IsText     bool
	IsTextarea bool
	IsSelect   bool
	IsCheckbox bool
	IsNumber   bool
	IsDate     bool
	IsSection  bool
}

type FormBuilder struct {
	godom.Component

	Title  string
	Fields []FormField

	// Selection
	Selected     int
	HasSelection bool
	NoSelection  bool // inverse of HasSelection
	HasFields    bool // true when Fields is non-empty
	ShowEmpty    bool // true when Fields is empty (inverse of HasFields)
	Editing      bool // true when not in preview mode

	// Preview
	Preview        bool
	PreviewBtnText string

	// Config panel (top-level for g-bind)
	CfgType       string
	CfgLabel      string
	CfgPlaceholder string
	CfgRequired   bool
	CfgHelpText   string
	CfgOptions    string
	CfgHasOptions bool
	CfgHasPlaceholder bool

	// Export
	ShowExport bool
	ExportJSON string
}

var defaultLabels = map[string]string{
	"text":     "Text Input",
	"textarea": "Text Area",
	"select":   "Dropdown",
	"checkbox": "Checkbox Group",
	"number":   "Number Input",
	"date":     "Date Picker",
	"section":  "Section Header",
}

func newField(typ string) FormField {
	f := FormField{
		Type:  typ,
		Label: defaultLabels[typ],
	}
	switch typ {
	case "text":
		f.IsText = true
		f.Placeholder = "Enter text..."
	case "textarea":
		f.IsTextarea = true
		f.Placeholder = "Enter text..."
	case "select":
		f.IsSelect = true
		f.Options = []string{"Option 1", "Option 2", "Option 3"}
	case "checkbox":
		f.IsCheckbox = true
		f.Options = []string{"Choice 1", "Choice 2", "Choice 3"}
	case "number":
		f.IsNumber = true
		f.Placeholder = "0"
	case "date":
		f.IsDate = true
	case "section":
		f.IsSection = true
		f.Label = "Section Title"
	}
	return f
}

// applyConfig copies Cfg* fields back to Fields[Selected].
func (f *FormBuilder) applyConfig() {
	if f.Selected < 0 || f.Selected >= len(f.Fields) {
		return
	}
	fld := &f.Fields[f.Selected]
	fld.Label = f.CfgLabel
	fld.Placeholder = f.CfgPlaceholder
	fld.Required = f.CfgRequired
	fld.HelpText = f.CfgHelpText
	if f.CfgHasOptions {
		fld.Options = splitOptions(f.CfgOptions)
	}
}

func splitOptions(s string) []string {
	var opts []string
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			opts = append(opts, o)
		}
	}
	return opts
}

// AddField is called when a palette item is dropped on the canvas.
func (f *FormBuilder) AddField(fieldType string) {
	f.applyConfig()
	f.Fields = append(f.Fields, newField(fieldType))
	f.HasFields = true
	f.ShowEmpty = false
}

// Reorder is called when a canvas field is dropped on another canvas field.
func (f *FormBuilder) Reorder(from, to float64, position string) {
	f.applyConfig()
	fr, t := int(from), int(to)
	if fr == t || fr < 0 || t < 0 || fr >= len(f.Fields) || t >= len(f.Fields) {
		return
	}
	// Track selected field
	wasSelected := f.Selected
	item := f.Fields[fr]
	f.Fields = append(f.Fields[:fr], f.Fields[fr+1:]...)

	if position == "below" && t <= fr {
		t++
		if t > len(f.Fields) {
			t = len(f.Fields)
		}
	}
	f.Fields = append(f.Fields[:t], append([]FormField{item}, f.Fields[t:]...)...)

	// Update selected index to follow the moved item
	if wasSelected == fr {
		f.Selected = t
	} else if wasSelected >= 0 {
		// Adjust for shift
		sel := wasSelected
		if fr < sel {
			sel--
		}
		if t <= sel {
			sel++
		}
		f.Selected = sel
	}
	f.updateSelectionFlags()
}

// SelectField selects a field for editing in the config panel.
func (f *FormBuilder) SelectField(i int) {
	f.applyConfig()
	// Clear old selection
	if f.Selected >= 0 && f.Selected < len(f.Fields) {
		f.Fields[f.Selected].Selected = false
	}
	f.Selected = i
	f.Fields[i].Selected = true
	f.HasSelection = true
	f.NoSelection = false
	// Load config
	fld := f.Fields[i]
	f.CfgType = fld.Type
	f.CfgLabel = fld.Label
	f.CfgPlaceholder = fld.Placeholder
	f.CfgRequired = fld.Required
	f.CfgHelpText = fld.HelpText
	f.CfgHasOptions = fld.IsSelect || fld.IsCheckbox
	f.CfgHasPlaceholder = !fld.IsSection && !fld.IsCheckbox
	if fld.Options != nil {
		f.CfgOptions = strings.Join(fld.Options, ", ")
	} else {
		f.CfgOptions = ""
	}
}

// Deselect clears the selection.
func (f *FormBuilder) Deselect() {
	f.applyConfig()
	if f.Selected >= 0 && f.Selected < len(f.Fields) {
		f.Fields[f.Selected].Selected = false
	}
	f.Selected = -1
	f.HasSelection = false
	f.NoSelection = true
}

// DeleteField removes a field by index.
func (f *FormBuilder) DeleteField(i int) {
	f.applyConfig()
	if i < 0 || i >= len(f.Fields) {
		return
	}
	f.Fields = append(f.Fields[:i], f.Fields[i+1:]...)
	if len(f.Fields) == 0 {
		f.Fields = nil
		f.HasFields = false
		f.ShowEmpty = true
	}
	// Update selection
	if f.Selected == i {
		f.Selected = -1
		f.HasSelection = false
	f.NoSelection = true
	} else if f.Selected > i {
		f.Selected--
	}
	f.updateSelectionFlags()
}

// RemoveField is called when a canvas field is dropped on the trash zone.
func (f *FormBuilder) RemoveField(from float64) {
	f.DeleteField(int(from))
}

// TogglePreview toggles between builder and preview mode.
func (f *FormBuilder) TogglePreview() {
	f.applyConfig()
	f.Preview = !f.Preview
	f.Editing = !f.Preview
	if f.Preview {
		f.PreviewBtnText = "Edit"
	} else {
		f.PreviewBtnText = "Preview"
	}
}

// Export generates JSON and shows the export modal.
func (f *FormBuilder) Export() {
	f.applyConfig()
	type exportField struct {
		Type        string   `json:"type"`
		Label       string   `json:"label"`
		Placeholder string   `json:"placeholder,omitempty"`
		Required    bool     `json:"required,omitempty"`
		Options     []string `json:"options,omitempty"`
		HelpText    string   `json:"helpText,omitempty"`
	}
	type exportForm struct {
		Title  string        `json:"title"`
		Fields []exportField `json:"fields"`
	}
	out := exportForm{Title: f.Title}
	for _, fld := range f.Fields {
		out.Fields = append(out.Fields, exportField{
			Type:        fld.Type,
			Label:       fld.Label,
			Placeholder: fld.Placeholder,
			Required:    fld.Required,
			Options:     fld.Options,
			HelpText:    fld.HelpText,
		})
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	f.ExportJSON = string(data)
	f.ShowExport = true
}

// CloseExport closes the export modal.
func (f *FormBuilder) CloseExport() {
	f.ShowExport = false
}

// ToggleRequired toggles the required flag in the config panel.
func (f *FormBuilder) ToggleRequired() {
	f.CfgRequired = !f.CfgRequired
}

func (f *FormBuilder) updateSelectionFlags() {
	// Clear all selection flags, then set the current one
	for i := range f.Fields {
		f.Fields[i].Selected = false
	}
	if f.Selected >= 0 && f.Selected < len(f.Fields) {
		f.Fields[f.Selected].Selected = true
		f.HasSelection = true
	f.NoSelection = false
	} else {
		f.HasSelection = false
	f.NoSelection = true
	}
}

func main() {
	app := godom.New()
	app.Port = 8084
	app.Mount(&FormBuilder{
		Title:          "My Form",
		Selected:       -1,
		Editing:        true,
		ShowEmpty:      true,
		NoSelection:    true,
		PreviewBtnText: "Preview",
	}, ui)
	log.Fatal(app.Start())
}
