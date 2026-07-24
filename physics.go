package main

import (
	"math"

	"github.com/jakecoffman/cp"
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Physics tuning constants. Units are pixels, seconds, and pixel-mass (part
// weights), so forces are in pixel·mass/s² and torque in pixel²·mass/s².
const (
	// engineThrust is the forward force produced by a single engine while W is held.
	engineThrust = 1500.0
	// thrusterTorque is the turning torque produced by a single control thruster
	// while A or D is held.
	thrusterTorque = 90000.0
	// spaceDamping is the fraction of a body's linear and angular velocity that
	// survives each second, i.e. general drag. Below 1 the ship always coasts to
	// a stop when unpowered.
	spaceDamping = 0.55

	// shipElasticity and asteroidElasticity control how bouncy each body is in a
	// collision (0 = no rebound, 1 = fully elastic). Chipmunk multiplies the two
	// colliding shapes' values, so these keep ship–asteroid and especially
	// asteroid–asteroid hits springy.
	shipElasticity     = 0.4
	asteroidElasticity = 0.9

	// asteroidDensity converts an asteroid's area (π·r²) into its mass, so bigger
	// rocks are heavier and shove the ship around more when hit.
	asteroidDensity = 0.02

	// damagePerImpulse is how much part health is removed per unit of collision
	// impulse. Harder hits (bigger, faster asteroids) do proportionally more damage.
	damagePerImpulse = 0.05
)

// Collision types tag shapes so the space can route ship↔asteroid contacts to
// the damage handler. Untagged pairs still resolve physically (they bounce).
const (
	collisionShip     cp.CollisionType = 1
	collisionAsteroid cp.CollisionType = 2
	collisionPlayer   cp.CollisionType = 3
)

// Physics owns the Chipmunk space and the ship's rigid body, and keeps the
// ship's Position/Direction/Velocity in sync with the simulation.
type Physics struct {
	space *cp.Space
	body  *cp.Body
	ship  *Ship

	// asteroids and asteroidBodies run in parallel: asteroidBodies[i] is the
	// rigid body simulating asteroids[i], synced back onto it each step.
	asteroids      []*Asteroid
	asteroidBodies []*cp.Body

	engines   int // number of PartEngine parts (forward thrust)
	thrusters int // number of PartControlThruster parts (turning)

	// player, playerBody and playerShape exist only during a spacewalk: the
	// astronaut's simulated body, added to the space by AttachPlayer and removed
	// by DetachPlayer. All are nil while the pilot is aboard.
	player      *Player
	playerBody  *cp.Body
	playerShape *cp.Shape
}

// AttachPlayer adds a body for the spacewalking astronaut to the space at its
// current position and velocity, so it collides with the ship and asteroids. It
// carries its own collision type with no registered handler, so those collisions
// resolve as pure physical bounces and deal no damage to anything.
func (p *Physics) AttachPlayer(pl *Player) {
	moment := cp.MomentForCircle(playerMass, 0, playerRadius, cp.Vector{})
	body := cp.NewBody(playerMass, moment)
	body.SetPosition(cp.Vector{X: float64(pl.Position.X), Y: float64(pl.Position.Y)})
	body.SetVelocityVector(cp.Vector{X: float64(pl.Velocity.X), Y: float64(pl.Velocity.Y)})
	// Custom velocity update applies the astronaut's own gentle drag instead of the
	// space's default ship damping, preserving the light spacewalk drift.
	body.SetVelocityUpdateFunc(func(b *cp.Body, gravity cp.Vector, _, dt float64) {
		b.UpdateVelocity(gravity, math.Pow(playerDamping, dt), dt)
	})
	p.space.AddBody(body)

	shape := p.space.AddShape(cp.NewCircle(body, playerRadius, cp.Vector{}))
	shape.SetCollisionType(collisionPlayer)
	shape.SetElasticity(playerElasticity)
	shape.SetFriction(0.4)

	p.player = pl
	p.playerBody = body
	p.playerShape = shape
}

// DetachPlayer removes the astronaut's body from the space (on re-entry). It is a
// no-op if no spacewalk is in progress.
func (p *Physics) DetachPlayer() {
	if p.playerBody == nil {
		return
	}
	p.space.RemoveShape(p.playerShape)
	p.space.RemoveBody(p.playerBody)
	p.player = nil
	p.playerBody = nil
	p.playerShape = nil
}

// NewPhysics builds a space and a single rigid body for the ship. The body's
// center of gravity is the cockpit origin ({0,0} in the part grid), so the
// simulation rotates the ship about the same point the renderer does. The
// asteroids are added as their own circular bodies so they bounce off the ship
// and off one another.
func NewPhysics(ship *Ship, asteroids []*Asteroid) *Physics {
	space := cp.NewSpace()
	// Global drag: no gravity, but every body sheds velocity over time so the
	// ship glides to a halt instead of drifting forever.
	space.SetDamping(spaceDamping)

	// Sum mass and rotational inertia about the cockpit origin. Each part is a
	// cellSize box offset by its grid position, so its contribution is the box's
	// own moment plus the parallel-axis term m·r².
	var mass, moment float64
	var engines, thrusters int
	for c, p := range ship.Parts {
		m := float64(p.Weight)
		x := float64(c.X) * cellSize
		y := float64(c.Y) * cellSize
		mass += m
		moment += cp.MomentForBox(m, cellSize, cellSize) + m*(x*x+y*y)

		switch p.Type {
		case PartEngine:
			engines++
		case PartControlThruster:
			thrusters++
		}
	}

	body := cp.NewBody(mass, moment)
	body.SetPosition(cp.Vector{X: float64(ship.Position.X), Y: float64(ship.Position.Y)})
	body.SetAngle(float64(ship.Direction))
	space.AddBody(body)

	// Give each part a box collision shape at its grid offset so the ship's
	// outline is what asteroids strike. The shape carries a pointer to its part
	// so the collision handler knows exactly which part took the hit.
	for c, p := range ship.Parts {
		center := cp.Vector{X: float64(c.X) * cellSize, Y: float64(c.Y) * cellSize}
		shape := space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(center, cellSize/2, cellSize/2), 0))
		shape.SetCollisionType(collisionShip)
		shape.SetElasticity(shipElasticity)
		shape.SetFriction(0.4)
		shape.UserData = p
	}

	// Add each asteroid as its own circular body. A custom velocity function
	// cancels the global damping for asteroids only, so rocks coast through space
	// instead of dragging to a halt like the ship.
	asteroidBodies := make([]*cp.Body, 0, len(asteroids))
	for _, a := range asteroids {
		r := float64(a.Size)
		m := asteroidDensity * math.Pi * r * r
		ab := cp.NewBody(m, cp.MomentForCircle(m, 0, r, cp.Vector{}))
		ab.SetPosition(cp.Vector{X: float64(a.Position.X), Y: float64(a.Position.Y)})
		ab.SetVelocityVector(cp.Vector{X: float64(a.Velocity.X), Y: float64(a.Velocity.Y)})
		ab.SetVelocityUpdateFunc(func(b *cp.Body, gravity cp.Vector, _, dt float64) {
			b.UpdateVelocity(gravity, 1.0, dt)
		})
		space.AddBody(ab)

		shape := space.AddShape(cp.NewCircle(ab, r, cp.Vector{}))
		shape.SetCollisionType(collisionAsteroid)
		shape.SetElasticity(asteroidElasticity)
		shape.SetFriction(0.3)

		asteroidBodies = append(asteroidBodies, ab)
	}

	p := &Physics{
		space:          space,
		body:           body,
		ship:           ship,
		asteroids:      asteroids,
		asteroidBodies: asteroidBodies,
		engines:        engines,
		thrusters:      thrusters,
	}

	// On every ship–asteroid contact, damage the struck part in proportion to the
	// collision impulse. The physical bounce is handled by the solver itself; this
	// handler only reads the result back to apply damage.
	handler := space.NewCollisionHandler(collisionShip, collisionAsteroid)
	handler.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		shipShape, _ := arb.Shapes()
		part, ok := shipShape.UserData.(*Part)
		if !ok {
			return
		}
		part.Health -= float32(arb.TotalImpulse().Length()) * damagePerImpulse
		if part.Health < 0 {
			part.Health = 0
		}
	}

	return p
}

// Update steps the simulation by dt seconds and writes the resulting motion back
// onto the ship. When piloting is true it also reads thrust input; while the
// pilot is spacewalking (piloting false) the ship takes no control input and
// simply coasts, though the simulation keeps running so it and the asteroids
// still drift and collide.
//
//	W     - fire all engines: forward thrust along the ship's heading.
//	A / D - fire the control thrusters: turn left / right.
func (p *Physics) Update(dt float64, piloting bool) {
	// GetFrameTime reports 0 on the first frame and can spike after a stall; clamp
	// to a sane range so the integrator never divides by zero or takes a huge step.
	if dt <= 0 {
		return
	}
	if dt > 1.0/30 {
		dt = 1.0 / 30
	}

	// Rebuild the force/torque accumulators from scratch each frame so releasing
	// a key immediately stops that input.
	force := cp.Vector{}
	if piloting && rl.IsKeyDown(rl.KeyW) {
		// Forward is -Y in the ship's local frame (toward the nose).
		local := cp.Vector{X: 0, Y: -engineThrust * float64(p.engines)}
		force = p.body.Rotation().Rotate(local)
	}
	p.body.SetForce(force)

	var torque float64
	if piloting {
		if rl.IsKeyDown(rl.KeyA) {
			torque -= thrusterTorque * float64(p.thrusters)
		}
		if rl.IsKeyDown(rl.KeyD) {
			torque += thrusterTorque * float64(p.thrusters)
		}
	}
	p.body.SetTorque(torque)

	// Drive the spacewalking astronaut with WASD, as a force scaled by its mass so
	// the acceleration matches playerThrust regardless of mass.
	if p.playerBody != nil {
		dx, dy := walkInputDir()
		p.playerBody.SetForce(cp.Vector{
			X: dx * playerMass * playerThrust,
			Y: dy * playerMass * playerThrust,
		})
	}

	p.space.Step(dt)

	pos := p.body.Position()
	vel := p.body.Velocity()
	p.ship.Position = rl.NewVector2(float32(pos.X), float32(pos.Y))
	p.ship.Direction = float32(p.body.Angle())
	p.ship.Velocity = rl.NewVector2(float32(vel.X), float32(vel.Y))

	// Write each asteroid body's motion back onto the asteroid it simulates.
	for i, a := range p.asteroids {
		apos := p.asteroidBodies[i].Position()
		avel := p.asteroidBodies[i].Velocity()
		a.Position = rl.NewVector2(float32(apos.X), float32(apos.Y))
		a.Velocity = rl.NewVector2(float32(avel.X), float32(avel.Y))
	}

	// Sync the astronaut's simulated motion back for rendering and re-entry checks.
	if p.playerBody != nil && p.player != nil {
		ppos := p.playerBody.Position()
		pvel := p.playerBody.Velocity()
		p.player.Position = rl.NewVector2(float32(ppos.X), float32(ppos.Y))
		p.player.Velocity = rl.NewVector2(float32(pvel.X), float32(pvel.Y))
	}
}
