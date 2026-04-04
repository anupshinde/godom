package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

//go:embed xterm-adapter.js
var xtermAdapterJS string

// App is the root godom component.
type App struct {
	godom.Component
	Terminal TerminalConfig
}

// TerminalConfig is passed to the xterm plugin so the browser
// knows where to connect for raw PTY I/O.
type TerminalConfig struct {
	WSPort int    `json:"wsPort"`
	Token  string `json:"token"`
}

func main() {
	token := randomToken()
	termPort := startTerminalServer(token)

	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.RegisterPlugin("xterm", xtermAdapterJS)

	root := &App{
		Terminal: TerminalConfig{
			WSPort: termPort,
			Token:  token,
		},
	}
	root.Template = "ui/index.html"
	log.Fatal(eng.QuickServe(root))
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("failed to generate token: %v", err)
	}
	return hex.EncodeToString(b)
}
