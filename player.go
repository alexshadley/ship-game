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
	// playerMaxHealth is the astronaut's hit points on a spacewalk. Space is
	// unforgiving: a fresh 15 each walk, and reaching zero is game over.
	playerMaxHealth = 15.0
)

type Player struct {
	Position rl.Vector2
	Velocity rl.Vector2
	Health   float32
}

// EjectFrom seeds the astronaut just behind the ship's cockpit (local +Y is the
// rear) moving with the ship, so stepping out doesn't leave them stranded. Each
// walk starts at full health.
func (p *Player) EjectFrom(ship *Ship) {
	p.Position = ship.worldPoint(0, spawnBackDistance)
	p.Velocity = ship.Velocity
	p.Health = playerMaxHealth
}

// Dead reports whether the astronaut's health has run out (game over).
func (p *Player) Dead() bool {
	return p.Health <= 0
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

	// Float a compact health bar above the astronaut once damaged, mirroring the
	// ship parts' bars (green→red as it drains).
	if p.Health < playerMaxHealth {
		frac := p.Health / playerMaxHealth
		if frac < 0 {
			frac = 0
		}
		const barWidth int32 = 2*playerRadius + 2
		const barHeight int32 = 3
		x := int32(p.Position.X) - barWidth/2
		y := int32(p.Position.Y-playerRadius) - barHeight - 3
		rl.DrawRectangle(x, y, barWidth, barHeight, rl.NewColor(0, 0, 0, 160))
		rl.DrawRectangle(x, y, int32(float32(barWidth)*frac), barHeight, healthColor(frac))
	}
}
