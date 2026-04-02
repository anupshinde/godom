package main

import (
	"embed"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/anupshinde/godom"
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
	bodies      []*Body
	followBody  string
	dragging    bool
	lastX       float64
	lastY       float64
}

func (a *App) ZoomIn()      { a.cam.Distance = math.Max(10, a.cam.Distance-50) }
func (a *App) ZoomOut()     { a.cam.Distance = math.Min(2000, a.cam.Distance+50) }
func (a *App) PanUp()       { a.cam.Tilt = math.Min(1.4, a.cam.Tilt+0.1) }
func (a *App) PanDown()     { a.cam.Tilt = math.Max(0.1, a.cam.Tilt-0.1) }
func (a *App) RotateLeft()  { a.cam.Rotation -= 0.1 }
func (a *App) RotateRight() { a.cam.Rotation += 0.1 }

func (a *App) Follow(name string) {
	if name == "Free" {
		a.followBody = ""
		a.cam.Distance = 800
		a.cam.Tilt = 0.5
		return
	}
	a.followBody = name
	a.cam.Distance = 40
	a.cam.Tilt = 0.3
}

// Mouse controls
func (a *App) MouseDown(x, y float64) {
	a.dragging = true
	a.lastX = x
	a.lastY = y
}

func (a *App) MouseMove(x, y float64) {
	if !a.dragging {
		return
	}
	dx := x - a.lastX
	dy := y - a.lastY
	a.cam.Rotation += dx * 0.005
	a.cam.Tilt = math.Max(0.1, math.Min(1.4, a.cam.Tilt-dy*0.005))
	a.lastX = x
	a.lastY = y
}

func (a *App) MouseUp(x, y float64) {
	a.dragging = false
}

func (a *App) Wheel(deltaY float64) {
	a.cam.Distance = math.Max(10, math.Min(2000, a.cam.Distance+deltaY*0.5))
}

func (a *App) run() {
	a.bodies = NewSolarSystem()

	a.cam = &Camera{
		Distance: 800,
		Tilt:     0.5,
		Rotation: -0.2,
		FOV:      700,
		Width:    canvasWidth,
		Height:   canvasHeight,
	}

	ticker := time.NewTicker(16 * time.Millisecond) // ~60fps
	for range ticker.C {
		dt := 0.016

		for _, b := range a.bodies {
			b.Update(dt)
		}

		// Update camera center if following a body
		if a.followBody != "" {
			if pos, ok := FindBodyPosition(a.bodies, a.followBody); ok {
				a.cam.Center = pos
			}
		} else {
			a.cam.Center = Vec3{}
		}

		cmds := make([]DrawCmd, 0, 20)
		origin := Vec3{}
		for _, b := range a.bodies {
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
	eng := godom.NewEngine()
	eng.SetFS(ui)
	eng.RegisterPlugin("canvas3d", canvasBridgeJS)

	root := &App{}
	go root.run()

	fmt.Println("Solar system — 3D engine in Go, Canvas 2D rendering")
	log.Fatal(eng.QuickServe(root, "ui/index.html"))
}
