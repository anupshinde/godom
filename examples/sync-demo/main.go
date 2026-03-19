package main

import (
	"embed"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Component
	BoxTop  string
	BoxLeft string
}

func main() {
	app := &App{BoxTop: "20px", BoxLeft: "20px"}

	go func() {
		for range time.Tick(2 * time.Second) {
			app.BoxTop = fmt.Sprintf("%dpx", rand.Intn(400))
			app.BoxLeft = fmt.Sprintf("%dpx", rand.Intn(400))
			fmt.Printf("box → top:%s left:%s\n", app.BoxTop, app.BoxLeft)
			app.Refresh("BoxTop", "BoxLeft")
		}
	}()

	eng := godom.NewEngine()
	eng.Mount(app, ui)
	log.Fatal(eng.Start())
}
