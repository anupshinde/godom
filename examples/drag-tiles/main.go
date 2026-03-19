package main

import (
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type Tile struct {
	Label string
	Color string
	Shine bool
}

type TileGrid struct {
	godom.Component
	Tiles []Tile
}

func (g *TileGrid) Reorder(from, to float64) {
	f, d := int(from), int(to)
	if f == d || f < 0 || d < 0 || f >= len(g.Tiles) || d >= len(g.Tiles) {
		return
	}
	item := g.Tiles[f]
	g.Tiles = append(g.Tiles[:f], g.Tiles[f+1:]...)
	g.Tiles = append(g.Tiles[:d], append([]Tile{item}, g.Tiles[d:]...)...)
}

func main() {
	tiles := make([]Tile, 24)
	for i := range tiles {
		hue := (i * 15) % 360
		tiles[i] = Tile{
			Label: fmt.Sprintf("%02d", i+1),
			Color: fmt.Sprintf("hsl(%d, 70%%, 55%%)", hue),
		}
	}

	grid := &TileGrid{Tiles: tiles}

	go func() {
		time.Sleep(3 * time.Second)
		for {
			// Walk tiles in label order (01, 02, ... 24)
			for num := 1; num <= 24; num++ {
				label := fmt.Sprintf("%02d", num)
				for j := range grid.Tiles {
					if grid.Tiles[j].Label == label {
						grid.Tiles[j].Shine = true
						grid.Refresh()
						time.Sleep(80 * time.Millisecond)
						grid.Tiles[j].Shine = false
						break
					}
				}
			}
			grid.Refresh()
			time.Sleep(6 * time.Second)
		}
	}()

	eng := godom.NewEngine()
	eng.Port = 8082
	eng.Mount(grid, ui)
	log.Fatal(eng.Start())
}
