package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type Layout struct {
	godom.Island
	Title string
}

type Counter struct {
	godom.Island
	Count int
	Step  int
}

func (c *Counter) Increment() {
	c.Count += c.Step
}

func (c *Counter) Decrement() {
	c.Count -= c.Step
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)

	counter := &Counter{Step: 1}
	counter.TargetName = "counter_single"
	counter.Template = "ui/counter/index.html"
	eng.Register(counter)

	layout := &Layout{Title: "Same Component Test"}
	layout.Template = "ui/layout/index.html"
	log.Fatal(eng.QuickServe(layout))
}
