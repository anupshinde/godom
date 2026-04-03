package main

import (
	"time"

	"github.com/anupshinde/godom"
)

// StatusView shows game status in the nav bar. Shares *GameState
// with PlayView and ControllerView so it sees the same state.
type StatusView struct {
	godom.Component
	*GameState
}

// RunStatusRefresh periodically refreshes the status view so heartbeat
// timeouts are reflected in the UI without depending on shared pointer propagation.
func (s *StatusView) RunStatusRefresh() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		s.Refresh()
	}
}
