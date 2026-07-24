package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	minimapRadius = 46
	minimapMargin = 8
	minimapScale  = 0.02
)

// DrawMinimap must be called in screen (texture) space — outside BeginMode2D —
// so it stays fixed as an overlay rather than moving with the camera. player is
// non-nil only during a spacewalk.
func DrawMinimap(ship *Ship, asteroids []*Asteroid, enemies []*Ship, player *Player) {
	center := rl.NewVector2(
		gameWidth-minimapRadius-minimapMargin,
		gameHeight-minimapRadius-minimapMargin,
	)

	rl.DrawCircleV(center, minimapRadius, rl.NewColor(0, 0, 0, 160))
	rl.DrawCircleLines(int32(center.X), int32(center.Y), minimapRadius, rl.NewColor(255, 255, 255, 120))

	for _, a := range asteroids {
		blip := rl.NewVector2(
			center.X+(a.Position.X-ship.Position.X)*minimapScale,
			center.Y+(a.Position.Y-ship.Position.Y)*minimapScale,
		)
		r := a.Size * minimapScale
		if r < 1.5 {
			r = 1.5
		}
		if dist(blip, center)+r > minimapRadius {
			continue
		}
		rl.DrawCircleV(blip, r, rl.Gray)
	}

	for _, e := range enemies {
		blip := rl.NewVector2(
			center.X+(e.Position.X-ship.Position.X)*minimapScale,
			center.Y+(e.Position.Y-ship.Position.Y)*minimapScale,
		)
		if dist(blip, center)+5 > minimapRadius {
			continue
		}
		drawMinimapMarker(blip, e.Direction, rl.Red)
	}

	drawMinimapMarker(center, ship.Direction, rl.SkyBlue)

	if player != nil {
		blip := rl.NewVector2(
			center.X+(player.Position.X-ship.Position.X)*minimapScale,
			center.Y+(player.Position.Y-ship.Position.Y)*minimapScale,
		)
		if dist(blip, center) <= minimapRadius {
			rl.DrawCircleV(blip, 2, rl.Orange)
		}
	}
}

func drawMinimapMarker(pos rl.Vector2, dir float32, color rl.Color) {
	sin := float32(math.Sin(float64(dir)))
	cos := float32(math.Cos(float64(dir)))

	pts := [3]rl.Vector2{
		{X: 0, Y: -5},
		{X: -3.5, Y: 4},
		{X: 3.5, Y: 4},
	}
	var w [3]rl.Vector2
	for i, p := range pts {
		w[i] = rl.NewVector2(
			pos.X+p.X*cos-p.Y*sin,
			pos.Y+p.X*sin+p.Y*cos,
		)
	}
	rl.DrawTriangle(w[0], w[1], w[2], color)
}

func dist(a, b rl.Vector2) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}
