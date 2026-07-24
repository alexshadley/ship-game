package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Projectile-firing tuning constants.
const (
	// cannonMuzzleSpeed is the projectile's speed relative to the firing ship, in
	// pixels/sec, along the cannon's facing direction.
	cannonMuzzleSpeed = 600.0
	// cannonFireInterval is the minimum time, in seconds, between shots while the
	// fire key is held.
	cannonFireInterval = 0.18
	// projectileLifespan is how long, in seconds, a projectile lives before it
	// expires and is removed.
	projectileLifespan = 1.5
)

// projectileSize is the box dimensions of a projectile in pixels: X across its
// travel direction, Y along it (so it reads as a bolt pointing where it flies).
var projectileSize = rl.NewVector2(4, 12)

// Projectile is a free-flying box fired from a cannon. Like an asteroid it is a
// lightweight kinematic struct, not a Chipmunk body.
type Projectile struct {
	Position rl.Vector2 // world position of the projectile's center
	Velocity rl.Vector2 // world-space velocity in pixels/sec
	Lifespan float32    // remaining time to live, in seconds
	Rotation float32    // heading in radians; 0 points "up" (-Y on screen)
	Size     rl.Vector2 // box dimensions in pixels (X across travel, Y along it)
}

// NewProjectile returns a projectile at pos travelling with the given velocity
// and oriented at rotation radians, living for projectileLifespan seconds.
func NewProjectile(pos, velocity rl.Vector2, rotation float32) *Projectile {
	return &Projectile{
		Position: pos,
		Velocity: velocity,
		Lifespan: projectileLifespan,
		Rotation: rotation,
		Size:     projectileSize,
	}
}

// Update advances the projectile by its velocity over dt seconds and ages it.
func (p *Projectile) Update(dt float32) {
	p.Position.X += p.Velocity.X * dt
	p.Position.Y += p.Velocity.Y * dt
	p.Lifespan -= dt
}

// Expired reports whether the projectile has outlived its lifespan.
func (p *Projectile) Expired() bool {
	return p.Lifespan <= 0
}

// Draw renders the projectile as a rotated box centered on its position.
func (p *Projectile) Draw() {
	rec := rl.NewRectangle(p.Position.X, p.Position.Y, p.Size.X, p.Size.Y)
	origin := rl.NewVector2(p.Size.X/2, p.Size.Y/2)
	rl.DrawRectanglePro(rec, origin, p.Rotation*180/math.Pi, rl.Yellow)
}

// FireCannons spawns one projectile from every cannon part on the ship, aimed
// along each cannon's facing (in world space) and inheriting the ship's velocity.
func (s *Ship) FireCannons() []*Projectile {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))

	var shots []*Projectile
	for c, part := range s.Parts {
		if part.Type != PartCannon {
			continue
		}

		// Local firing direction is the facing's unit grid step (up = -Y = nose).
		off := part.Facing.offset()
		ldx, ldy := float32(off.X), float32(off.Y)
		// Rotate that direction into world space by the ship's heading.
		wdx := ldx*cos - ldy*sin
		wdy := ldx*sin + ldy*cos

		// Muzzle sits half a cell out from the cannon's center along its barrel.
		lx := float32(c.X)*cellSize + ldx*cellSize*0.5
		ly := float32(c.Y)*cellSize + ldy*cellSize*0.5
		pos := s.worldPoint(lx, ly)

		vel := rl.NewVector2(
			s.Velocity.X+wdx*cannonMuzzleSpeed,
			s.Velocity.Y+wdy*cannonMuzzleSpeed,
		)
		rotation := s.Direction + part.Facing.angle()
		shots = append(shots, NewProjectile(pos, vel, rotation))
	}
	return shots
}
