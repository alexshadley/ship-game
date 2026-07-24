package main

import (
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/jakecoffman/cp"
)

const (
	engineThrust   = 3000.0
	thrusterTorque = 90000.0
	// engineStraightTolerance is how far (in world units) the combined engine
	// thrust line may pass from the ship's center of mass and still count as
	// "straight". Within this slack the net torque is dropped so a nearly
	// symmetric engine layout drives forward instead of drifting into a slow
	// spin; beyond it (e.g. a lone off-center engine) the rotation is kept.
	engineStraightTolerance = cellSize * 0.2
	// spaceDamping is the fraction of velocity that survives each second; it applies
	// to every body, so anything unpowered coasts to a stop.
	spaceDamping = 0.55

	shipElasticity     = 0.4
	asteroidElasticity = 0.9

	asteroidDensity = 0.02

	damagePerImpulse = 0.05
	projectileDamage = 5.0
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

	engines   int
	thrusters int
	controls  Controls
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

	// playerShip identifies the human-controlled ship, and godMode points at the
	// main loop's debug flag. When godMode is set, the player's ship takes no
	// damage from collisions or projectiles — a testing aid, wired in main.
	playerShip *Ship
	godMode    *bool
}

// playerInvincible reports whether damage to sb should be suppressed because it
// is the player's ship and debug god mode is on.
func (p *Physics) playerInvincible(sb *shipBody) bool {
	return p.godMode != nil && *p.godMode && sb != nil && sb.ship == p.playerShip
}

// shipBodyFor returns the shipBody whose rigid body is body, or nil if none
// (e.g. an asteroid or loose-part body).
func (p *Physics) shipBodyFor(body *cp.Body) *shipBody {
	for _, sb := range p.ships {
		if sb.body == body {
			return sb
		}
	}
	return nil
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
		if p.playerInvincible(p.shipBodyFor(shipShape.Body())) {
			return
		}
		if part, ok := shipShape.UserData.(*Part); ok {
			damagePart(part, arb.TotalImpulse().Length())
		}
	}

	shipHandler := space.NewCollisionHandler(collisionShip, collisionShip)
	shipHandler.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		a, b := arb.Shapes()
		impulse := arb.TotalImpulse().Length()
		if part, ok := a.UserData.(*Part); ok && !p.playerInvincible(p.shipBodyFor(a.Body())) {
			damagePart(part, impulse)
		}
		if part, ok := b.UserData.(*Part); ok && !p.playerInvincible(p.shipBodyFor(b.Body())) {
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
			// The projectile is consumed on contact either way; in god mode the
			// player's ship simply shrugs it off without losing health.
			if !p.playerInvincible(sb) {
				part.Health -= projectileDamage
				if part.Health < 0 {
					part.Health = 0
				}
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

// addShipShape builds the collision box for part at grid cell c on body, giving it
// the part's weight as mass so the body's center of gravity, mass, and moment can
// be derived from its shapes (see AccumulateMassFromShapes). Shared by AddShip and
// AttachPart.
func (p *Physics) addShipShape(body *cp.Body, c GridCoord, part *Part) *cp.Shape {
	center := cp.Vector{X: float64(c.X) * cellSize, Y: float64(c.Y) * cellSize}
	shape := p.space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(center, cellSize/2, cellSize/2), 0))
	shape.SetCollisionType(collisionShip)
	shape.SetElasticity(shipElasticity)
	shape.SetFriction(0.4)
	shape.SetMass(float64(part.Weight))
	shape.UserData = part
	return shape
}

// AddShip builds a rigid body for ship and registers controller as the source of
// its Controls. Mass lives on the per-part shapes, so AccumulateMassFromShapes
// sets the body's center of gravity to the weight-weighted centroid — the sim
// rotates the ship about its true balance point, matching the HUD marker. The
// body's local origin stays the cockpit cell, so the renderer maps straight across.
func (p *Physics) AddShip(ship *Ship, controller Controller) {
	var engines, thrusters int
	for _, part := range ship.Parts {
		switch part.Type {
		case PartEngine:
			engines++
		case PartControlThruster:
			thrusters++
		}
	}

	body := cp.NewBody(1, 1) // mass, moment, and cog are derived from the shapes below
	body.SetAngle(float64(ship.Direction))
	body.SetPosition(cp.Vector{X: float64(ship.Position.X), Y: float64(ship.Position.Y)})
	p.space.AddBody(body)

	shipShapes := make(map[*Part]*cp.Shape, len(ship.Parts))
	for c, part := range ship.Parts {
		shipShapes[part] = p.addShipShape(body, c, part)
	}
	body.AccumulateMassFromShapes()

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

		// Each engine pushes from its own cell along the axis opposite its facing, so
		// an off-center engine layout yields a net torque and the ship rotates. Force
		// and torque accumulate here (both are zeroed by the integrator each step) and
		// the turn torque is added on top of whatever the engines contributed.
		if controls.Thrust != 0 {
			// The net effect of the engines is fully described by their combined force
			// and the torque it exerts about the center of gravity. engineForceTorqueAbout
			// (shared with the HUD thrust-line overlay) computes both about the body's
			// cog — the true centroid — in engineThrust units; scale by the throttle.
			cog := sb.body.CenterOfGravity()
			force, torque := engineForceTorqueAbout(sb.ship.Parts, nil, GridCoord{}, rl.NewVector2(float32(cog.X), float32(cog.Y)))
			netForce := cp.Vector{X: float64(force.X) * float64(controls.Thrust), Y: float64(force.Y) * float64(controls.Thrust)}
			netTorque := float64(torque) * float64(controls.Thrust)
			// If the combined thrust line passes close enough to the center of mass,
			// treat it as straight and drop the residual torque so a nearly symmetric
			// layout doesn't slowly spin. A near-zero net force means the engines
			// cancel out translationally and any torque is a deliberate couple, so
			// leave it alone.
			if f := netForce.Length(); f > 0 && math.Abs(netTorque)/f <= engineStraightTolerance {
				netTorque = 0
			}
			// Apply the net force at the center of mass (zero torque contribution),
			// then add the net torque on top.
			sb.body.ApplyForceAtLocalPoint(netForce, cog)
			sb.body.SetTorque(sb.body.Torque() + netTorque)
		}
		sb.body.SetTorque(sb.body.Torque() + thrusterTorque*float64(sb.thrusters)*float64(controls.Turn))
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
		// Position() reports the world location of the body's local origin (the
		// cockpit cell), independent of where the center of gravity sits, so the
		// renderer's cockpit-origin frame maps straight across.
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

		projectiles = append(projectiles, sb.ship.FireCannons(float32(dt), sb.controls.Fire)...)
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

	p.addLoosePart(part, worldPos, sb.ship.Direction, vel, w)
}

// addLoosePart creates a free-floating body for part at world position pos with
// the given rotation (radians), linear velocity, and spin, and records it in the
// parallel looseParts/looseBodies slices. It is the low-level constructor shared
// by spawnLoosePart (breakage) and DropPart (a scavenged part released back into
// the field).
func (p *Physics) addLoosePart(part *Part, pos rl.Vector2, rotation float32, vel cp.Vector, spin float64) {
	m := float64(part.Weight)
	body := cp.NewBody(m, cp.MomentForBox(m, cellSize, cellSize))
	body.SetPosition(cp.Vector{X: float64(pos.X), Y: float64(pos.Y)})
	body.SetAngle(float64(rotation))
	body.SetVelocityVector(vel)
	body.SetAngularVelocity(spin)
	p.space.AddBody(body)

	shape := p.space.AddShape(cp.NewBox2(body, cp.NewBBForExtents(cp.Vector{}, cellSize/2, cellSize/2), 0))
	shape.SetCollisionType(collisionLoose)
	shape.SetElasticity(shipElasticity)
	shape.SetFriction(0.4)

	p.looseParts = append(p.looseParts, &LoosePart{
		Part:     part,
		Position: pos,
		Velocity: rl.NewVector2(float32(vel.X), float32(vel.Y)),
		Rotation: rotation,
	})
	p.looseBodies = append(p.looseBodies, body)
}

// scavengePartTypes are the part types scattered as free debris for the player to
// salvage — every type except the cockpit (a ship has exactly one, at {0,0}).
var scavengePartTypes = []PartType{PartBlock, PartEngine, PartControlThruster, PartCannon}

// Loose-part scatter tuning: n stationary parts drift in a ring around the world
// origin, close enough to reach on a spacewalk but clear of the ship's spawn.
const (
	loosePartMinRadius = 250
	loosePartMaxRadius = 1200
)

// SeedLooseParts scatters n random salvageable parts (assorted types and facings)
// at rest in a ring around the origin, so there's debris to scavenge from the
// start. A testing/onboarding aid, invoked once at startup.
func (p *Physics) SeedLooseParts(n int) {
	for i := 0; i < n; i++ {
		angle := rand.Float64() * 2 * math.Pi
		r := loosePartMinRadius + rand.Float64()*(loosePartMaxRadius-loosePartMinRadius)
		pos := rl.NewVector2(float32(math.Cos(angle)*r), float32(math.Sin(angle)*r))
		part := NewPart(scavengePartTypes[rand.Intn(len(scavengePartTypes))], Facing(rand.Intn(4)))
		p.addLoosePart(part, pos, rand.Float32()*2*math.Pi, cp.Vector{}, 0)
	}
}

// GrabLoosePart removes and returns the loose part whose cell contains world
// point wp (nil if none), so the spacewalking player can pick it up and drag it.
// It deletes the part's body and shape from the space and drops it from the
// parallel loose slices; the caller now owns the returned *Part.
func (p *Physics) GrabLoosePart(wp rl.Vector2) *Part {
	for i, l := range p.looseParts {
		// Transform wp into the part's local (un-rotated) frame and test it against
		// the part's cellSize box, so grabbing respects the part's tumble.
		sin := float32(math.Sin(float64(l.Rotation)))
		cos := float32(math.Cos(float64(l.Rotation)))
		dx := wp.X - l.Position.X
		dy := wp.Y - l.Position.Y
		lx := dx*cos + dy*sin
		ly := -dx*sin + dy*cos
		if math.Abs(float64(lx)) > cellSize/2 || math.Abs(float64(ly)) > cellSize/2 {
			continue
		}

		body := p.looseBodies[i]
		body.EachShape(func(s *cp.Shape) { p.space.RemoveShape(s) })
		p.space.RemoveBody(body)
		p.looseParts = append(p.looseParts[:i], p.looseParts[i+1:]...)
		p.looseBodies = append(p.looseBodies[:i], p.looseBodies[i+1:]...)
		return l.Part
	}
	return nil
}

// DropPart releases a held part back into the loose-part field at world position
// pos with rotation (radians), moving with velocity vel (typically the
// astronaut's, so it stays within reach). Used when a drag ends somewhere the
// part can't attach.
func (p *Physics) DropPart(part *Part, pos rl.Vector2, rotation float32, vel rl.Vector2) {
	p.addLoosePart(part, pos, rotation, cp.Vector{X: float64(vel.X), Y: float64(vel.Y)}, 0)
}

// AttachPart adds part to ship at grid cell c, building its collision shape on
// the ship's body and recomputing the body's mass/moment/engine counts so the
// enlarged ship flies correctly. It is the inverse of removeShipPart and mirrors
// the per-part shape setup in AddShip. No-op if ship isn't simulated here.
func (p *Physics) AttachPart(ship *Ship, c GridCoord, part *Part) {
	for _, sb := range p.ships {
		if sb.ship != ship {
			continue
		}
		sb.ship.Parts[c] = part
		sb.shipShapes[part] = p.addShipShape(sb.body, c, part)
		p.recomputeShipBody(sb)
		return
	}
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

// recomputeShipBody rebuilds the body's mass, moment, and center of gravity from
// its current shapes after parts are added or lost, and refreshes the cached
// engine/thruster counts. AccumulateMassFromShapes re-derives the centroid and
// keeps the cockpit-origin cell pinned in the world, so the hull doesn't jump.
func (p *Physics) recomputeShipBody(sb *shipBody) {
	var engines, thrusters int
	for _, part := range sb.ship.Parts {
		switch part.Type {
		case PartEngine:
			engines++
		case PartControlThruster:
			thrusters++
		}
	}
	if len(sb.shipShapes) > 0 {
		sb.body.AccumulateMassFromShapes()
	}
	sb.engines = engines
	sb.thrusters = thrusters
}
