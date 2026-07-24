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
)

// shipBody binds a ship to its rigid body and controller. Each frame the
// controller emits Controls (thrust/turn/fire) that drive the body; the body's
// resulting motion is synced back onto the ship. Every ship — player or enemy —
// runs through one of these, so they all obey the same physics.
type shipBody struct {
	ship       *Ship
	body       *cp.Body
	controller Controller

	engines      int      // number of PartEngine parts (forward thrust)
	thrusters    int      // number of PartControlThruster parts (turning)
	fireCooldown float32  // time until the cannons may fire again
	controls     Controls // this frame's controls, captured before the step
}

// Physics owns the Chipmunk space and every rigid body in it, and keeps each
// ship's and asteroid's kinematic state in sync with the simulation.
type Physics struct {
	space *cp.Space
	ships []*shipBody

	// asteroids and asteroidBodies run in parallel: asteroidBodies[i] is the
	// rigid body simulating asteroids[i], synced back onto it each step.
	asteroids      []*Asteroid
	asteroidBodies []*cp.Body
}

// NewPhysics builds a space containing the asteroids. Ships are added afterward
// with AddShip. The asteroids are circular bodies so they bounce off the ships
// and off one another.
func NewPhysics(asteroids []*Asteroid) *Physics {
	space := cp.NewSpace()
	// Global drag: no gravity, but every body sheds velocity over time so the
	// ship glides to a halt instead of drifting forever.
	space.SetDamping(spaceDamping)

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
		asteroids:      asteroids,
		asteroidBodies: asteroidBodies,
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

// AddShip builds a rigid body for ship and registers controller as the source of
// its Controls. The body's center of gravity is the cockpit origin ({0,0} in the
// part grid), so the simulation rotates the ship about the same point the
// renderer does. Each part gets a box collision shape at its grid offset carrying
// a pointer to its part, so the collision handler knows exactly which part is hit.
func (p *Physics) AddShip(ship *Ship, controller Controller) {
	// Sum mass and rotational inertia about the cockpit origin. Each part is a
	// cellSize box offset by its grid position, so its contribution is the box's
	// own moment plus the parallel-axis term m·r².
	var mass, moment float64
	var engines, thrusters int
	for c, part := range ship.Parts {
		m := float64(part.Weight)
		x := float64(c.X) * cellSize
		y := float64(c.Y) * cellSize
		mass += m
		moment += cp.MomentForBox(m, cellSize, cellSize) + m*(x*x+y*y)

		switch part.Type {
		case PartEngine:
			engines++
		case PartControlThruster:
			thrusters++
		}
	}

	body := cp.NewBody(mass, moment)
	body.SetPosition(cp.Vector{X: float64(ship.Position.X), Y: float64(ship.Position.Y)})
	body.SetAngle(float64(ship.Direction))
	p.space.AddBody(body)

	for c, part := range ship.Parts {
		center := cp.Vector{X: float64(c.X) * cellSize, Y: float64(c.Y) * cellSize}
		shape := p.space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(center, cellSize/2, cellSize/2), 0))
		shape.SetCollisionType(collisionShip)
		shape.SetElasticity(shipElasticity)
		shape.SetFriction(0.4)
		shape.UserData = part
	}

	p.ships = append(p.ships, &shipBody{
		ship:       ship,
		body:       body,
		controller: controller,
		engines:    engines,
		thrusters:  thrusters,
	})
}

// Update pulls Controls from every ship's controller, applies them as force and
// torque, steps the simulation by dt seconds, and writes the resulting motion
// back onto each ship. Each ship emits exhaust into particles for whatever it's
// firing, and ships that fired their cannons spawn projectiles, which are
// returned. Each ship's Controls come from either player input or AI, but the
// force/torque/exhaust/fire handling below is identical for all of them.
func (p *Physics) Update(dt float64, particles *ParticleSystem) []*Projectile {
	// GetFrameTime reports 0 on the first frame and can spike after a stall; clamp
	// to a sane range so the integrator never divides by zero or takes a huge step.
	if dt <= 0 {
		return nil
	}
	if dt > 1.0/30 {
		dt = 1.0 / 30
	}

	// Apply each ship's controls. Force and torque are rebuilt from scratch every
	// frame so dropping a control (key up, or the AI easing off) stops it at once.
	for _, sb := range p.ships {
		controls := sb.controller.Controls(float32(dt))

		force := cp.Vector{}
		if controls.Thrust != 0 {
			// Forward is -Y in the ship's local frame (toward the nose/cockpit).
			local := cp.Vector{X: 0, Y: -engineThrust * float64(sb.engines) * float64(controls.Thrust)}
			force = sb.body.Rotation().Rotate(local)
		}
		sb.body.SetForce(force)
		sb.body.SetTorque(thrusterTorque * float64(sb.thrusters) * float64(controls.Turn))
		sb.controls = controls
	}

	p.space.Step(dt)

	// Sync each body back onto its ship, then (after the step, so plumes and muzzles
	// sit at the ship's post-step position) emit exhaust and resolve firing.
	var projectiles []*Projectile
	for _, sb := range p.ships {
		pos := sb.body.Position()
		vel := sb.body.Velocity()
		sb.ship.Position = rl.NewVector2(float32(pos.X), float32(pos.Y))
		sb.ship.Direction = float32(sb.body.Angle())
		sb.ship.Velocity = rl.NewVector2(float32(vel.X), float32(vel.Y))
		sb.ship.AngularVelocity = float32(sb.body.AngularVelocity())

		emitExhaust(sb.ship, sb.controls, particles)

		sb.fireCooldown -= float32(dt)
		if sb.controls.Fire && sb.fireCooldown <= 0 {
			projectiles = append(projectiles, sb.ship.FireCannons()...)
			sb.fireCooldown = cannonFireInterval
		}
	}

	// Write each asteroid body's motion back onto the asteroid it simulates.
	for i, a := range p.asteroids {
		apos := p.asteroidBodies[i].Position()
		avel := p.asteroidBodies[i].Velocity()
		a.Position = rl.NewVector2(float32(apos.X), float32(apos.Y))
		a.Velocity = rl.NewVector2(float32(avel.X), float32(avel.Y))
	}

	return projectiles
}
