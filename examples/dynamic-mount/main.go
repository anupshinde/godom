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

	counter := &Counter{Count: 0, Step: 1}
	eng.Register("counter", counter, "components/counter/index.html")

	clock := &Clock{}
	eng.Register("clock", clock, "components/clock/index.html")

	go clock.startClock()

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

	fmt.Println("dynamic-mount example running")
	log.Fatal(eng.ListenAndServe())
}
