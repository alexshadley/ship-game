package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/jakecoffman/cp"
)

const (
	engineThrust   = 1500.0
	thrusterTorque = 90000.0
	// spaceDamping is the fraction of velocity that survives each second. Below 1
	// the ship always coasts to a stop when unpowered.
	spaceDamping = 0.55

	shipElasticity     = 0.4
	asteroidElasticity = 0.9

	asteroidDensity = 0.02

	damagePerImpulse = 0.05
	projectileDamage = 20.0
)

const (
	collisionShip     cp.CollisionType = 1
	collisionAsteroid cp.CollisionType = 2
	// collisionLoose and collisionPlayer have no damage handler registered against
	// any type, so those bodies bounce off everything via the solver but never deal
	// or take damage.
	collisionLoose  cp.CollisionType = 3
	collisionPlayer cp.CollisionType = 4
)

type shipBody struct {
	ship       *Ship
	body       *cp.Body
	controller Controller

	shipShapes map[*Part]*cp.Shape

	engines      int
	thrusters    int
	fireCooldown float32
	controls     Controls
}

type Physics struct {
	space *cp.Space
	ships []*shipBody

	asteroids      []*Asteroid
	asteroidBodies []*cp.Body

	looseParts  []*LoosePart
	looseBodies []*cp.Body

	player      *Player
	playerBody  *cp.Body
	playerShape *cp.Shape
}

func (p *Physics) AttachPlayer(pl *Player) {
	moment := cp.MomentForCircle(playerMass, 0, playerRadius, cp.Vector{})
	body := cp.NewBody(playerMass, moment)
	body.SetPosition(cp.Vector{X: float64(pl.Position.X), Y: float64(pl.Position.Y)})
	body.SetVelocityVector(cp.Vector{X: float64(pl.Velocity.X), Y: float64(pl.Velocity.Y)})
	// Apply the astronaut's own gentle drag instead of the space's ship damping.
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

func NewPhysics(asteroids []*Asteroid) *Physics {
	space := cp.NewSpace()
	space.SetDamping(spaceDamping)

	asteroidBodies := make([]*cp.Body, 0, len(asteroids))
	for _, a := range asteroids {
		r := float64(a.Size)
		m := asteroidDensity * math.Pi * r * r
		ab := cp.NewBody(m, cp.MomentForCircle(m, 0, r, cp.Vector{}))
		ab.SetPosition(cp.Vector{X: float64(a.Position.X), Y: float64(a.Position.Y)})
		ab.SetVelocityVector(cp.Vector{X: float64(a.Velocity.X), Y: float64(a.Velocity.Y)})
		// Cancel the global damping (multiplier 1.0) so rocks coast forever.
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

	// The solver handles the bounce; this handler only reads the impulse back to
	// apply damage to the struck part.
	handler := space.NewCollisionHandler(collisionShip, collisionAsteroid)
	handler.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		shipShape, _ := arb.Shapes()
		if part, ok := shipShape.UserData.(*Part); ok {
			damagePart(part, arb.TotalImpulse().Length())
		}
	}

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

func damagePart(part *Part, impulse float64) {
	part.Health -= float32(impulse) * damagePerImpulse
	if part.Health < 0 {
		part.Health = 0
	}
}

// ResolveProjectiles tests every projectile against every ship part and asteroid,
// consuming any that connect. Projectiles carry no team, so a ship's own shots can
// strike it — friendly fire is intentional. Survivors are returned.
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
// its Controls. The body's center of gravity is the cockpit origin, so the sim
// rotates the ship about the same point the renderer does.
func (p *Physics) AddShip(ship *Ship, controller Controller) {
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

func (p *Physics) LooseParts() []*LoosePart {
	return p.looseParts
}

func (p *Physics) Update(dt float64, particles *ParticleSystem) []*Projectile {
	// GetFrameTime reports 0 on the first frame and can spike after a stall; clamp
	// so the integrator never divides by zero or takes a huge step.
	if dt <= 0 {
		return nil
	}
	if dt > 1.0/30 {
		dt = 1.0 / 30
	}

	for _, sb := range p.ships {
		controls := sb.controller.Controls(float32(dt))

		force := cp.Vector{}
		if controls.Thrust != 0 {
			local := cp.Vector{X: 0, Y: -engineThrust * float64(sb.engines) * float64(controls.Thrust)}
			force = sb.body.Rotation().Rotate(local)
		}
		sb.body.SetForce(force)
		sb.body.SetTorque(thrusterTorque * float64(sb.thrusters) * float64(controls.Turn))
		sb.controls = controls
	}

	// Drive the astronaut with a force scaled by its mass so the acceleration
	// matches playerThrust regardless of mass.
	if p.playerBody != nil {
		dx, dy := walkInputDir()
		p.playerBody.SetForce(cp.Vector{
			X: dx * playerMass * playerThrust,
			Y: dy * playerMass * playerThrust,
		})
	}

	p.space.Step(dt)

	var projectiles []*Projectile
	survivors := p.ships[:0]
	for _, sb := range p.ships {
		pos := sb.body.Position()
		vel := sb.body.Velocity()
		sb.ship.Position = rl.NewVector2(float32(pos.X), float32(pos.Y))
		sb.ship.Direction = float32(sb.body.Angle())
		sb.ship.Velocity = rl.NewVector2(float32(vel.X), float32(vel.Y))
		sb.ship.AngularVelocity = float32(sb.body.AngularVelocity())

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

	for i, a := range p.asteroids {
		apos := p.asteroidBodies[i].Position()
		avel := p.asteroidBodies[i].Velocity()
		a.Position = rl.NewVector2(float32(apos.X), float32(apos.Y))
		a.Velocity = rl.NewVector2(float32(avel.X), float32(avel.Y))
	}

	for i, l := range p.looseParts {
		lb := p.looseBodies[i]
		lpos := lb.Position()
		lvel := lb.Velocity()
		l.Position = rl.NewVector2(float32(lpos.X), float32(lpos.Y))
		l.Velocity = rl.NewVector2(float32(lvel.X), float32(lvel.Y))
		l.Rotation = float32(lb.Angle())
	}

	if p.playerBody != nil && p.player != nil {
		ppos := p.playerBody.Position()
		pvel := p.playerBody.Velocity()
		p.player.Position = rl.NewVector2(float32(ppos.X), float32(ppos.Y))
		p.player.Velocity = rl.NewVector2(float32(pvel.X), float32(pvel.Y))
	}

	return projectiles
}

// handleBreakage removes parts of sb whose health reached zero and cuts loose any
// parts thereby stranded from the cockpit; a destroyed cockpit scatters the whole
// ship. Must run outside space.Step — bodies/shapes can't be mutated mid-step.
func (p *Physics) handleBreakage(sb *shipBody) {
	s := sb.ship

	cockpit, hasCockpit := s.Cockpit()

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

	p.recomputeShipBody(sb)
}

// removeShipPart deletes the part at c and removes its collision shape. It leaves
// the ship body's mass untouched; callers rebuild that after a batch of removals.
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

// spawnLoosePart creates a free-floating body for the part at c. The debris
// inherits the velocity of that point on the ship (body velocity plus ω × r) so it
// flies off naturally. The caller still removes the part from the grid.
func (p *Physics) spawnLoosePart(sb *shipBody, c GridCoord) {
	part, ok := sb.ship.Parts[c]
	if !ok {
		return
	}
	worldPos := sb.ship.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)

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
	// Cancel global damping (multiplier 1.0) so debris coasts like the asteroids.
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
		Rotation: sb.ship.Direction,
	})
	p.looseBodies = append(p.looseBodies, body)
}

// destroyShip scatters every remaining part of sb as loose debris and removes its
// body, marking the ship destroyed. The cockpit simply vanishes rather than
// scattering. The caller drops the ship from p.ships.
func (p *Physics) destroyShip(sb *shipBody) {
	s := sb.ship

	for c, part := range s.Parts {
		if part.Type == PartCockpit {
			continue
		}
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
