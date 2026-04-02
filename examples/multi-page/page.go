package main

// PageData is template data for pages (plain struct, NOT a godom component).
type PageData struct {
	Title string
	Page  string // current page identifier, used for nav highlighting
}
