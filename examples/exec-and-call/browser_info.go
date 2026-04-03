package main

import (
	"encoding/json"
	"fmt"

	"github.com/anupshinde/godom"
)

// BrowserInfo displays browser state fetched via ExecJS.
type BrowserInfo struct {
	godom.Component
	URL       string
	Path      string
	UserAgent string
	Viewport  string
	Ready     bool
}

// FetchInfo queries the browser for current state via ExecJS.
func (b *BrowserInfo) FetchInfo() {
	b.ExecJS("({url: location.href, path: location.pathname, ua: navigator.userAgent, vw: window.innerWidth, vh: window.innerHeight})", func(result []byte, err string) {
		if err != "" {
			b.URL = "Error: " + err
			b.Refresh()
			return
		}
		var info struct {
			URL  string `json:"url"`
			Path string `json:"path"`
			UA   string `json:"ua"`
			VW   int    `json:"vw"`
			VH   int    `json:"vh"`
		}
		if e := json.Unmarshal(result, &info); e != nil {
			b.URL = "Parse error: " + e.Error()
			b.Refresh()
			return
		}
		b.URL = info.URL
		b.Path = info.Path
		b.UserAgent = info.UA
		b.Viewport = fmt.Sprintf("%d x %d", info.VW, info.VH)
		b.Ready = true
		b.Refresh()
	})
}

// Navigate uses ExecJS to redirect the browser to a different page.
func (b *BrowserInfo) Navigate(path string) {
	b.ExecJS(fmt.Sprintf("window.location.href = %q", path), func(result []byte, err string) {
		// No result needed — browser will navigate
	})
}

// GoToCatalog navigates to the catalog page.
func (b *BrowserInfo) GoToCatalog() {
	b.Navigate("/catalog")
}
