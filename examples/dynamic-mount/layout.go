package main

import "github.com/anupshinde/godom"

type Widget struct {
	Name string
	ID   int
}

type Layout struct {
	godom.Component
	Widgets []Widget
	nextID  int
}

func (l *Layout) AddCounter() {
	l.nextID++
	l.Widgets = append(l.Widgets, Widget{Name: "counter", ID: l.nextID})
}

func (l *Layout) AddClock() {
	l.nextID++
	l.Widgets = append(l.Widgets, Widget{Name: "clock", ID: l.nextID})
}
