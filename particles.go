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

	// A missile's motor plume is the same kind of exhaust as an engine's, just a
	// smaller, tighter, shorter-lived version.
	missileExhaustScale    = 0.55
	missileExhaustSpeed    = 90.0
	missileExhaustSpread   = 0.18
	missileExhaustLifetime = 0.3

	// An explosion animation: a bright core flash plus a shockwave ring that
	// expands out to the full blast radius so its reach is visible.
	explosionLifetime  = 0.4
	explosionRingWidth = 4.0
)

var (
	explosionCoreColor = rl.NewColor(255, 220, 130, 255)
	explosionRingColor = rl.NewColor(255, 130, 50, 255)
)

var (
	engineExhaustColor   = rl.NewColor(230, 230, 235, 255)
	thrusterExhaustColor = rl.NewColor(230, 230, 235, 255)
	missileExhaustColor  = rl.NewColor(230, 230, 235, 255)
)

type particle struct {
	pos      rl.Vector2
	vel      rl.Vector2
	age      float32
	lifetime float32
	color    rl.Color
	// scale multiplies the age-driven radius so plumes of different sizes (a
	// missile's small motor vs. an engine's) can share one system.
	scale float32
}

// explosion is a short-lived blast animation centred on a detonation: its core
// flashes and fades while a ring expands out to radius (the blast's reach).
type explosion struct {
	pos      rl.Vector2
	radius   float32
	age      float32
	lifetime float32
}

type ParticleSystem struct {
	particles  []particle
	explosions []explosion
}

func NewParticleSystem() *ParticleSystem {
	return &ParticleSystem{}
}

// Emit spawns a particle at pos travelling outward along dir (a unit vector).
// shipVel is added so the plume drifts with the ship instead of hanging in place.
func (ps *ParticleSystem) Emit(pos, dir, shipVel rl.Vector2, color rl.Color) {
	ps.emit(pos, dir, shipVel, color, exhaustSpeed, exhaustSpread, exhaustLifetime, 1)
}

// EmitMissile spawns a scaled-down exhaust plume for a missile's motor: same
// concept as an engine plume (Emit), just smaller, tighter, and shorter-lived.
func (ps *ParticleSystem) EmitMissile(pos, dir, missileVel rl.Vector2) {
	ps.emit(pos, dir, missileVel, missileExhaustColor,
		missileExhaustSpeed, missileExhaustSpread, missileExhaustLifetime, missileExhaustScale)
}

// EmitTrail drops a lingering puff at pos to mark a projectile's flight path. It
// hangs nearly in place (unlike an exhaust plume, which is flung outward) and fades
// over lifetime; scale sizes it, so a caller passing a speed-proportional scale
// lays a trail that fattens with velocity.
func (ps *ParticleSystem) EmitTrail(pos rl.Vector2, color rl.Color, lifetime, scale float32) {
	ps.particles = append(ps.particles, particle{
		pos:      pos,
		lifetime: lifetime,
		color:    color,
		scale:    scale,
	})
}

// emit spawns a single exhaust particle at pos travelling outward along dir (a
// unit vector). baseVel is added so the plume drifts with its source instead of
// hanging in place; scale sizes the particle relative to the default plume.
func (ps *ParticleSystem) emit(pos, dir, baseVel rl.Vector2, color rl.Color, speed, spread, lifetime, scale float32) {
	ang := math.Atan2(float64(dir.Y), float64(dir.X))
	ang += float64((rand.Float32()*2 - 1) * spread)
	sp := float64(speed) + float64((rand.Float32()*2-1)*exhaustSpeedJitter*scale)

	vx := float32(math.Cos(ang) * sp)
	vy := float32(math.Sin(ang) * sp)

	ps.particles = append(ps.particles, particle{
		pos:      pos,
		vel:      rl.NewVector2(baseVel.X+vx, baseVel.Y+vy),
		lifetime: lifetime,
		color:    color,
		scale:    scale,
	})
}

// SpawnExplosion starts a blast animation centred at pos whose shockwave ring
// expands out to radius, giving a visible read on the detonation's reach.
func (ps *ParticleSystem) SpawnExplosion(pos rl.Vector2, radius float32) {
	ps.explosions = append(ps.explosions, explosion{
		pos:      pos,
		radius:   radius,
		lifetime: explosionLifetime,
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

	liveBlasts := ps.explosions[:0]
	for _, e := range ps.explosions {
		e.age += dt
		if e.age >= e.lifetime {
			continue
		}
		liveBlasts = append(liveBlasts, e)
	}
	ps.explosions = liveBlasts
}

// UpdateParticles advances the particle system as its own frame pass, run after
// Game.Update. Particles are purely cosmetic — they never feed back into the
// simulation — so they live outside the gameplay update. They freeze alongside
// the rest of the world while paused or after game over; Draw still owns
// rendering them.
func (g *Game) UpdateParticles(dt float32) {
	if g.state == StatePlaying && !g.gameOver {
		g.particles.Update(dt)
	}
}

func (ps *ParticleSystem) Draw() {
	for _, p := range ps.particles {
		t := p.age / p.lifetime
		radius := (exhaustStartRadius + t*(exhaustEndRadius-exhaustStartRadius)) * p.scale
		c := p.color
		c.A = uint8(float32(p.color.A) * (1 - t))
		rl.DrawCircleV(p.pos, radius, c)
	}
	ps.drawExplosions()
}

func (ps *ParticleSystem) drawExplosions() {
	for _, e := range ps.explosions {
		t := e.age / e.lifetime // 0..1 over the blast's life
		fade := 1 - t

		// The shockwave races out fast then eases into the full radius, so the ring
		// lands right on the blast's edge as it fades.
		ringR := e.radius * (1 - fade*fade)
		innerR := ringR - explosionRingWidth
		if innerR < 0 {
			innerR = 0
		}
		ring := explosionRingColor
		ring.A = uint8(float32(explosionRingColor.A) * fade)
		rl.DrawRing(e.pos, innerR, ringR, 0, 360, 48, ring)

		// A bright core flash shrinks and fades inside the ring.
		core := explosionCoreColor
		core.A = uint8(float32(explosionCoreColor.A) * fade)
		rl.DrawCircleV(e.pos, e.radius*0.45*fade, core)
	}
}
