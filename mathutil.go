package main

import "math"

// heading returns the ship heading that points the nose along world direction
// (vx, vy). The nose vector at heading h is (sin h, -cos h), hence the -vy.
func heading(vx, vy float32) float32 {
	return float32(math.Atan2(float64(vx), float64(-vy)))
}

func angleDiff(a, b float32) float32 {
	d := float64(a - b)
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	return float32(d)
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
