package main

import (
	"embed"
	"fmt"
	"log"
	"time"

	"godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Component
	Time       string
	HourHand   string
	MinuteHand string
	SecondHand string
}

func (a *App) updateTime() {
	now := time.Now()
	a.Time = now.Format("15:04:05")

	h, m, s := now.Hour()%12, now.Minute(), now.Second()
	hourAngle := float64(h)*30 + float64(m)*0.5
	minuteAngle := float64(m)*6 + float64(s)*0.1
	secondAngle := float64(s) * 6

	a.HourHand = fmt.Sprintf("rotate(%.1f 50 50)", hourAngle)
	a.MinuteHand = fmt.Sprintf("rotate(%.1f 50 50)", minuteAngle)
	a.SecondHand = fmt.Sprintf("rotate(%.1f 50 50)", secondAngle)
}

func (a *App) startClock() {
	ticker := time.NewTicker(50 * time.Millisecond)
	for range ticker.C {
		old := a.Time
		a.updateTime()
		if a.Time != old {
			a.Refresh()
		}
	}
}

func main() {
	app := godom.New()
	root := &App{}
	go root.startClock()
	app.Mount(root, ui)
	log.Fatal(app.Start())
}
