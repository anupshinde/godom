package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/anupshinde/godom"
)

//go:embed components
var components embed.FS

//go:embed pages
var pages embed.FS

//go:embed tree_plugin.js
var treePluginJS string

var (
	infoTmpl    = template.Must(template.ParseFS(pages, "pages/layout/base.html", "pages/info/page.html"))
	catalogTmpl = template.Must(template.ParseFS(pages, "pages/layout/base.html", "pages/catalog/page.html"))
)

type PageData struct {
	Title string
	Page  string
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(components)
	eng.RegisterPlugin("tree", treePluginJS)
	// eng.DisableExecJS = true // uncomment to disable ExecJS (server won't send, bridge won't execute)

	// Browser info component — uses ExecJS
	info := &BrowserInfo{}
	eng.Register("browserinfo", info, "components/browser-info/index.html")

	// Catalog component — uses godom.call from Shoelace tree plugin
	cats := SampleCategories()
	catalog := &Catalog{
		Categories:   cats,
		Tree:         categoriesToTreeData(cats),
		SelectedDesc: "Select a category from the tree.",
	}
	eng.Register("catalog", catalog, "components/catalog/index.html")

	// User owns the mux and routes.
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		infoTmpl.Execute(w, &PageData{Title: "Browser Info", Page: "info"})
	})

	mux.HandleFunc("/catalog", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		catalogTmpl.Execute(w, &PageData{Title: "Catalog", Page: "catalog"})
	})

	eng.SetMux(mux, nil)
	if err := eng.Run(); err != nil {
		log.Fatal(err)
	}

	log.Fatal(eng.ListenAndServe())
}
