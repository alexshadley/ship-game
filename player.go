package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	playerThrust = 300.0
	playerMass   = 2.0
	// playerDamping is the fraction of velocity that survives each second — much
	// lighter than the ship's drag, so the astronaut coasts on a spacewalk.
	playerDamping     = 0.08
	playerElasticity  = 0.5
	playerRadius      = 7.0
	reentryDistance   = cellSize * 1.3
	spawnBackDistance = cellSize * 1.7
)

type Player struct {
	Position rl.Vector2
	Velocity rl.Vector2
}

// EjectFrom seeds the astronaut just behind the ship's cockpit (local +Y is the
// rear) moving with the ship, so stepping out doesn't leave them stranded.
func (p *Player) EjectFrom(ship *Ship) {
	p.Position = ship.worldPoint(0, spawnBackDistance)
	p.Velocity = ship.Velocity
}

func walkInputDir() (float64, float64) {
	var dx, dy float64
	if rl.IsKeyDown(rl.KeyW) {
		dy -= 1
	}
	if rl.IsKeyDown(rl.KeyS) {
		dy += 1
	}
	if rl.IsKeyDown(rl.KeyA) {
		dx -= 1
	}
	if rl.IsKeyDown(rl.KeyD) {
		dx += 1
	}
	if dx != 0 || dy != 0 {
		l := math.Hypot(dx, dy)
		dx, dy = dx/l, dy/l
	}
	return dx, dy
}

func (p *Player) NearCockpit(ship *Ship) bool {
	return dist(p.Position, ship.Position) <= reentryDistance
}

func (p *Player) Draw() {
	rl.DrawCircleV(p.Position, playerRadius, rl.Orange)
	rl.DrawCircleV(p.Position, playerRadius-3, rl.SkyBlue)
	rl.DrawCircleLines(int32(p.Position.X), int32(p.Position.Y), playerRadius, rl.Black)
}
