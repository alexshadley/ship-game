package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	minimapRadius = 46
	minimapMargin = 8
	minimapScale  = 0.01

	missileBlipRadius = 1.4
)

var (
	// Missiles read as bright blips distinct from the ship/enemy markers:
	// friendly (fired from the player's ship) in cyan, hostile in orange.
	minimapMissileFriendlyColor = rl.NewColor(90, 220, 255, 255)
	minimapMissileHostileColor  = rl.NewColor(255, 150, 40, 255)
)

// DrawMinimap must be called in screen (texture) space — outside BeginMode2D —
// so it stays fixed as an overlay rather than moving with the camera. player is
// non-nil only during a spacewalk.
func DrawMinimap(ship *Ship, asteroids []*Asteroid, enemies []*Ship, projectiles []*Projectile, looseParts []*LoosePart, player *Player) {
	center := rl.NewVector2(
		gameWidth-minimapRadius-minimapMargin,
		gameHeight-minimapRadius-minimapMargin,
	)

	rl.DrawCircleV(center, minimapRadius, rl.NewColor(0, 0, 0, 160))
	rl.DrawCircleLines(int32(center.X), int32(center.Y), minimapRadius, rl.NewColor(255, 255, 255, 120))

	drawMinimapBounds(center, ship.Position)

	for _, l := range looseParts {
		blip := rl.NewVector2(
			center.X+(l.Position.X-ship.Position.X)*minimapScale,
			center.Y+(l.Position.Y-ship.Position.Y)*minimapScale,
		)
		if dist(blip, center)+1.5 > minimapRadius {
			continue
		}
		rl.DrawCircleV(blip, 1.5, rl.Yellow)
	}

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

	// Missiles in flight — friendly and hostile alike — read as small bright
	// blips so the player can track incoming ordnance and their own salvos.
	for _, pr := range projectiles {
		if pr.Kind != projectileMissile {
			continue
		}
		blip := rl.NewVector2(
			center.X+(pr.Position.X-ship.Position.X)*minimapScale,
			center.Y+(pr.Position.Y-ship.Position.Y)*minimapScale,
		)
		if dist(blip, center)+missileBlipRadius > minimapRadius {
			continue
		}
		color := minimapMissileHostileColor
		if pr.Owner == ship {
			color = minimapMissileFriendlyColor
		}
		rl.DrawCircleV(blip, missileBlipRadius, color)
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

// drawMinimapBounds dots the square world boundary (see worldBound) onto the
// minimap, clipped to its circular frame, so the edge of the play area shows up
// as the ship nears it. Points are sampled along the boundary's perimeter and each
// is dropped if it falls outside the radar disc — at minimapScale the full field
// is larger than the radar, so only the nearest wall(s) are ever visible.
func drawMinimapBounds(center, shipPos rl.Vector2) {
	const step = 100 // world units between sampled boundary dots
	plot := func(x, y float32) {
		blip := rl.NewVector2(
			center.X+(x-shipPos.X)*minimapScale,
			center.Y+(y-shipPos.Y)*minimapScale,
		)
		if dist(blip, center) > minimapRadius {
			return
		}
		rl.DrawCircleV(blip, 1, worldBoundColor)
	}
	for d := float32(-worldBound); d <= worldBound; d += step {
		plot(d, -worldBound) // top edge
		plot(d, worldBound)  // bottom edge
		plot(-worldBound, d) // left edge
		plot(worldBound, d)  // right edge
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
