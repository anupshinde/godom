package solar

import "math"

type body struct {
	Name        string
	Radius      float64
	OrbitRadius float64
	Speed       float64
	Angle       float64
	Color       string
	HasRings    bool
	RingColor   string
	IsGlow      bool
	Moons       []*body
}

func (b *body) position(parent vec3) vec3 {
	return vec3{
		X: parent.X + b.OrbitRadius*math.Cos(b.Angle),
		Y: 0,
		Z: parent.Z + b.OrbitRadius*math.Sin(b.Angle),
	}
}

func (b *body) update(dt float64) {
	b.Angle += b.Speed * dt
	for _, m := range b.Moons {
		m.update(dt)
	}
}

func (b *body) collect(parent vec3, cam *camera, cmds *[]DrawCmd) {
	pos := b.position(parent)
	sx, sy, sr, _ := cam.project(pos, b.Radius)
	br := 1.0
	if !b.IsGlow {
		br = brightness(pos)
	}
	*cmds = append(*cmds, DrawCmd{
		X:          math.Round(sx*10) / 10,
		Y:          math.Round(sy*10) / 10,
		Radius:     math.Max(1, math.Round(sr*10)/10),
		Color:      b.Color,
		Glow:       b.IsGlow,
		Ring:       b.HasRings,
		RingColor:  b.RingColor,
		Brightness: math.Round(br*100) / 100,
	})
	for _, m := range b.Moons {
		m.collect(pos, cam, cmds)
	}
}

func newBodies() []*body {
	return []*body{
		{Name: "Sun", Radius: 25, Color: "#FDB813", IsGlow: true},
		{Name: "Mercury", Radius: 4, OrbitRadius: 60, Speed: 1.6, Color: "#A0A0A0"},
		{Name: "Venus", Radius: 6, OrbitRadius: 90, Speed: 1.2, Color: "#E8CDA0"},
		{Name: "Earth", Radius: 7, OrbitRadius: 130, Speed: 1.0, Color: "#4A90D9",
			Moons: []*body{{Name: "Moon", Radius: 2, OrbitRadius: 14, Speed: 4.0, Color: "#C0C0C0"}},
		},
		{Name: "Mars", Radius: 5, OrbitRadius: 170, Speed: 0.8, Color: "#C1440E"},
		{Name: "Jupiter", Radius: 14, OrbitRadius: 230, Speed: 0.45, Color: "#C88B3A",
			Moons: []*body{
				{Name: "Io", Radius: 2, OrbitRadius: 22, Speed: 3.5, Color: "#F0E060"},
				{Name: "Europa", Radius: 2, OrbitRadius: 28, Speed: 2.8, Color: "#B0C4DE"},
			},
		},
		{Name: "Saturn", Radius: 12, OrbitRadius: 300, Speed: 0.35, Color: "#E8D590",
			HasRings: true, RingColor: "#D4BE8D",
			Moons:    []*body{{Name: "Titan", Radius: 3, OrbitRadius: 24, Speed: 2.5, Color: "#DAA520"}},
		},
	}
}

func findBodyPos(bodies []*body, name string) (vec3, bool) {
	origin := vec3{}
	for _, b := range bodies {
		if b.Name == name {
			return b.position(origin), true
		}
		for _, m := range b.Moons {
			if m.Name == name {
				return m.position(b.position(origin)), true
			}
		}
	}
	return vec3{}, false
}
