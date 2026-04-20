package solar

import "math"

type vec3 struct{ X, Y, Z float64 }

func (v vec3) scale(s float64) vec3 { return vec3{v.X * s, v.Y * s, v.Z * s} }
func (v vec3) length() float64      { return math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z) }
func (v vec3) normalize() vec3 {
	l := v.length()
	if l == 0 {
		return vec3{}
	}
	return vec3{v.X / l, v.Y / l, v.Z / l}
}
func (v vec3) dot(o vec3) float64 { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }
