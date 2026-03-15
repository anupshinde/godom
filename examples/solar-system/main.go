package main

import (
	"embed"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"godom"
)

//go:embed ui
var ui embed.FS

//go:embed canvas-bridge.js
var canvasBridgeJS string

const (
	canvasWidth  = 1200
	canvasHeight = 750
)

type Scene struct {
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	Commands []DrawCmd `json:"commands"`
}

type App struct {
	godom.Component
	SolarSystem Scene
	cam         *Camera
}

func (a *App) ZoomIn()      { a.cam.Distance = math.Max(200, a.cam.Distance-50) }
func (a *App) ZoomOut()     { a.cam.Distance = math.Min(2000, a.cam.Distance+50) }
func (a *App) PanUp()       { a.cam.Tilt = math.Min(1.4, a.cam.Tilt+0.1) }
func (a *App) PanDown()     { a.cam.Tilt = math.Max(0.1, a.cam.Tilt-0.1) }
func (a *App) RotateLeft()  { a.cam.Rotation -= 0.1 }
func (a *App) RotateRight() { a.cam.Rotation += 0.1 }

func (a *App) run() {
	bodies := NewSolarSystem()

	a.cam = &Camera{
		Distance: 800,
		Tilt:     0.5,
		Rotation: -0.2,
		FOV:      700,
		Width:    canvasWidth,
		Height:   canvasHeight,
	}

	ticker := time.NewTicker(33 * time.Millisecond) // ~30fps
	for range ticker.C {
		dt := 0.033

		for _, b := range bodies {
			b.Update(dt)
		}

		cmds := make([]DrawCmd, 0, 20)
		origin := Vec3{}
		for _, b := range bodies {
			b.Collect(origin, a.cam, &cmds)
		}

		sort.Slice(cmds, func(i, j int) bool {
			if cmds[i].Glow != cmds[j].Glow {
				return cmds[i].Glow
			}
			return cmds[i].Radius < cmds[j].Radius
		})

		a.SolarSystem = Scene{
			Width:    canvasWidth,
			Height:   canvasHeight,
			Commands: cmds,
		}
		a.Refresh()
	}
}

func main() {
	app := godom.New()
	app.Plugin("canvas3d", canvasBridgeJS)

	root := &App{}
	go root.run()

	fmt.Println("Solar system — 3D engine in Go, Canvas 2D rendering")
	app.Mount(root, ui)
	log.Fatal(app.Start())
}
