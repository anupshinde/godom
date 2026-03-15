package main

import "math"

// Body represents a celestial body with orbital parameters.
type Body struct {
	Name        string
	Radius      float64 // display radius (not to scale)
	OrbitRadius float64 // distance from parent
	Speed       float64 // radians per second
	Angle       float64 // current angle
	Color       string
	HasRings    bool
	RingColor   string
	IsGlow      bool // for the sun
	Moons       []*Body
}

// Position computes the 3D position of this body relative to a parent position.
func (b *Body) Position(parent Vec3) Vec3 {
	return Vec3{
		X: parent.X + b.OrbitRadius*math.Cos(b.Angle),
		Y: 0,
		Z: parent.Z + b.OrbitRadius*math.Sin(b.Angle),
	}
}

// Update advances the orbital angle.
func (b *Body) Update(dt float64) {
	b.Angle += b.Speed * dt
	for _, m := range b.Moons {
		m.Update(dt)
	}
}

// Collect gathers draw commands for this body and its moons.
func (b *Body) Collect(parent Vec3, cam *Camera, cmds *[]DrawCmd) {
	pos := b.Position(parent)

	sx, sy, sr, _ := cam.Project(pos, b.Radius)

	brightness := 1.0
	if !b.IsGlow {
		brightness = ComputeBrightness(pos)
	}

	*cmds = append(*cmds, DrawCmd{
		X:          math.Round(sx*10) / 10,
		Y:          math.Round(sy*10) / 10,
		Radius:     math.Max(1, math.Round(sr*10)/10),
		Color:      b.Color,
		Glow:       b.IsGlow,
		Ring:       b.HasRings,
		RingColor:  b.RingColor,
		Brightness: math.Round(brightness*100) / 100,
	})

	for _, m := range b.Moons {
		m.Collect(pos, cam, cmds)
	}
}

// NewSolarSystem creates the sun and planets with approximate relative proportions.
// Sizes and distances are not to scale — adjusted for visual clarity.
func NewSolarSystem() []*Body {
	return []*Body{
		{
			Name: "Sun", Radius: 25, Color: "#FDB813", IsGlow: true,
		},
		{
			Name: "Mercury", Radius: 4, OrbitRadius: 60, Speed: 1.6, Color: "#A0A0A0",
		},
		{
			Name: "Venus", Radius: 6, OrbitRadius: 90, Speed: 1.2, Color: "#E8CDA0",
		},
		{
			Name: "Earth", Radius: 7, OrbitRadius: 130, Speed: 1.0, Color: "#4A90D9",
			Moons: []*Body{
				{Name: "Moon", Radius: 2, OrbitRadius: 14, Speed: 4.0, Color: "#C0C0C0"},
			},
		},
		{
			Name: "Mars", Radius: 5, OrbitRadius: 170, Speed: 0.8, Color: "#C1440E",
		},
		{
			Name: "Jupiter", Radius: 14, OrbitRadius: 230, Speed: 0.45, Color: "#C88B3A",
			Moons: []*Body{
				{Name: "Io", Radius: 2, OrbitRadius: 22, Speed: 3.5, Color: "#F0E060"},
				{Name: "Europa", Radius: 2, OrbitRadius: 28, Speed: 2.8, Color: "#B0C4DE"},
			},
		},
		{
			Name: "Saturn", Radius: 12, OrbitRadius: 300, Speed: 0.35, Color: "#E8D590",
			HasRings: true, RingColor: "#D4BE8D",
			Moons: []*Body{
				{Name: "Titan", Radius: 3, OrbitRadius: 24, Speed: 2.5, Color: "#DAA520"},
			},
		},
	}
}
