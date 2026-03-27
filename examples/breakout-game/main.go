package main

import (
	"embed"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

//go:embed gyro.js
var gyroJS string

//go:embed sfx.js
var sfxJS string

const (
	areaW = 1024.0
	areaH = 768.0

	paddleW = 120.0
	paddleH = 14.0
	paddleY = areaH - 30

	ballSize = 12.0

	brickCols = 12
	brickRows = 6
	brickW    = 72.0
	brickH    = 24.0
	brickGapX = 8.0
	brickGapY = 6.0
	brickTop  = 50.0

	initialSpeed = 4.0
)

var rowColors = []string{"#e74c3c", "#e67e22", "#f1c40f", "#2ecc71", "#3498db", "#9b59b6"}
var rowPoints = []int{6, 5, 4, 3, 2, 1}

type Brick struct {
	Left    string
	Top     string
	Width   string
	Height  string
	Color   string
	Visible bool
	Points  int
	// internal coords for collision
	x, y float64
}

type App struct {
	godom.Component

	// Ball (CSS strings for rendering)
	BallLeft    string
	BallTop     string
	BallHidden  bool   // true during death freeze
	BallReady   bool   // sitting on paddle, dimmed, not yet launched

	// Paddle
	PaddleLeft string

	// Impact flash at death line
	ImpactShow bool
	ImpactLeft string

	// Game state
	Bricks  []Brick
	Score   int
	Lives   int
	Message string
	Playing bool
	Paused  bool

	// internal state (unexported = not sent to browser)
	ballX, ballY float64
	ballDX       float64
	ballDY       float64
	paddleX      float64
	speed        float64
	bricksLeft     int
	calibrated     bool
	areaLeftOffset float64

	// Controller steering: click sets direction + timestamp,
	// game loop moves paddle with acceleration, auto-stops after timeout.
	steerDir   int       // -1 left, 0 none, 1 right
	steerStart time.Time // when current steering began
	steerLast  time.Time // last click timestamp (for timeout)

	// Gyroscope: tilt angle from deviceorientation, synced via hidden g-bind input.
	Tilt      string
	TiltPct   string // tilt mapped to 0-100% for indicator position
	GyroMode  string // "off", "portrait", "landscape"
	GyroBtn   string // button label
	Landscape bool   // true when gyro mode is landscape (for CSS rotation)

	// Controller presence: set by JS on tabs showing the controller view.
	// When true, game-view tabs skip sounds (controller plays them instead).
	ControllerActive string

	// Sound/vibration events: Go sets this, JS watches via MutationObserver.
	// Values: "brick", "paddle", "wall", "life", "gameover", "win"
	SoundEvent string

	// Life-lost sequence timing
	freezeUntil  time.Time // physics frozen until this time
	respawnAt    time.Time // ball reappears on paddle at this time
	impactAt     time.Time // when impact flash started
	waitingSpawn bool      // true between death and respawn
}

func (a *App) resetBall() {
	// Place ball on the paddle, ready to launch
	a.ballX = a.paddleX + paddleW/2
	a.ballY = paddleY - ballSize/2 - 1
	// Random launch angle
	a.ballDX = 2 + (rand.Float64()-0.5)*3
	a.ballDY = -3
	a.speed = initialSpeed
	a.BallReady = true
	a.BallHidden = false
}

func (a *App) initBricks() {
	a.Bricks = make([]Brick, 0, brickRows*brickCols)
	offsetX := (areaW - float64(brickCols)*(brickW+brickGapX) + brickGapX) / 2

	for row := range brickRows {
		for col := range brickCols {
			x := offsetX + float64(col)*(brickW+brickGapX)
			y := brickTop + float64(row)*(brickH+brickGapY)
			a.Bricks = append(a.Bricks, Brick{
				Left:    px(x),
				Top:     px(y),
				Width:   px(brickW),
				Height:  px(brickH),
				Color:   rowColors[row],
				Visible: true,
				Points:  rowPoints[row],
				x:       x,
				y:       y,
			})
		}
	}
	a.bricksLeft = brickRows * brickCols
}

func (a *App) StartGame() {
	a.Score = 0
	a.Lives = 3
	a.Playing = true
	a.Paused = false
	a.Message = ""
	a.ImpactShow = false
	a.initBricks()
	a.paddleX = (areaW - paddleW) / 2
	a.calibrated = false
	a.resetBall()
	a.syncCSS()
}

func (a *App) PauseToggle() {
	if a.Playing {
		a.Paused = !a.Paused
		if a.Paused {
			a.Message = "PAUSED"
		} else {
			a.Message = ""
		}
	}
}

func (a *App) MoveLeft() {
	a.steer(-1)
}

func (a *App) MoveRight() {
	a.steer(1)
}

func (a *App) steer(dir int) {
	now := time.Now()
	if a.steerDir != dir {
		// Changed direction — reset acceleration
		a.steerDir = dir
		a.steerStart = now
	}
	a.steerLast = now
}

func (a *App) GyroToggle() {
	switch a.GyroMode {
	case "off":
		a.GyroMode = "portrait"
		a.GyroBtn = "GYRO: PORTRAIT"
		a.Landscape = false
	case "portrait":
		a.GyroMode = "landscape"
		a.GyroBtn = "GYRO: LANDSCAPE"
		a.Landscape = true
	default:
		a.GyroMode = "off"
		a.GyroBtn = "GYRO: OFF"
		a.Landscape = false
		a.Tilt = ""
	}
}

func (a *App) MouseMove(clientX, clientY float64) {
	// clientX is viewport-relative. On the first mousemove, calibrate
	// by assuming the mouse is over the paddle's current center.
	if !a.calibrated {
		a.areaLeftOffset = clientX - (a.paddleX + paddleW/2)
		a.calibrated = true
	}
	relX := clientX - a.areaLeftOffset
	a.paddleX = clamp(relX-paddleW/2, 0, areaW-paddleW)
	a.syncCSS()
}

func (a *App) syncCSS() {
	// When ball is ready (sitting on paddle), track paddle position
	if a.BallReady {
		a.ballX = a.paddleX + paddleW/2
		a.ballY = paddleY - ballSize/2 - 1
	}
	a.BallLeft = px(a.ballX - ballSize/2)
	a.BallTop = px(a.ballY - ballSize/2)
	a.PaddleLeft = px(a.paddleX)
}

const steerTimeout = 250 * time.Millisecond

func (a *App) tick() {
	// Process controller steering (works even when paused for positioning)
	if a.steerDir != 0 {
		if time.Since(a.steerLast) > steerTimeout {
			// No recent input — stop
			a.steerDir = 0
		} else {
			// Accelerate: starts at 2 px/frame, ramps to 12 over ~0.5s
			held := time.Since(a.steerStart).Seconds()
			speed := math.Min(7, 2+held*8)
			a.paddleX = clamp(a.paddleX+float64(a.steerDir)*speed, 0, areaW-paddleW)
			a.syncCSS()
		}
	}

	// Gyroscope: map tilt angle to paddle position
	if a.GyroMode != "off" && a.Tilt != "" {
		if tilt, err := strconv.ParseFloat(a.Tilt, 64); err == nil {
			// Landscape has a narrower comfortable tilt range
			maxTilt := 45.0
			if a.GyroMode == "landscape" {
				maxTilt = 20.0
			}
			t := clamp(tilt, -maxTilt, maxTilt)
			ratio := (t + maxTilt) / (2 * maxTilt)
			a.paddleX = ratio * (areaW - paddleW)
			a.TiltPct = fmt.Sprintf("%.0f%%", ratio*100)
			a.syncCSS()
		}
	}

	// Clear sound event from previous frame
	a.SoundEvent = ""

	if !a.Playing || a.Paused {
		return
	}

	now := time.Now()

	// Life-lost sequence: impact visible → ball hidden → ball respawns on paddle → launch
	if a.waitingSpawn {
		// Clear impact flash after 400ms
		if a.ImpactShow && now.After(a.impactAt.Add(400*time.Millisecond)) {
			a.ImpactShow = false
		}
		// Respawn ball on paddle
		if now.After(a.respawnAt) {
			a.resetBall()
			a.syncCSS()
			a.waitingSpawn = false
		}
		return
	}
	if now.Before(a.freezeUntil) {
		// Ball is on paddle in ready state, keep it tracking the paddle
		if a.BallReady {
			a.ballX = a.paddleX + paddleW/2
			a.ballY = paddleY - ballSize/2 - 1
			a.syncCSS()
		}
		return
	}
	if a.BallReady {
		// Freeze ended — launch the ball
		a.BallReady = false
		a.Message = ""
	}

	// Normalize direction and apply speed
	mag := math.Sqrt(a.ballDX*a.ballDX + a.ballDY*a.ballDY)
	if mag == 0 {
		return
	}
	dx := a.ballDX / mag * a.speed
	dy := a.ballDY / mag * a.speed

	a.ballX += dx
	a.ballY += dy

	// Wall collisions (left, right, top)
	if a.ballX-ballSize/2 <= 0 {
		a.ballX = ballSize / 2
		a.ballDX = math.Abs(a.ballDX)
		a.SoundEvent = "wall"
	}
	if a.ballX+ballSize/2 >= areaW {
		a.ballX = areaW - ballSize/2
		a.ballDX = -math.Abs(a.ballDX)
		a.SoundEvent = "wall"
	}
	if a.ballY-ballSize/2 <= 0 {
		a.ballY = ballSize / 2
		a.ballDY = math.Abs(a.ballDY)
		a.SoundEvent = "wall"
	}

	// Paddle collision
	if a.ballDY > 0 && a.ballY+ballSize/2 >= paddleY && a.ballY+ballSize/2 <= paddleY+paddleH {
		if a.ballX >= a.paddleX && a.ballX <= a.paddleX+paddleW {
			a.ballY = paddleY - ballSize/2
			// Vary angle based on where ball hits paddle
			hitPos := (a.ballX - a.paddleX) / paddleW // 0..1
			angle := (hitPos - 0.5) * 2.4              // -1.2 to 1.2 radians
			a.ballDX = math.Sin(angle)
			a.ballDY = -math.Cos(angle)
			a.SoundEvent = "paddle"
		}
	}

	// Ball fell below death line
	if a.ballY > areaH {
		// Show impact at where ball crossed
		a.ImpactLeft = px(a.ballX - 30)
		a.ImpactShow = true
		a.impactAt = time.Now()

		a.Lives--
		if a.Lives <= 0 {
			a.Playing = false
			a.BallHidden = true
			a.Message = "GAME OVER"
			a.SoundEvent = "gameover"
		} else {
			a.BallHidden = true
			a.Message = fmt.Sprintf("%d LIVES LEFT", a.Lives)
			a.waitingSpawn = true
			a.respawnAt = time.Now().Add(1200 * time.Millisecond)
			a.freezeUntil = time.Now().Add(2000 * time.Millisecond)
			a.SoundEvent = "life"
		}
		a.syncCSS()
		return
	}

	// Brick collisions
	for i := range a.Bricks {
		b := &a.Bricks[i]
		if !b.Visible {
			continue
		}
		if a.ballX+ballSize/2 >= b.x && a.ballX-ballSize/2 <= b.x+brickW &&
			a.ballY+ballSize/2 >= b.y && a.ballY-ballSize/2 <= b.y+brickH {

			b.Visible = false
			a.Score += b.Points
			a.bricksLeft--

			// Determine bounce direction
			overlapLeft := (a.ballX + ballSize/2) - b.x
			overlapRight := (b.x + brickW) - (a.ballX - ballSize/2)
			overlapTop := (a.ballY + ballSize/2) - b.y
			overlapBottom := (b.y + brickH) - (a.ballY - ballSize/2)

			minOverlapX := math.Min(overlapLeft, overlapRight)
			minOverlapY := math.Min(overlapTop, overlapBottom)

			if minOverlapX < minOverlapY {
				a.ballDX = -a.ballDX
			} else {
				a.ballDY = -a.ballDY
			}

			// Speed up slightly
			a.speed = math.Min(8.0, a.speed+0.05)

			if a.bricksLeft == 0 {
				a.Playing = false
				a.Message = "YOU WIN!"
				a.SoundEvent = "win"
			} else {
				a.SoundEvent = "brick"
			}
			break // one brick per frame
		}
	}

	a.syncCSS()
}

func (a *App) run() {
	ticker := time.NewTicker(16 * time.Millisecond) // ~60fps
	for range ticker.C {
		a.tick()
		a.Refresh()
	}
}

func px(v float64) string {
	return fmt.Sprintf("%.1fpx", v)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func main() {
	eng := godom.NewEngine()
	eng.SetUI(ui)
	eng.RegisterPlugin("gyro", gyroJS)
	eng.RegisterPlugin("sfx", sfxJS)

	root := &App{
		Lives:       3,
		Message:     "Click START to play",
		BallHidden:  true,
		GyroMode:    "off",
		GyroBtn:     "GYRO: OFF",
	}
	root.paddleX = (areaW - paddleW) / 2
	root.syncCSS()

	go root.run()

	fmt.Println("Breakout — classic brick-breaking game in Go")
	eng.Mount(root, "ui/index.html")
	log.Fatal(eng.Start())
}
