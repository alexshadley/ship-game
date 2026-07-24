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
	// survives each second, i.e. general drag. It applies to every body in the
	// space — ships, asteroids, and loose parts alike — so anything unpowered
	// gradually coasts to a stop.
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

	// projectileDamage is the part health removed by a single projectile hit. It's
	// a flat amount (unlike asteroid impacts, which scale with impulse), so cannon
	// fire chews through parts at a predictable rate.
	projectileDamage = 20.0
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

// shipBody binds a ship to its rigid body and controller. Each frame the
// controller emits Controls (thrust/turn/fire) that drive the body; the body's
// resulting motion is synced back onto the ship. Every ship — player or enemy —
// runs through one of these, so they all obey the same physics.
type shipBody struct {
	ship       *Ship
	body       *cp.Body
	controller Controller

	// shipShapes maps each attached part to its collision shape on this ship's
	// body, so a broken or stranded part's shape can be removed from the space.
	shipShapes map[*Part]*cp.Shape

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

	// looseParts and looseBodies run in parallel like the asteroid slices:
	// looseBodies[i] simulates looseParts[i], synced back onto it each step. They
	// are a shared debris field: parts broken off any ship end up here.
	looseParts  []*LoosePart
	looseBodies []*cp.Body
}

// NewPhysics builds a space containing the asteroids. Ships are added afterward
// with AddShip. The asteroids are circular bodies so they bounce off the ships
// and off one another.
func NewPhysics(asteroids []*Asteroid) *Physics {
	space := cp.NewSpace()
	// Global drag: no gravity, but every body sheds velocity over time so the
	// ship glides to a halt instead of drifting forever.
	space.SetDamping(spaceDamping)

	// Add each asteroid as its own circular body. Like every other body, asteroids
	// use the space's global damping, so rocks gradually coast to a halt too.
	asteroidBodies := make([]*cp.Body, 0, len(asteroids))
	for _, a := range asteroids {
		r := float64(a.Size)
		m := asteroidDensity * math.Pi * r * r
		ab := cp.NewBody(m, cp.MomentForCircle(m, 0, r, cp.Vector{}))
		ab.SetPosition(cp.Vector{X: float64(a.Position.X), Y: float64(a.Position.Y)})
		ab.SetVelocityVector(cp.Vector{X: float64(a.Velocity.X), Y: float64(a.Velocity.Y)})
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
		if part, ok := shipShape.UserData.(*Part); ok {
			damagePart(part, arb.TotalImpulse().Length())
		}
	}

	// Ship–ship contacts damage the struck part on both ships, again in proportion
	// to the collision impulse. Both shapes are ship parts, so damage is applied to
	// each of them.
	shipHandler := space.NewCollisionHandler(collisionShip, collisionShip)
	shipHandler.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		a, b := arb.Shapes()
		impulse := arb.TotalImpulse().Length()
		if part, ok := a.UserData.(*Part); ok {
			damagePart(part, impulse)
		}
		if part, ok := b.UserData.(*Part); ok {
			damagePart(part, impulse)
		}
	}

	return p
}

// damagePart removes health from a struck part in proportion to the collision
// impulse, clamping at zero. Shared by the ship–asteroid and ship–ship handlers.
func damagePart(part *Part, impulse float64) {
	part.Health -= float32(impulse) * damagePerImpulse
	if part.Health < 0 {
		part.Health = 0
	}
}

// ResolveProjectiles tests every projectile against every ship part and every
// asteroid, consuming any that connect. A projectile that strikes a ship part
// removes projectileDamage from that part's health; one that strikes an asteroid
// is destroyed with no effect (rocks shrug off cannon fire). Projectiles carry no
// team, so a ship's own shots can strike it — friendly fire is intentional. The
// surviving projectiles are returned (the input slice is filtered in place).
func (p *Physics) ResolveProjectiles(projectiles []*Projectile) []*Projectile {
	live := projectiles[:0]
	for _, pr := range projectiles {
		if p.projectileHit(pr) {
			continue
		}
		live = append(live, pr)
	}
	return live
}

// projectileHit reports whether pr connected with a ship part or an asteroid this
// frame, damaging a struck part. It tests the projectile's center point against
// each ship's grid and each asteroid's disc — cheap for the handful of shots in
// flight and precise enough at these speeds and sizes.
func (p *Physics) projectileHit(pr *Projectile) bool {
	for _, sb := range p.ships {
		if part := sb.ship.partAtWorld(pr.Position); part != nil {
			part.Health -= projectileDamage
			if part.Health < 0 {
				part.Health = 0
			}
			return true
		}
	}
	for _, a := range p.asteroids {
		dx := pr.Position.X - a.Position.X
		dy := pr.Position.Y - a.Position.Y
		if dx*dx+dy*dy <= a.Size*a.Size {
			return true
		}
	}
	return false
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

	// Track each part's shape so it can be removed if the part breaks off.
	shipShapes := make(map[*Part]*cp.Shape, len(ship.Parts))
	for c, part := range ship.Parts {
		center := cp.Vector{X: float64(c.X) * cellSize, Y: float64(c.Y) * cellSize}
		shape := p.space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(center, cellSize/2, cellSize/2), 0))
		shape.SetCollisionType(collisionShip)
		shape.SetElasticity(shipElasticity)
		shape.SetFriction(0.4)
		shape.UserData = part
		shipShapes[part] = shape
	}

	p.ships = append(p.ships, &shipBody{
		ship:       ship,
		body:       body,
		controller: controller,
		shipShapes: shipShapes,
		engines:    engines,
		thrusters:  thrusters,
	})
}

// LooseParts returns the parts that have broken free of any ship, for rendering.
func (p *Physics) LooseParts() []*LoosePart {
	return p.looseParts
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

	// Sync each body back onto its ship, cut loose any parts that broke this step,
	// then (for ships that survived) emit exhaust and resolve firing. Destroyed
	// ships are dropped from the list; their parts live on as loose debris.
	var projectiles []*Projectile
	survivors := p.ships[:0]
	for _, sb := range p.ships {
		pos := sb.body.Position()
		vel := sb.body.Velocity()
		sb.ship.Position = rl.NewVector2(float32(pos.X), float32(pos.Y))
		sb.ship.Direction = float32(sb.body.Angle())
		sb.ship.Velocity = rl.NewVector2(float32(vel.X), float32(vel.Y))
		sb.ship.AngularVelocity = float32(sb.body.AngularVelocity())

		// With the ship transform freshly synced, break off parts that died this
		// step (and any left stranded from the cockpit). May destroy the ship.
		p.handleBreakage(sb)
		if sb.ship.Destroyed {
			continue
		}
		survivors = append(survivors, sb)

		emitExhaust(sb.ship, sb.controls, particles)

		sb.fireCooldown -= float32(dt)
		if sb.controls.Fire && sb.fireCooldown <= 0 {
			projectiles = append(projectiles, sb.ship.FireCannons()...)
			sb.fireCooldown = cannonFireInterval
		}
	}
	p.ships = survivors

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

	return projectiles
}

// handleBreakage removes parts of sb whose health has reached zero and cuts loose
// any parts thereby stranded from the cockpit. A destroyed cockpit scatters the
// whole ship. It must run outside space.Step (bodies/shapes can't be mutated
// mid-step).
func (p *Physics) handleBreakage(sb *shipBody) {
	s := sb.ship

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
		p.destroyShip(sb)
		return
	}
	if len(broken) == 0 {
		return
	}

	// Broken parts vanish outright (they are not cut loose as debris).
	for _, c := range broken {
		p.removeShipPart(sb, c)
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
		p.spawnLoosePart(sb, c)
		p.removeShipPart(sb, c)
	}

	// Removing mass and engines/thrusters changes how the ship flies; rebuild it.
	p.recomputeShipBody(sb)
}

// removeShipPart deletes the part at c from sb's ship grid and removes its
// collision shape from the space. It leaves the ship body's mass untouched;
// callers rebuild that once after a batch of removals.
func (p *Physics) removeShipPart(sb *shipBody, c GridCoord) {
	part, ok := sb.ship.Parts[c]
	if !ok {
		return
	}
	if shape, ok := sb.shipShapes[part]; ok {
		p.space.RemoveShape(shape)
		delete(sb.shipShapes, part)
	}
	delete(sb.ship.Parts, c)
}

// spawnLoosePart creates a free-floating body for the part at grid coordinate c
// on sb's ship. The debris inherits the velocity of that point on the ship
// (linear plus the spin about the cockpit) so it flies off naturally, then coasts
// to a halt under the space's global damping. The caller still removes the part
// from the grid.
func (p *Physics) spawnLoosePart(sb *shipBody, c GridCoord) {
	part, ok := sb.ship.Parts[c]
	if !ok {
		return
	}
	worldPos := sb.ship.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)

	// Velocity of the ship at this world point: body velocity plus ω × r, where r
	// is the offset from the ship body's center (the cockpit origin).
	bodyPos := sb.body.Position()
	bodyVel := sb.body.Velocity()
	w := sb.body.AngularVelocity()
	rx := float64(worldPos.X) - bodyPos.X
	ry := float64(worldPos.Y) - bodyPos.Y
	vel := cp.Vector{X: bodyVel.X - w*ry, Y: bodyVel.Y + w*rx}

	m := float64(part.Weight)
	body := cp.NewBody(m, cp.MomentForBox(m, cellSize, cellSize))
	body.SetPosition(cp.Vector{X: float64(worldPos.X), Y: float64(worldPos.Y)})
	body.SetAngle(float64(sb.ship.Direction))
	body.SetVelocityVector(vel)
	body.SetAngularVelocity(w)
	p.space.AddBody(body)

	shape := p.space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(cp.Vector{}, cellSize/2, cellSize/2), 0))
	shape.SetCollisionType(collisionLoose)
	shape.SetElasticity(shipElasticity)
	shape.SetFriction(0.4)

	p.looseParts = append(p.looseParts, &LoosePart{
		Part:     part,
		Position: worldPos,
		Velocity: rl.NewVector2(float32(vel.X), float32(vel.Y)),
		Rotation: sb.ship.Direction,
	})
	p.looseBodies = append(p.looseBodies, body)
}

// destroyShip scatters every remaining part of sb as loose debris and removes its
// body from the space, marking the ship destroyed. Called when the cockpit is
// lost (or somehow already gone). The caller drops the ship from p.ships.
func (p *Physics) destroyShip(sb *shipBody) {
	s := sb.ship

	// Fling each part off before the body goes away (spawnLoosePart reads it).
	for c := range s.Parts {
		p.spawnLoosePart(sb, c)
	}
	for part, shape := range sb.shipShapes {
		p.space.RemoveShape(shape)
		delete(sb.shipShapes, part)
	}
	p.space.RemoveBody(sb.body)

	s.Parts = make(map[GridCoord]*Part)
	s.Destroyed = true
	sb.engines = 0
	sb.thrusters = 0
}

// recomputeShipBody recomputes sb's body mass, moment, and engine/thruster counts
// over the parts that remain after some broke off, keeping the simulation (and
// thrust/turn strength) consistent with the smaller ship.
func (p *Physics) recomputeShipBody(sb *shipBody) {
	var mass, moment float64
	var engines, thrusters int
	for c, part := range sb.ship.Parts {
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
		sb.body.SetMass(mass)
		sb.body.SetMoment(moment)
	}
	sb.engines = engines
	sb.thrusters = thrusters
}
