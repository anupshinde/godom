package main

import "github.com/anupshinde/godom"

// Navbar is a fixed top navigation bar.
type Navbar struct {
	godom.Island
	ComponentCount int
	Status         string
}
