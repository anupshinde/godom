package main

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"
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
	x, y    float64
}

// GameState holds all shared game state. Both PlayView and ControllerView
// embed a pointer to the same GameState instance, so changes are visible
// to both and godom's shared pointer refresh keeps them in sync.
type GameState struct {
	// Rendered by both views
	Score    int
	Lives    int
	Message  string
	Playing  bool
	Paused   bool
	StartBtn string
	PauseBtn string

	// Ball (rendered by play view, but state is shared for physics)
	BallLeft   string
	BallTop    string
	BallHidden bool
	BallReady  bool

	// Paddle
	PaddleLeft string

	// Impact flash
	ImpactShow bool
	ImpactLeft string

	// Bricks
	Bricks []Brick

	// Gyroscope (rendered by controller, but state is shared)
	Tilt      string
	TiltPct   string
	GyroMode  string
	GyroBtn   string
	Landscape bool

	// Sound/vibration
	SoundEvent string

	// Internal physics state (unexported — not sent to browser)
	ballX, ballY       float64
	ballDX, ballDY     float64
	paddleX            float64
	speed              float64
	bricksLeft         int
	calibrated         bool
	areaLeftOffset     float64
	steerDir           int
	steerStart         time.Time
	steerLast          time.Time
	freezeUntil        time.Time
	respawnAt          time.Time
	impactAt           time.Time
	waitingSpawn       bool

	// Page coordination
	ControllerConnected bool // true when controller heartbeat is recent
	lastControllerPing  time.Time
	PlayViewConnected   bool // true when play view heartbeat is recent
	lastPlayPing        time.Time
	onGameOver          func(score int)
	playViewCount       int
	pendingAutoStart    bool // set by controller when redirecting to /play
}

func NewGameState() *GameState {
	gs := &GameState{
		Lives:      3,
		Message:    "Click START to play",
		BallHidden: true,
		StartBtn:   "START",
		PauseBtn:   "PAUSE",
		GyroMode:   "off",
		GyroBtn:    "GYRO: OFF",
	}
	gs.paddleX = (areaW - paddleW) / 2
	gs.syncCSS()
	return gs
}

func (a *GameState) resetBall() {
	a.ballX = a.paddleX + paddleW/2
	a.ballY = paddleY - ballSize/2 - 1
	a.ballDX = 2 + (rand.Float64()-0.5)*3
	a.ballDY = -3
	a.speed = initialSpeed
	a.BallReady = true
	a.BallHidden = false
}

func (a *GameState) initBricks() {
	a.Bricks = make([]Brick, 0, brickRows*brickCols)
	offsetX := (areaW - float64(brickCols)*(brickW+brickGapX) + brickGapX) / 2
	for row := range brickRows {
		for col := range brickCols {
			x := offsetX + float64(col)*(brickW+brickGapX)
			y := brickTop + float64(row)*(brickH+brickGapY)
			a.Bricks = append(a.Bricks, Brick{
				Left: px(x), Top: px(y), Width: px(brickW), Height: px(brickH),
				Color: rowColors[row], Visible: true, Points: rowPoints[row],
				x: x, y: y,
			})
		}
	}
	a.bricksLeft = brickRows * brickCols
}

func (a *GameState) syncCSS() {
	if a.BallReady {
		a.ballX = a.paddleX + paddleW/2
		a.ballY = paddleY - ballSize/2 - 1
	}
	a.BallLeft = px(a.ballX - ballSize/2)
	a.BallTop = px(a.ballY - ballSize/2)
	a.PaddleLeft = px(a.paddleX)
}

func (a *GameState) steer(dir int) {
	now := time.Now()
	if a.steerDir != dir {
		a.steerDir = dir
		a.steerStart = now
	}
	a.steerLast = now
}

const steerTimeout = 250 * time.Millisecond
const heartbeatTimeout = 5 * time.Second

func (a *GameState) tick() {
	// Check heartbeat timeouts
	if a.ControllerConnected && time.Since(a.lastControllerPing) > heartbeatTimeout {
		a.ControllerConnected = false
	}
	if a.PlayViewConnected && time.Since(a.lastPlayPing) > heartbeatTimeout {
		a.PlayViewConnected = false
		a.playViewCount = 0
		if a.Playing && !a.Paused {
			a.Paused = true
			a.Message = "PAUSED — play view disconnected"
			a.PauseBtn = "CONTINUE"
		}
	}

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

	mag := math.Sqrt(a.ballDX*a.ballDX + a.ballDY*a.ballDY)
	if mag == 0 {
		return
	}
	dx := a.ballDX / mag * a.speed
	dy := a.ballDY / mag * a.speed

	a.ballX += dx
	a.ballY += dy

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
