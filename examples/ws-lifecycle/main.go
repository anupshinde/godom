package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"

	"github.com/anupshinde/godom"
)

//go:embed components
var components embed.FS

//go:embed index.html
var indexHTML string

func main() {
	eng := godom.NewEngine()
	eng.SetFS(components)
	eng.NoAuth = true

	// Background component — registered but not rendered on the page.
	// Proves godom is alive even with no g-component target in the DOM.
	ticker := NewTicker()
	ticker.TargetName = "ticker"
	ticker.Template = "components/ticker/index.html"
	eng.Register(ticker)

	go ticker.Run()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})

	eng.SetMux(mux, nil)
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("ws-lifecycle — stop/restart server to test reconnect")
	log.Fatal(eng.ListenAndServe())
}
