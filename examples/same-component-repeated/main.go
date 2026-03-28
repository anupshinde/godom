package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type Layout struct {
	godom.Component
	Title string
}

type Counter struct {
	godom.Component
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
	eng.SetUI(ui)

	// Same instance, registered twice with different names
	counter := &Counter{Step: 1}
	eng.Register("counter_single", counter, "ui/counter/index.html")

	layout := &Layout{Title: "Same Component Test"}
	eng.Mount(layout, "ui/layout/index.html")

	log.Fatal(eng.Start())
}
