package main

import (
	"bytes"
	"embed"
	"log"
	"math"
	"os"
	"time"

	"github.com/anupshinde/godom"
	"github.com/yuin/goldmark"
)

//go:embed ui
var ui embed.FS

const modifiedPath = "source-modified.md"

type App struct {
	godom.Island
	Markdown      string
	EditorScroll  float64 // scroll target for textarea (set by preview scroll)
	PreviewScroll float64 // scroll target for preview pane (set by editor scroll)
	EditorOpen    bool    // whether editor pane is visible (mobile toggle)
	ToggleLabel   string  // button text
	SaveMessage   string  // transient save feedback
}

func (a *App) ShowSaved() bool {
	return a.SaveMessage != ""
}

func (a *App) Preview() string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(a.Markdown), &buf); err != nil {
		return "<p>Error rendering markdown</p>"
	}
	return buf.String()
}

func (a *App) ToggleEditor() {
	a.EditorOpen = !a.EditorOpen
	if a.EditorOpen {
		a.ToggleLabel = "Hide Editor"
	} else {
		a.ToggleLabel = "Edit"
	}
}

func (a *App) Save() {
	if err := os.WriteFile(modifiedPath, []byte(a.Markdown), 0644); err != nil {
		a.SaveMessage = "Error: " + err.Error()
	} else {
		a.SaveMessage = "Saved"
	}
	go func() {
		time.Sleep(2 * time.Second)
		a.SaveMessage = ""
		a.Refresh()
	}()
}

func (a *App) OnEditorScroll(scrollTop, scrollHeight, clientHeight int) {
	if scrollHeight <= clientHeight {
		return
	}
	ratio := float64(scrollTop) / float64(scrollHeight-clientHeight)
	if math.Abs(ratio-a.EditorScroll) < 0.005 {
		return
	}
	a.PreviewScroll = ratio
}

func (a *App) OnPreviewScroll(scrollTop, scrollHeight, clientHeight int) {
	if scrollHeight <= clientHeight {
		return
	}
	ratio := float64(scrollTop) / float64(scrollHeight-clientHeight)
	if math.Abs(ratio-a.PreviewScroll) < 0.005 {
		return
	}
	a.EditorScroll = ratio
}

func main() {
	// Try modified source first, fall back to embedded demo
	content, err := os.ReadFile(modifiedPath)
	if err != nil {
		content, _ = ui.ReadFile("ui/demo-source.md")
	}

	eng := godom.NewEngine()
	eng.SetFS(ui)
	app := &App{
		Markdown:    string(content),
		EditorOpen:  true,
		ToggleLabel: "Hide Editor",
	}
	app.Template = "ui/index.html"
	log.Fatal(eng.QuickServe(app))
}
