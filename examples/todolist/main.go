package main

import (
	"embed"
	"log"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type Todo struct {
	Text string
	Done bool
}

type TodoApp struct {
	godom.Component
	InputText string
	Todos     []Todo
}

func (t *TodoApp) AddTodo() {
	if t.InputText == "" {
		return
	}
	t.Todos = append(t.Todos, Todo{Text: t.InputText})
	t.InputText = ""
}

func (t *TodoApp) Toggle(i int) {
	t.Todos[i].Done = !t.Todos[i].Done
}

func (t *TodoApp) Remove(i int) {
	t.Todos = append(t.Todos[:i], t.Todos[i+1:]...)
}

func (t *TodoApp) Reorder(from, to float64) {
	f, d := int(from), int(to)
	if f == d || f < 0 || d < 0 || f >= len(t.Todos) || d >= len(t.Todos) {
		return
	}
	item := t.Todos[f]
	t.Todos = append(t.Todos[:f], t.Todos[f+1:]...)
	t.Todos = append(t.Todos[:d], append([]Todo{item}, t.Todos[d:]...)...)
}

func main() {
	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.Port = 8081
	log.Fatal(eng.QuickServe(&TodoApp{}, "ui/index.html"))
}
