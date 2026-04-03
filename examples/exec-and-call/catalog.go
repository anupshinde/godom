package main

import "github.com/anupshinde/godom"

// Category represents an item in the tree.
type Category struct {
	ID          string
	Name        string
	Description string
	Children    []Category
}

// Catalog is the component that owns the tree and detail view.
type Catalog struct {
	godom.Component
	Categories   []Category
	Tree         TreeData // plugin data for g-plugin:tree
	SelectedName string
	SelectedDesc string
}

// SelectCategory is called by godom.call from the Shoelace tree plugin
// when a tree item is clicked.
func (c *Catalog) SelectCategory(id string) {
	cat := c.findCategory(id, c.Categories)
	if cat != nil {
		c.SelectedName = cat.Name
		c.SelectedDesc = cat.Description
	} else {
		c.SelectedName = ""
		c.SelectedDesc = "Select a category from the tree."
	}
}

func (c *Catalog) findCategory(id string, cats []Category) *Category {
	for i := range cats {
		if cats[i].ID == id {
			return &cats[i]
		}
		if found := c.findCategory(id, cats[i].Children); found != nil {
			return found
		}
	}
	return nil
}

// SampleCategories returns a sample category tree.
func SampleCategories() []Category {
	return []Category{
		{
			ID:          "languages",
			Name:        "Languages",
			Description: "Programming languages used in modern software development.",
			Children: []Category{
				{ID: "go", Name: "Go", Description: "A statically typed, compiled language designed at Google. Known for simplicity, concurrency via goroutines, and fast compilation. Powers Docker, Kubernetes, and godom."},
				{ID: "javascript", Name: "JavaScript", Description: "The language of the web. Runs in every browser, powers Node.js on the server. Dynamic, prototype-based, and ubiquitous."},
				{ID: "rust", Name: "Rust", Description: "A systems language focused on safety and performance. Ownership model prevents memory bugs at compile time. Used in Firefox, Linux kernel, and CLI tools."},
				{ID: "python", Name: "Python", Description: "Readable, versatile, and beginner-friendly. Dominates data science, ML, and scripting. Slow but expressive."},
			},
		},
		{
			ID:          "frameworks",
			Name:        "Frameworks",
			Description: "Software frameworks for building applications.",
			Children: []Category{
				{ID: "godom", Name: "godom", Description: "Local GUI apps in Go using the browser as the rendering engine. Virtual DOM in Go, WebSocket to browser, single binary output. You're looking at it right now."},
				{ID: "react", Name: "React", Description: "A JavaScript library for building user interfaces. Component-based, virtual DOM, maintained by Meta. The most popular frontend framework."},
				{ID: "htmx", Name: "htmx", Description: "Access modern browser features directly from HTML. No JavaScript needed for most interactions. Extends HTML with attributes like hx-get, hx-post."},
			},
		},
		{
			ID:          "tools",
			Name:        "Tools",
			Description: "Developer tools and utilities.",
			Children: []Category{
				{ID: "git", Name: "Git", Description: "Distributed version control. Tracks changes, enables collaboration, powers GitHub/GitLab. Every developer uses it."},
				{ID: "docker", Name: "Docker", Description: "Container runtime. Packages apps with their dependencies into portable containers. Build once, run anywhere."},
				{ID: "vscode", Name: "VS Code", Description: "A free, extensible code editor by Microsoft. Built on Electron. Extensions for every language. The most popular editor."},
			},
		},
	}
}
