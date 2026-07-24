package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Spacewalk (extravehicular) tuning constants.
const (
	// playerThrust is the astronaut's acceleration while a WASD key is held, in
	// pixels/sec². Physics converts it to a force via the astronaut's mass.
	playerThrust = 300.0
	// playerMass is the astronaut's mass in the same pixel-mass units as part
	// weights, so collisions with asteroids and the ship carry realistic momentum.
	playerMass = 2.0
	// playerDamping is the fraction of the astronaut's velocity that survives each
	// second, so they coast to a gentle stop when not thrusting.
	playerDamping = 0.08
	// playerElasticity is how bouncy the astronaut is in a collision (0 = none,
	// 1 = fully elastic); Chipmunk multiplies it with the other shape's value.
	playerElasticity = 0.5
	// playerRadius is the astronaut's on-screen and collision radius in pixels.
	playerRadius = 7.0
	// reentryDistance is how close, in world pixels, the astronaut must be to the
	// cockpit to climb back in.
	reentryDistance = cellSize * 1.3
	// spawnBackDistance is how far behind the cockpit (local +Y, the rear) the
	// astronaut appears when it pops out, just clear of the re-entry range.
	spawnBackDistance = cellSize * 1.7
)

// Player is the astronaut during a spacewalk: a free-floating body the player
// thrusts around with WASD while outside the ship. Its motion is simulated by a
// Chipmunk body owned by Physics (created on eject); this struct holds the state
// synced back each frame for rendering and re-entry checks.
type Player struct {
	Position rl.Vector2 // world position of the astronaut's center
	Velocity rl.Vector2 // world-space velocity in pixels/sec
}

// EjectFrom seeds the astronaut just behind the ship's cockpit (out the open rear
// slot) moving with the ship, so stepping out doesn't leave them stranded.
func (p *Player) EjectFrom(ship *Ship) {
	// Local +Y is the ship's rear; the cockpit is at grid {0,0}.
	p.Position = ship.worldPoint(0, spawnBackDistance)
	p.Velocity = ship.Velocity
}

// walkInputDir returns the WASD thrust direction as a unit vector in world
// (screen) space (W/S up/down, A/D left/right), or (0,0) when no key is held.
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
	// Normalize diagonals so moving on two axes isn't faster than one.
	if dx != 0 || dy != 0 {
		l := math.Hypot(dx, dy)
		dx, dy = dx/l, dy/l
	}
	return dx, dy
}

// NearCockpit reports whether the astronaut is close enough to the ship's
// cockpit (grid {0,0}, i.e. ship.Position) to re-enter.
func (p *Player) NearCockpit(ship *Ship) bool {
	return dist(p.Position, ship.Position) <= reentryDistance
}

// Draw renders the astronaut as a small suited figure: an orange body with a
// sky-blue helmet visor and a dark outline.
func (p *Player) Draw() {
	rl.DrawCircleV(p.Position, playerRadius, rl.Orange)
	rl.DrawCircleV(p.Position, playerRadius-3, rl.SkyBlue)
	rl.DrawCircleLines(int32(p.Position.X), int32(p.Position.Y), playerRadius, rl.Black)
}
