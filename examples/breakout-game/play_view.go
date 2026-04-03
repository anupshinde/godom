package main

import (
	"time"

	"github.com/anupshinde/godom"
)

// PlayView is the game view component. It embeds *GameState so all shared
// state (score, lives, bricks, ball, paddle) is visible and synced.
type PlayView struct {
	godom.Component
	*GameState
}

// MouseMove handles mouse input on the game area.
func (p *PlayView) MouseMove(clientX, clientY float64) {
	if !p.calibrated {
		p.areaLeftOffset = clientX - (p.paddleX + paddleW/2)
		p.calibrated = true
	}
	relX := clientX - p.areaLeftOffset
	p.paddleX = clamp(relX-paddleW/2, 0, areaW-paddleW)
	p.syncCSS()
}

// StartGame starts or restarts the game.
func (p *PlayView) StartGame() {
	if !p.PlayViewConnected {
		p.Message = "Open the Play page first"
		p.ExecJS("if(location.pathname==='/') location.href='/play'", func([]byte, string) {})
		return
	}
	p.Score = 0
	p.Lives = 3
	p.Playing = true
	p.Paused = false
	p.Message = ""
	p.StartBtn = "RESTART"
	p.PauseBtn = "PAUSE"
	p.ImpactShow = false
	p.initBricks()
	p.paddleX = (areaW - paddleW) / 2
	p.calibrated = false
	p.resetBall()
	p.syncCSS()
}

// PauseToggle toggles pause state.
func (p *PlayView) PauseToggle() {
	if p.Playing {
		p.Paused = !p.Paused
		if p.Paused {
			p.Message = "PAUSED"
			p.PauseBtn = "CONTINUE"
		} else {
			p.Message = ""
			p.PauseBtn = "PAUSE"
		}
	}
}

// MoveLeft steers the paddle left (keyboard/controller).
func (p *PlayView) MoveLeft()  { p.steer(-1) }

// MoveRight steers the paddle right (keyboard/controller).
func (p *PlayView) MoveRight() { p.steer(1) }

// PlayPing is called periodically by the play page to signal presence.
func (p *PlayView) PlayPing() {
	wasConnected := p.PlayViewConnected
	p.lastPlayPing = time.Now()
	p.PlayViewConnected = true
	p.playViewCount = 1
	// Auto-start if the controller requested it (redirected from menu)
	if !wasConnected && p.pendingAutoStart {
		p.pendingAutoStart = false
		p.StartGame()
	}
}

// Run is the game loop — ~60fps tick + refresh.
func (p *PlayView) Run() {
	ticker := time.NewTicker(16 * time.Millisecond)
	for range ticker.C {
		p.tick()
		p.Refresh()
	}
}
