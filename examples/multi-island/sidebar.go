package main

import "github.com/anupshinde/godom"

// MenuItem represents a single sidebar menu entry.
type MenuItem struct {
	ID       string
	Icon     string
	Label    string
	Active   bool
	Inactive bool
}

// Sidebar is a navigation sidebar with menu items.
type Sidebar struct {
	godom.Island
	Items      []MenuItem
	ActiveID   string
	OnNavigate func(msg, kind string)
}

func NewSidebar() *Sidebar {
	items := []MenuItem{
		{ID: "dashboard", Icon: "\u25A0", Label: "Dashboard", Active: true},
		{ID: "counter", Icon: "\u25B6", Label: "Counter", Inactive: true},
		{ID: "clock", Icon: "\u25CB", Label: "Clock", Inactive: true},
		{ID: "ticker", Icon: "\u25B2", Label: "Ticker", Inactive: true},
		{ID: "users", Icon: "\u25C6", Label: "Users", Inactive: true},
		{ID: "analytics", Icon: "\u25AC", Label: "Analytics", Inactive: true},
		{ID: "settings", Icon: "\u2699", Label: "Settings", Inactive: true},
	}
	return &Sidebar{ActiveID: "dashboard", Items: items}
}

var toastTypes = map[string]string{
	"dashboard": "info",
	"counter":   "success",
	"clock":     "warning",
	"ticker":    "error",
	"users":     "info",
	"analytics": "success",
	"settings":  "warning",
}

var toastMessages = map[string]string{
	"dashboard": "Dashboard view is not implemented yet",
	"counter":   "Counter component is live below!",
	"clock":     "Clock is running — check the widget",
	"ticker":    "Stock data is simulated",
	"users":     "User management coming soon",
	"analytics": "Analytics module not available",
	"settings":  "Settings page under construction",
}

func (s *Sidebar) Navigate(id string) {
	s.ActiveID = id
	for i := range s.Items {
		s.Items[i].Active = s.Items[i].ID == id
		s.Items[i].Inactive = s.Items[i].ID != id
	}
	if s.OnNavigate != nil {
		msg := toastMessages[id]
		if msg == "" {
			msg = "\"" + id + "\" is just a demo item"
		}
		kind := toastTypes[id]
		s.OnNavigate(msg, kind)
	}
}
