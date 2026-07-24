package main

import (
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Exhaust tuning. Distances are in world pixels (the same frame as the ship's
// parts, so a cell is cellSize wide), and times are in seconds.
const (
	// exhaustSpeed is how fast a particle leaves the nozzle, relative to the ship.
	exhaustSpeed = 170.0
	// exhaustSpeedJitter randomizes each particle's speed by ±this amount.
	exhaustSpeedJitter = 45.0
	// exhaustSpread is the random angular scatter, in radians, applied to the
	// outward direction so the plume fans out slightly instead of being a line.
	exhaustSpread = 0.28
	// exhaustLifetime is how long a particle lives before it disappears.
	exhaustLifetime = 0.45
	// Particles grow from the start radius to the end radius over their lifetime.
	exhaustStartRadius = 3.0
	exhaustEndRadius    = 11.0

	// Particles spawned per active nozzle per frame.
	engineParticlesPerFrame   = 1
	thrusterParticlesPerFrame = 1
)

// Exhaust plume color: a soft grey-white puff for every thruster.
var (
	engineExhaustColor   = rl.NewColor(230, 230, 235, 255)
	thrusterExhaustColor = rl.NewColor(230, 230, 235, 255)
)

// particle is a single expanding puff of exhaust in world space.
type particle struct {
	pos      rl.Vector2
	vel      rl.Vector2
	age      float32
	lifetime float32
	color    rl.Color
}

// ParticleSystem holds and advances all live exhaust particles.
type ParticleSystem struct {
	particles []particle
}

// NewParticleSystem returns an empty particle system.
func NewParticleSystem() *ParticleSystem {
	return &ParticleSystem{}
}

// Emit spawns a particle at pos travelling outward along dir (a unit vector).
// shipVel is added so the plume drifts with the ship instead of hanging in place.
func (ps *ParticleSystem) Emit(pos, dir, shipVel rl.Vector2, color rl.Color) {
	ang := math.Atan2(float64(dir.Y), float64(dir.X))
	ang += float64((rand.Float32()*2 - 1) * exhaustSpread)
	speed := exhaustSpeed + float64((rand.Float32()*2-1)*exhaustSpeedJitter)

	vx := float32(math.Cos(ang) * speed)
	vy := float32(math.Sin(ang) * speed)

	ps.particles = append(ps.particles, particle{
		pos:      pos,
		vel:      rl.NewVector2(shipVel.X+vx, shipVel.Y+vy),
		lifetime: exhaustLifetime,
		color:    color,
	})
}

// Update advances every particle by dt seconds and drops any that have expired.
func (ps *ParticleSystem) Update(dt float32) {
	if dt <= 0 {
		return
	}
	alive := ps.particles[:0]
	for _, p := range ps.particles {
		p.age += dt
		if p.age >= p.lifetime {
			continue
		}
		p.pos.X += p.vel.X * dt
		p.pos.Y += p.vel.Y * dt
		alive = append(alive, p)
	}
	ps.particles = alive
}

// Draw renders each particle as a circle that grows and fades over its lifetime.
// Call this inside the 2D camera so particles share the ship's world frame.
func (ps *ParticleSystem) Draw() {
	for _, p := range ps.particles {
		t := p.age / p.lifetime
		radius := exhaustStartRadius + t*(exhaustEndRadius-exhaustStartRadius)
		c := p.color
		c.A = uint8(float32(p.color.A) * (1 - t))
		rl.DrawCircleV(p.pos, radius, c)
	}
}
