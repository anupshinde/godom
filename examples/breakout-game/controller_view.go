package main

import (
	"time"

	"github.com/anupshinde/godom"
)

// ControllerView is the mobile controller component. It embeds *GameState
// so score, lives, and control inputs are shared with PlayView.
type ControllerView struct {
	godom.Island
	*GameState
}

// StartGame starts or restarts the game.
func (c *ControllerView) StartGame() {
	if !c.PlayViewConnected {
		c.Message = "Opening Play page..."
		c.pendingAutoStart = true
		// Redirect browsers on menu to /play
		c.ExecJS("if(location.pathname==='/') location.href='/play'", func([]byte, string) {})
		return
	}
	c.Score = 0
	c.Lives = 3
	c.Playing = true
	c.Paused = false
	c.Message = ""
	c.StartBtn = "RESTART"
	c.PauseBtn = "PAUSE"
	c.ImpactShow = false
	c.initBricks()
	c.paddleX = (areaW - paddleW) / 2
	c.calibrated = false
	c.resetBall()
	c.syncCSS()
}

// PauseToggle toggles pause state.
func (c *ControllerView) PauseToggle() {
	if c.Playing {
		c.Paused = !c.Paused
		if c.Paused {
			c.Message = "PAUSED"
			c.PauseBtn = "CONTINUE"
		} else {
			c.Message = ""
			c.PauseBtn = "PAUSE"
		}
	}
}

// MoveLeft steers the paddle left.
func (c *ControllerView) MoveLeft()  { c.steer(-1) }

// MoveRight steers the paddle right.
func (c *ControllerView) MoveRight() { c.steer(1) }

// ControllerPing is called periodically by the controller page to signal presence.
func (c *ControllerView) ControllerPing() {
	c.lastControllerPing = time.Now()
	c.ControllerConnected = true
}

// GyroToggle cycles through gyroscope modes.
func (c *ControllerView) GyroToggle() {
	switch c.GyroMode {
	case "off":
		c.GyroMode = "portrait"
		c.GyroBtn = "GYRO: PORTRAIT"
		c.Landscape = false
	case "portrait":
		c.GyroMode = "landscape"
		c.GyroBtn = "GYRO: LANDSCAPE"
		c.Landscape = true
	default:
		c.GyroMode = "off"
		c.GyroBtn = "GYRO: OFF"
		c.Landscape = false
		c.Tilt = ""
	}
}
