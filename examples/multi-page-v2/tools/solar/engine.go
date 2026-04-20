package solar

import "math"

type camera struct {
	Center   vec3
	Distance float64
	Tilt     float64
	Rotation float64
	FOV      float64
	Width    float64
	Height   float64
}

type DrawCmd struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Radius     float64 `json:"r"`
	Color      string  `json:"color"`
	Glow       bool    `json:"glow,omitempty"`
	Ring       bool    `json:"ring,omitempty"`
	RingColor  string  `json:"ringColor,omitempty"`
	Brightness float64 `json:"brightness"`
}

func (c *camera) project(pos vec3, radius float64) (sx, sy, sr float64, depth float64) {
	camX := c.Center.X + c.Distance*math.Sin(c.Rotation)*math.Cos(c.Tilt)
	camY := c.Center.Y + c.Distance*math.Sin(c.Tilt)
	camZ := c.Center.Z + c.Distance*math.Cos(c.Rotation)*math.Cos(c.Tilt)

	dx := pos.X - camX
	dy := pos.Y - camY
	dz := pos.Z - camZ

	fwd := vec3{c.Center.X - camX, c.Center.Y - camY, c.Center.Z - camZ}.normalize()
	up := vec3{0, 1, 0}
	right := vec3{
		fwd.Y*up.Z - fwd.Z*up.Y,
		fwd.Z*up.X - fwd.X*up.Z,
		fwd.X*up.Y - fwd.Y*up.X,
	}.normalize()
	camUp := vec3{
		right.Y*fwd.Z - right.Z*fwd.Y,
		right.Z*fwd.X - right.X*fwd.Z,
		right.X*fwd.Y - right.Y*fwd.X,
	}.normalize()

	depth = dx*fwd.X + dy*fwd.Y + dz*fwd.Z
	if depth < 0.1 {
		depth = 0.1
	}
	px := dx*right.X + dy*right.Y + dz*right.Z
	py := dx*camUp.X + dy*camUp.Y + dz*camUp.Z
	scale := c.FOV / depth
	sx = c.Width/2 + px*scale
	sy = c.Height/2 - py*scale
	sr = radius * scale
	return sx, sy, sr, depth
}

func brightness(pos vec3) float64 {
	toSun := pos.scale(-1).normalize()
	light := vec3{-0.3, 0.8, -0.5}.normalize()
	d := toSun.dot(light)
	if d < 0 {
		d = 0
	}
	return 0.3 + 0.7*d
}
