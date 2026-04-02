package main

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/anupshinde/godom"
)

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

type Game struct {
	godom.Component

	// Ball (CSS strings for rendering)
	BallLeft   string
	BallTop    string
	BallHidden bool // true during death freeze
	BallReady  bool // sitting on paddle, dimmed, not yet launched

	// Paddle
	PaddleLeft string

	// Impact flash at death line
	ImpactShow bool
	ImpactLeft string

	// Game state
	Bricks   []Brick
	Score    int
	Lives    int
	Message  string
	Playing  bool
	Paused   bool
	StartBtn string // "START" or "RESTART"

	// internal state (unexported = not sent to browser)
	ballX, ballY float64
	ballDX       float64
	ballDY       float64
	paddleX      float64
	speed        float64
	bricksLeft   int
	calibrated   bool
	areaLeftOffset float64

	// Controller steering
	steerDir   int
	steerStart time.Time
	steerLast  time.Time

	// Gyroscope
	Tilt      string
	TiltPct   string
	GyroMode  string
	GyroBtn   string
	Landscape bool

	// Controller presence
	ControllerActive string

	// Sound/vibration events
	SoundEvent string

	// Life-lost sequence timing
	freezeUntil  time.Time
	respawnAt    time.Time
	impactAt     time.Time
	waitingSpawn bool

	// Score tracking callback — called when game ends
	onGameOver func(score int)
}

func NewGame() *Game {
	g := &Game{
		Lives:      3,
		Message:    "Click START to play",
		BallHidden: true,
		StartBtn:   "START",
		GyroMode:   "off",
		GyroBtn:    "GYRO: OFF",
	}
	g.paddleX = (areaW - paddleW) / 2
	g.syncCSS()
	return g
}

func (a *Game) resetBall() {
	a.ballX = a.paddleX + paddleW/2
	a.ballY = paddleY - ballSize/2 - 1
	a.ballDX = 2 + (rand.Float64()-0.5)*3
	a.ballDY = -3
	a.speed = initialSpeed
	a.BallReady = true
	a.BallHidden = false
}

func (a *Game) initBricks() {
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

func (a *Game) StartGame() {
	a.Score = 0
	a.Lives = 3
	a.Playing = true
	a.Paused = false
	a.Message = ""
	a.StartBtn = "RESTART"
	a.ImpactShow = false
	a.initBricks()
	a.paddleX = (areaW - paddleW) / 2
	a.calibrated = false
	a.resetBall()
	a.syncCSS()
}

func (a *Game) PauseToggle() {
	if a.Playing {
		a.Paused = !a.Paused
		if a.Paused {
			a.Message = "PAUSED"
		} else {
			a.Message = ""
		}
	}
}

func (a *Game) MoveLeft() {
	a.steer(-1)
}

func (a *Game) MoveRight() {
	a.steer(1)
}

func (a *Game) steer(dir int) {
	now := time.Now()
	if a.steerDir != dir {
		a.steerDir = dir
		a.steerStart = now
	}
	a.steerLast = now
}

func (a *Game) GyroToggle() {
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

func (a *Game) MouseMove(clientX, clientY float64) {
	if !a.calibrated {
		a.areaLeftOffset = clientX - (a.paddleX + paddleW/2)
		a.calibrated = true
	}
	relX := clientX - a.areaLeftOffset
	a.paddleX = clamp(relX-paddleW/2, 0, areaW-paddleW)
	a.syncCSS()
}

func (a *Game) syncCSS() {
	if a.BallReady {
		a.ballX = a.paddleX + paddleW/2
		a.ballY = paddleY - ballSize/2 - 1
	}
	a.BallLeft = px(a.ballX - ballSize/2)
	a.BallTop = px(a.ballY - ballSize/2)
	a.PaddleLeft = px(a.paddleX)
}

const steerTimeout = 250 * time.Millisecond

func (a *Game) tick() {
	// Process controller steering
	if a.steerDir != 0 {
		if time.Since(a.steerLast) > steerTimeout {
			a.steerDir = 0
		} else {
			held := time.Since(a.steerStart).Seconds()
			speed := math.Min(7, 2+held*8)
			a.paddleX = clamp(a.paddleX+float64(a.steerDir)*speed, 0, areaW-paddleW)
			a.syncCSS()
		}
	}

	// Gyroscope
	if a.GyroMode != "off" && a.Tilt != "" {
		if tilt, err := strconv.ParseFloat(a.Tilt, 64); err == nil {
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

	a.SoundEvent = ""

	if !a.Playing || a.Paused {
		return
	}

	now := time.Now()

	// Life-lost sequence
	if a.waitingSpawn {
		if a.ImpactShow && now.After(a.impactAt.Add(400*time.Millisecond)) {
			a.ImpactShow = false
		}
		if now.After(a.respawnAt) {
			a.resetBall()
			a.syncCSS()
			a.waitingSpawn = false
		}
		return
	}
	if now.Before(a.freezeUntil) {
		if a.BallReady {
			a.ballX = a.paddleX + paddleW/2
			a.ballY = paddleY - ballSize/2 - 1
			a.syncCSS()
		}
		return
	}
	if a.BallReady {
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

	// Wall collisions
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
			hitPos := (a.ballX - a.paddleX) / paddleW
			angle := (hitPos - 0.5) * 2.4
			a.ballDX = math.Sin(angle)
			a.ballDY = -math.Cos(angle)
			a.SoundEvent = "paddle"
		}
	}

	// Ball fell below death line
	if a.ballY > areaH {
		a.ImpactLeft = px(a.ballX - 30)
		a.ImpactShow = true
		a.impactAt = time.Now()

		a.Lives--
		if a.Lives <= 0 {
			a.Playing = false
			a.BallHidden = true
			a.Message = "GAME OVER"
			a.SoundEvent = "gameover"
			if a.onGameOver != nil {
				a.onGameOver(a.Score)
			}
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

			a.speed = math.Min(8.0, a.speed+0.05)

			if a.bricksLeft == 0 {
				a.Playing = false
				a.Message = "YOU WIN!"
				a.SoundEvent = "win"
				if a.onGameOver != nil {
					a.onGameOver(a.Score)
				}
			} else {
				a.SoundEvent = "brick"
			}
			break
		}
	}

	a.syncCSS()
}

func (a *Game) Run() {
	ticker := time.NewTicker(16 * time.Millisecond)
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
