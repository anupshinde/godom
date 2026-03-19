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

type TodoItem struct {
	godom.Component
	Text  string `godom:"prop"`
	Done  bool   `godom:"prop"`
	Index int    `godom:"prop"`
}

func (t *TodoItem) Toggle() {
	// TODO: replace Emit with new communication mechanism
	// t.Emit("ToggleTodo", t.Index)
}

func (t *TodoItem) Remove() {
	// TODO: replace Emit with new communication mechanism
	// t.Emit("RemoveTodo", t.Index)
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

func (t *TodoApp) ToggleTodo(i int) {
	t.Todos[i].Done = !t.Todos[i].Done
}

func (t *TodoApp) RemoveTodo(i int) {
	t.Todos = append(t.Todos[:i], t.Todos[i+1:]...)
}

func main() {
	eng := godom.NewEngine()
	eng.Port = 8082
	eng.RegisterComponent("todo-item", &TodoItem{})
	eng.Mount(&TodoApp{}, ui)
	log.Fatal(eng.Start())
}
