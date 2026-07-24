package main

import (
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
)

// Physics owns the Chipmunk space and the ship's rigid body, and keeps the
// ship's Position/Direction/Velocity in sync with the simulation.
type Physics struct {
	space *cp.Space
	body  *cp.Body
	ship  *Ship

	engines   int // number of PartEngine parts (forward thrust)
	thrusters int // number of PartControlThruster parts (turning)
}

// NewPhysics builds a space and a single rigid body for the ship. The body's
// center of gravity is the cockpit origin ({0,0} in the part grid), so the
// simulation rotates the ship about the same point the renderer does.
func NewPhysics(ship *Ship) *Physics {
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

	return &Physics{
		space:     space,
		body:      body,
		ship:      ship,
		engines:   engines,
		thrusters: thrusters,
	}
}

// Update reads thrust input, steps the simulation by dt seconds, and writes the
// resulting motion back onto the ship.
//
//	W     - fire all engines: forward thrust along the ship's heading.
//	A / D - fire the control thrusters: turn left / right.
func (p *Physics) Update(dt float64) {
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
	if rl.IsKeyDown(rl.KeyW) {
		// Forward is -Y in the ship's local frame (toward the nose/cockpit).
		local := cp.Vector{X: 0, Y: -engineThrust * float64(p.engines)}
		force = p.body.Rotation().Rotate(local)
	}
	p.body.SetForce(force)

	var torque float64
	if rl.IsKeyDown(rl.KeyA) {
		torque -= thrusterTorque * float64(p.thrusters)
	}
	if rl.IsKeyDown(rl.KeyD) {
		torque += thrusterTorque * float64(p.thrusters)
	}
	p.body.SetTorque(torque)

	p.space.Step(dt)

	pos := p.body.Position()
	vel := p.body.Velocity()
	p.ship.Position = rl.NewVector2(float32(pos.X), float32(pos.Y))
	p.ship.Direction = float32(p.body.Angle())
	p.ship.Velocity = rl.NewVector2(float32(vel.X), float32(vel.Y))
}
