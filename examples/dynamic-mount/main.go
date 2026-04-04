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
var jsPageHTML string

//go:embed godom.html
var godomPageHTML string

func main() {
	eng := godom.NewEngine()
	eng.SetFS(components)

	counter := &Counter{Count: 0, Step: 1}
	counter.TargetName = "counter"
	counter.Template = "components/counter/index.html"

	clock := &Clock{}
	clock.TargetName = "clock"
	clock.Template = "components/clock/index.html"

	layout := &Layout{}
	layout.TargetName = "layout"
	layout.Template = "components/layout/index.html"

	eng.Register(counter, clock, layout)

	go clock.startClock()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(jsPageHTML))
	})
	mux.HandleFunc("/godom", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(godomPageHTML))
	})

	eng.SetMux(mux, nil)
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("dynamic-mount example running")
	log.Fatal(eng.ListenAndServe())
}
