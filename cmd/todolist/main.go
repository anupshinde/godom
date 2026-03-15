package main

import (
	"embed"
	"log"

	"godom"
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

func main() {
	app := godom.New()
	app.Port = 8081
	app.Mount(&TodoApp{}, ui)
	log.Fatal(app.Start())
}
