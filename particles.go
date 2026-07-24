package main

import (
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	exhaustSpeed       = 170.0
	exhaustSpeedJitter = 45.0
	exhaustSpread      = 0.28
	exhaustLifetime    = 0.45
	exhaustStartRadius = 3.0
	exhaustEndRadius   = 11.0

	engineParticlesPerFrame   = 1
	thrusterParticlesPerFrame = 1
)

var (
	engineExhaustColor   = rl.NewColor(230, 230, 235, 255)
	thrusterExhaustColor = rl.NewColor(230, 230, 235, 255)
)

type particle struct {
	pos      rl.Vector2
	vel      rl.Vector2
	age      float32
	lifetime float32
	color    rl.Color
}

type ParticleSystem struct {
	particles []particle
}

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

func (ps *ParticleSystem) Draw() {
	for _, p := range ps.particles {
		t := p.age / p.lifetime
		radius := exhaustStartRadius + t*(exhaustEndRadius-exhaustStartRadius)
		c := p.color
		c.A = uint8(float32(p.color.A) * (1 - t))
		rl.DrawCircleV(p.pos, radius, c)
	}
}
