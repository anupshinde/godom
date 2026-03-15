package main

import "math"

// Camera defines a perspective projection from a tilted viewpoint.
type Camera struct {
	Distance float64 // distance from origin
	Tilt     float64 // tilt angle in radians (0 = top-down, π/2 = side)
	Rotation float64 // rotation around Y axis
	FOV      float64 // field of view factor
	Width    float64 // screen width
	Height   float64 // screen height
}

// ProjectedBody is what gets sent to the JS renderer.
type DrawCmd struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Radius     float64 `json:"r"`
	Color      string  `json:"color"`
	Glow       bool    `json:"glow,omitempty"`
	Ring       bool    `json:"ring,omitempty"`
	RingColor  string  `json:"ringColor,omitempty"`
	Brightness float64 `json:"brightness"` // 0-1, for shading
}

// Project converts a 3D position to 2D screen coordinates.
func (c *Camera) Project(pos Vec3, radius float64) (sx, sy, sr float64, depth float64) {
	// Camera position
	camX := c.Distance * math.Sin(c.Rotation) * math.Cos(c.Tilt)
	camY := c.Distance * math.Sin(c.Tilt)
	camZ := c.Distance * math.Cos(c.Rotation) * math.Cos(c.Tilt)

	// Vector from camera to object
	dx := pos.X - camX
	dy := pos.Y - camY
	dz := pos.Z - camZ

	// Camera forward direction (toward origin)
	fwd := Vec3{-camX, -camY, -camZ}.Normalize()
	// Camera right direction (cross of fwd and world up)
	up := Vec3{0, 1, 0}
	right := Vec3{
		fwd.Y*up.Z - fwd.Z*up.Y,
		fwd.Z*up.X - fwd.X*up.Z,
		fwd.X*up.Y - fwd.Y*up.X,
	}.Normalize()
	// Camera up direction
	camUp := Vec3{
		right.Y*fwd.Z - right.Z*fwd.Y,
		right.Z*fwd.X - right.X*fwd.Z,
		right.X*fwd.Y - right.Y*fwd.X,
	}.Normalize()

	// Project onto camera plane
	depth = dx*fwd.X + dy*fwd.Y + dz*fwd.Z
	if depth < 0.1 {
		depth = 0.1
	}

	px := dx*right.X + dy*right.Y + dz*right.Z
	py := dx*camUp.X + dy*camUp.Y + dz*camUp.Z

	// Perspective divide
	scale := c.FOV / depth
	sx = c.Width/2 + px*scale
	sy = c.Height/2 - py*scale
	sr = radius * scale

	return sx, sy, sr, depth
}

// ComputeBrightness returns a 0-1 value based on how much a body
// faces the sun (at origin). Simple diffuse shading.
func ComputeBrightness(pos Vec3) float64 {
	toSun := pos.Scale(-1).Normalize()
	// Use a fixed light direction from camera-ish angle
	light := Vec3{-0.3, 0.8, -0.5}.Normalize()
	d := toSun.Dot(light)
	// Remap to 0.3 - 1.0 range (ambient + diffuse)
	if d < 0 {
		d = 0
	}
	return 0.3 + 0.7*d
}
