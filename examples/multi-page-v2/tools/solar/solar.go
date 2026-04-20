// Package solar is a 3D solar-system island: Go builds per-frame draw commands,
// a small canvas2D plugin on the bridge paints them.
package solar

import (
	_ "embed"
	"math"
	"sort"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed canvas-bridge.js
var canvasBridgeJS string

const (
	canvasWidth  = 1200
	canvasHeight = 750
)

// Plugin is passed to eng.Use() to register the canvas3d plugin.
var Plugin godom.PluginFunc = func(e *godom.Engine) {
	e.RegisterPlugin("canvas3d", canvasBridgeJS)
}

type Scene struct {
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	Commands []DrawCmd `json:"commands"`
}

type Solar struct {
	godom.Island
	SolarSystem Scene
	cam         *camera
	bodies      []*body
	followBody  string
	dragging    bool
	lastX       float64
	lastY       float64
}

func (a *Solar) ZoomIn()      { a.cam.Distance = math.Max(10, a.cam.Distance-50) }
func (a *Solar) ZoomOut()     { a.cam.Distance = math.Min(2000, a.cam.Distance+50) }
func (a *Solar) PanUp()       { a.cam.Tilt = math.Min(1.4, a.cam.Tilt+0.1) }
func (a *Solar) PanDown()     { a.cam.Tilt = math.Max(0.1, a.cam.Tilt-0.1) }
func (a *Solar) RotateLeft()  { a.cam.Rotation -= 0.1 }
func (a *Solar) RotateRight() { a.cam.Rotation += 0.1 }

func (a *Solar) Follow(name string) {
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

func (a *Solar) MouseDown(x, y float64) {
	a.dragging = true
	a.lastX = x
	a.lastY = y
}

func (a *Solar) MouseMove(x, y float64) {
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

func (a *Solar) MouseUp(x, y float64) {
	a.dragging = false
}

func (a *Solar) Wheel(deltaY float64) {
	a.cam.Distance = math.Max(10, math.Min(2000, a.cam.Distance+deltaY*0.5))
}

func (a *Solar) Run() {
	a.bodies = newBodies()
	a.cam = &camera{
		Distance: 800, Tilt: 0.5, Rotation: -0.2, FOV: 700,
		Width: canvasWidth, Height: canvasHeight,
	}

	ticker := time.NewTicker(16 * time.Millisecond) // ~60fps
	for range ticker.C {
		dt := 0.016
		for _, b := range a.bodies {
			b.update(dt)
		}
		if a.followBody != "" {
			if pos, ok := findBodyPos(a.bodies, a.followBody); ok {
				a.cam.Center = pos
			}
		} else {
			a.cam.Center = vec3{}
		}
		cmds := make([]DrawCmd, 0, 20)
		origin := vec3{}
		for _, b := range a.bodies {
			b.collect(origin, a.cam, &cmds)
		}
		sort.Slice(cmds, func(i, j int) bool {
			if cmds[i].Glow != cmds[j].Glow {
				return cmds[i].Glow
			}
			return cmds[i].Radius < cmds[j].Radius
		})
		a.SolarSystem = Scene{Width: canvasWidth, Height: canvasHeight, Commands: cmds}
		a.Refresh()
	}
}

func New() *Solar {
	return &Solar{
		Island: godom.Island{
			TargetName: "solar",
			Template:   "island-templates/solar/index.html",
		},
	}
}
