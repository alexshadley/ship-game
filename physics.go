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
	// collisionLoose tags a broken-off part. Loose parts have no damage handler
	// registered against any type, so they bounce off ships, asteroids, and one
	// another via the solver but never deal or take damage.
	collisionLoose cp.CollisionType = 3
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

	// shipShapes maps each attached part to its collision shape on the ship body,
	// so a broken or stranded part's shape can be removed from the space.
	shipShapes map[*Part]*cp.Shape

	// looseParts and looseBodies run in parallel like the asteroid slices:
	// looseBodies[i] simulates looseParts[i], synced back onto it each step.
	looseParts  []*LoosePart
	looseBodies []*cp.Body

	engines   int // number of PartEngine parts (forward thrust)
	thrusters int // number of PartControlThruster parts (turning)
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
	// so the collision handler knows exactly which part took the hit, and is
	// tracked in shipShapes so it can be removed if the part breaks off.
	shipShapes := make(map[*Part]*cp.Shape, len(ship.Parts))
	for c, p := range ship.Parts {
		center := cp.Vector{X: float64(c.X) * cellSize, Y: float64(c.Y) * cellSize}
		shape := space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(center, cellSize/2, cellSize/2), 0))
		shape.SetCollisionType(collisionShip)
		shape.SetElasticity(shipElasticity)
		shape.SetFriction(0.4)
		shape.UserData = p
		shipShapes[p] = shape
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
		shipShapes:     shipShapes,
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

	// A destroyed ship has no body in the space; skip its thrust input entirely.
	if !p.ship.Destroyed {
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
	}

	p.space.Step(dt)

	if !p.ship.Destroyed {
		pos := p.body.Position()
		vel := p.body.Velocity()
		p.ship.Position = rl.NewVector2(float32(pos.X), float32(pos.Y))
		p.ship.Direction = float32(p.body.Angle())
		p.ship.Velocity = rl.NewVector2(float32(vel.X), float32(vel.Y))

		// With the ship transform freshly synced, cut loose any parts that broke
		// this step (and, in turn, any left stranded from the cockpit).
		p.handleBreakage()
	}

	// Write each asteroid body's motion back onto the asteroid it simulates.
	for i, a := range p.asteroids {
		apos := p.asteroidBodies[i].Position()
		avel := p.asteroidBodies[i].Velocity()
		a.Position = rl.NewVector2(float32(apos.X), float32(apos.Y))
		a.Velocity = rl.NewVector2(float32(avel.X), float32(avel.Y))
	}

	// Likewise sync each loose part's tumbling body back onto it for rendering.
	for i, l := range p.looseParts {
		lb := p.looseBodies[i]
		lpos := lb.Position()
		lvel := lb.Velocity()
		l.Position = rl.NewVector2(float32(lpos.X), float32(lpos.Y))
		l.Velocity = rl.NewVector2(float32(lvel.X), float32(lvel.Y))
		l.Rotation = float32(lb.Angle())
	}
}

// LooseParts returns the parts that have broken free of the ship, for rendering.
func (p *Physics) LooseParts() []*LoosePart {
	return p.looseParts
}

// handleBreakage removes parts whose health has reached zero and cuts loose any
// parts thereby stranded from the cockpit. A destroyed cockpit scatters the whole
// ship. It must run outside space.Step (bodies/shapes can't be mutated mid-step).
func (p *Physics) handleBreakage() {
	s := p.ship

	cockpit, hasCockpit := s.Cockpit()

	// Collect the parts that broke this step. Losing the cockpit destroys the ship.
	var broken []GridCoord
	cockpitBroken := false
	for c, part := range s.Parts {
		if part.Health <= 0 {
			broken = append(broken, c)
			if part.Type == PartCockpit {
				cockpitBroken = true
			}
		}
	}
	if !hasCockpit || cockpitBroken {
		p.destroyShip()
		return
	}
	if len(broken) == 0 {
		return
	}

	// Broken parts vanish outright (they are not cut loose as debris).
	for _, c := range broken {
		p.removeShipPart(c)
	}

	// Any part no longer connected to the cockpit breaks free as a loose part.
	connected := s.connectedParts(cockpit)
	var stranded []GridCoord
	for c := range s.Parts {
		if !connected[c] {
			stranded = append(stranded, c)
		}
	}
	for _, c := range stranded {
		p.spawnLoosePart(s.Parts[c], c)
		p.removeShipPart(c)
	}

	// Removing mass and engines/thrusters changes how the ship flies; rebuild it.
	p.recomputeShipBody()
}

// removeShipPart deletes the part at c from the ship grid and removes its
// collision shape from the space. It leaves the ship body's mass untouched;
// callers rebuild that once after a batch of removals.
func (p *Physics) removeShipPart(c GridCoord) {
	part, ok := p.ship.Parts[c]
	if !ok {
		return
	}
	if shape, ok := p.shipShapes[part]; ok {
		p.space.RemoveShape(shape)
		delete(p.shipShapes, part)
	}
	delete(p.ship.Parts, c)
}

// spawnLoosePart creates a free-floating body for part, which currently sits at
// grid coordinate c on the ship. The debris inherits the velocity of that point
// on the ship (linear plus the spin about the cockpit) so it flies off naturally,
// and coasts without drag like an asteroid. The caller still removes the part
// from the ship grid.
func (p *Physics) spawnLoosePart(part *Part, c GridCoord) {
	worldPos := p.ship.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)

	// Velocity of the ship at this world point: body velocity plus ω × r, where r
	// is the offset from the ship body's center (the cockpit origin).
	bodyPos := p.body.Position()
	bodyVel := p.body.Velocity()
	w := p.body.AngularVelocity()
	rx := float64(worldPos.X) - bodyPos.X
	ry := float64(worldPos.Y) - bodyPos.Y
	vel := cp.Vector{X: bodyVel.X - w*ry, Y: bodyVel.Y + w*rx}

	m := float64(part.Weight)
	body := cp.NewBody(m, cp.MomentForBox(m, cellSize, cellSize))
	body.SetPosition(cp.Vector{X: float64(worldPos.X), Y: float64(worldPos.Y)})
	body.SetAngle(float64(p.ship.Direction))
	body.SetVelocityVector(vel)
	body.SetAngularVelocity(w)
	// Cancel global damping so debris coasts through space like the asteroids do.
	body.SetVelocityUpdateFunc(func(b *cp.Body, gravity cp.Vector, _, dt float64) {
		b.UpdateVelocity(gravity, 1.0, dt)
	})
	p.space.AddBody(body)

	shape := p.space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(cp.Vector{}, cellSize/2, cellSize/2), 0))
	shape.SetCollisionType(collisionLoose)
	shape.SetElasticity(shipElasticity)
	shape.SetFriction(0.4)

	p.looseParts = append(p.looseParts, &LoosePart{
		Part:     part,
		Position: worldPos,
		Velocity: rl.NewVector2(float32(vel.X), float32(vel.Y)),
		Rotation: p.ship.Direction,
	})
	p.looseBodies = append(p.looseBodies, body)
}

// destroyShip scatters every remaining part as loose debris and removes the ship
// body from the space, marking the ship destroyed. Called when the cockpit is
// lost (or somehow already gone).
func (p *Physics) destroyShip() {
	s := p.ship

	// Fling each part off before the body goes away (spawnLoosePart reads it).
	for c, part := range s.Parts {
		p.spawnLoosePart(part, c)
	}
	for part, shape := range p.shipShapes {
		p.space.RemoveShape(shape)
		delete(p.shipShapes, part)
	}
	p.space.RemoveBody(p.body)

	s.Parts = make(map[GridCoord]*Part)
	s.Destroyed = true
	p.engines = 0
	p.thrusters = 0
}

// recomputeShipBody recomputes the ship body's mass, moment, and engine/thruster
// counts over the parts that remain after some broke off, keeping the simulation
// (and thrust/turn strength) consistent with the smaller ship.
func (p *Physics) recomputeShipBody() {
	var mass, moment float64
	var engines, thrusters int
	for c, part := range p.ship.Parts {
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
	if mass > 0 {
		p.body.SetMass(mass)
		p.body.SetMoment(moment)
	}
	p.engines = engines
	p.thrusters = thrusters
}
