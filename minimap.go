package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	// minimapRadius is the on-screen radius of the circular minimap, in game
	// (internal-resolution) pixels.
	minimapRadius = 46
	// minimapMargin is the gap between the minimap and the bottom-right corner.
	minimapMargin = 8
	// minimapScale converts world pixels to minimap pixels. At 0.04 the minimap
	// spans minimapRadius/0.04 ≈ 1150 world pixels from the ship in every
	// direction.
	minimapScale = 0.04
)

// DrawMinimap renders a small circular overview in the bottom-right of the game
// view, centered on the ship. The ship sits at the center pointing along its
// heading; each asteroid is drawn at its position relative to the ship. Objects
// beyond the minimap's range are clipped out.
//
// Call this in screen (texture) space — outside BeginMode2D — so it stays fixed
// as an overlay rather than moving with the camera.
func DrawMinimap(ship *Ship, asteroids []*Asteroid) {
	center := rl.NewVector2(
		gameWidth-minimapRadius-minimapMargin,
		gameHeight-minimapRadius-minimapMargin,
	)

	// Panel: translucent dark disc with a light border.
	rl.DrawCircleV(center, minimapRadius, rl.NewColor(0, 0, 0, 160))
	rl.DrawCircleLines(int32(center.X), int32(center.Y), minimapRadius, rl.NewColor(255, 255, 255, 120))

	// Asteroids relative to the ship.
	for _, a := range asteroids {
		blip := rl.NewVector2(
			center.X+(a.Position.X-ship.Position.X)*minimapScale,
			center.Y+(a.Position.Y-ship.Position.Y)*minimapScale,
		)
		// Blip radius scales with the asteroid, with a floor so small rocks stay
		// visible.
		r := a.Size * minimapScale
		if r < 1.5 {
			r = 1.5
		}
		// Clip to the disc: skip anything whose blip falls outside the minimap.
		if dist(blip, center)+r > minimapRadius {
			continue
		}
		rl.DrawCircleV(blip, r, rl.Gray)
	}

	// Player ship: a small triangle at the center pointing along its heading.
	drawMinimapShip(center, ship.Direction)
}

// drawMinimapShip draws the ship marker: a triangle pointing in the ship's
// facing direction (dir 0 points "up", i.e. -Y).
func drawMinimapShip(center rl.Vector2, dir float32) {
	sin := float32(math.Sin(float64(dir)))
	cos := float32(math.Cos(float64(dir)))

	// Canonical up-pointing triangle relative to the center.
	pts := [3]rl.Vector2{
		{X: 0, Y: -5}, // nose
		{X: -3.5, Y: 4},
		{X: 3.5, Y: 4},
	}
	var w [3]rl.Vector2
	for i, p := range pts {
		w[i] = rl.NewVector2(
			center.X+p.X*cos-p.Y*sin,
			center.Y+p.X*sin+p.Y*cos,
		)
	}
	rl.DrawTriangle(w[0], w[1], w[2], rl.SkyBlue)
}

// dist returns the Euclidean distance between two points.
func dist(a, b rl.Vector2) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}
