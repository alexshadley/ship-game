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

	damagePerImpulse = 0.01
	projectileDamage = 5.0
	// playerDamagePerImpulse scales collision impulse into astronaut damage. The
	// spacesuit is fragile (15 hp), so ramming an asteroid or a ship hurts fast.
	playerDamagePerImpulse = 0.05

	// Missile detonation. On impact a missile deals up to missileBlastDamage,
	// falling off linearly to zero at missileBlastRadius. The falloff is measured
	// from the blast center to the nearest point of each part, so a directly hit
	// part (nearest point at the center) takes full damage. The same falloff
	// scales missileBlastImpulse, the knockback that shoves bodies away from the
	// blast. missileInterceptRadius is how close a PDC round must pass to a missile
	// to damage it in flight.
	missileBlastDamage     = 90.0
	missileBlastRadius     = 3 * cellSize
	missileBlastImpulse    = 6000.0
	missileInterceptRadius = 18.0
)

const (
	collisionShip     cp.CollisionType = 1
	collisionAsteroid cp.CollisionType = 2
	// collisionLoose is free-floating debris: it both takes and deals collision
	// damage against ships, asteroids, and other debris (but not the astronaut, who
	// bumps it harmlessly), and is culled once battered to zero health — see the
	// handlers in NewPhysics.
	// collisionPlayer takes impact damage only from enemy ships; its own hull,
	// asteroids, and debris are harmless. It deals none.
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

	// grab is the loose part the spacewalking player is dragging toward their
	// cursor, or nil. The part never leaves the simulation while held — it is towed
	// toward grab.target (a reachable point refreshed each frame) rather than lifted
	// out — so it keeps colliding with and shoving the world the whole time.
	grab *grabbedPart

	player      *Player
	playerBody  *cp.Body
	playerShape *cp.Shape

	// playerShip identifies the human-controlled ship, and godMode points at the
	// main loop's debug flag. When godMode is set, the player's ship takes no
	// damage from collisions or projectiles — a testing aid, wired in main.
	playerShip *Ship
	godMode    *bool
}

// grabDragSpeed caps how fast (world px/s) a grabbed part is towed toward the
// cursor. It's deliberately slow so a part feels heavy — it lags well behind the
// pointer and crawls into place — and so a dragged part meaningfully shoves
// whatever it rams into rather than batting it aside.
const grabDragSpeed = 200

// grabbedPart is the loose part the player is dragging on a spacewalk: its debris
// entry and rigid body (both stable pointers, so they survive the loose slices
// reshuffling as other debris comes and goes) plus the world point it's being
// towed toward this frame. Attaching, releasing, or the part being destroyed all
// clear it.
type grabbedPart struct {
	loose  *LoosePart
	body   *cp.Body
	target cp.Vector
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

	// The solver handles every bounce; these PostSolve handlers only read the
	// impulse back to apply damage to the parts (and astronaut) involved.

	// Ship parts chip against asteroids.
	shipAsteroid := space.NewCollisionHandler(collisionShip, collisionAsteroid)
	shipAsteroid.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		shipShape, _ := arb.Shapes()
		p.damageShipShapePart(shipShape, arb.TotalImpulse().Length())
	}

	// Ramming: both ships' struck parts take the impact.
	shipShip := space.NewCollisionHandler(collisionShip, collisionShip)
	shipShip.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		a, b := arb.Shapes()
		impulse := arb.TotalImpulse().Length()
		p.damageShipShapePart(a, impulse)
		p.damageShipShapePart(b, impulse)
	}

	// Loose debris is a hazard in its own right: it chips ship parts, asteroid
	// impacts chip it, and it grinds against other debris. Each of these damages
	// the debris too, so a battered chunk eventually breaks apart (culled once its
	// health hits zero in cullDeadLooseParts).
	looseAsteroid := space.NewCollisionHandler(collisionLoose, collisionAsteroid)
	looseAsteroid.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		looseShape, _ := arb.Shapes()
		damageShapePart(looseShape, arb.TotalImpulse().Length())
	}
	looseShip := space.NewCollisionHandler(collisionLoose, collisionShip)
	looseShip.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		looseShape, shipShape := arb.Shapes()
		// A part the player is dragging in to attach mustn't chip the hull it's being
		// bolted onto (nor grind itself down) as it's towed into place — the bounce
		// still happens, just no damage.
		if p.grabbedShape(looseShape) {
			return
		}
		impulse := arb.TotalImpulse().Length()
		damageShapePart(looseShape, impulse)
		p.damageShipShapePart(shipShape, impulse)
	}
	looseLoose := space.NewCollisionHandler(collisionLoose, collisionLoose)
	looseLoose.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		a, b := arb.Shapes()
		impulse := arb.TotalImpulse().Length()
		damageShapePart(a, impulse)
		damageShapePart(b, impulse)
	}

	// The astronaut only takes impact damage from hard knocks against enemy ships
	// while out on a spacewalk; bumping its own hull, an asteroid, or loose debris
	// is harmless (see the collisionLoose/collisionPlayer notes). The solver still
	// handles the bounce in every case.
	playerShipHandler := space.NewCollisionHandler(collisionPlayer, collisionShip)
	playerShipHandler.PostSolveFunc = func(arb *cp.Arbiter, _ *cp.Space, _ interface{}) {
		_, shipShape := arb.Shapes()
		sb := p.shipBodyFor(shipShape.Body())
		if sb == nil || sb.ship == p.playerShip {
			return
		}
		p.damagePlayer(arb.TotalImpulse().Length() * playerDamagePerImpulse)
	}

	return p
}

// damagePlayer subtracts amount from the spacewalking astronaut's health, clamped
// at zero. No-op when no one is out on a walk.
func (p *Physics) damagePlayer(amount float64) {
	if p.player == nil {
		return
	}
	p.player.Health -= float32(amount)
	if p.player.Health < 0 {
		p.player.Health = 0
	}
}

func damagePart(part *Part, impulse float64) {
	part.Health -= float32(impulse) * damagePerImpulse
	if part.Health < 0 {
		part.Health = 0
	}
}

// damageShapePart applies collision damage to the part behind shape, if any. Both
// ship parts and loose debris stash their *Part in the shape's UserData, so this
// works for either.
func damageShapePart(shape *cp.Shape, impulse float64) {
	if part, ok := shape.UserData.(*Part); ok {
		damagePart(part, impulse)
	}
}

// damageShipShapePart is damageShapePart for a ship shape, skipping the hit when
// the shape belongs to the player's ship while debug god mode is on.
func (p *Physics) damageShipShapePart(shape *cp.Shape, impulse float64) {
	if p.playerInvincible(p.shipBodyFor(shape.Body())) {
		return
	}
	damageShapePart(shape, impulse)
}

// ResolveProjectiles tests every projectile against every ship part and asteroid,
// consuming any that connect. A round passes harmlessly through the ship that
// fired it (see Projectile.Owner) but strikes everything else, friend or foe.
// Survivors are returned.
func (p *Physics) ResolveProjectiles(projectiles []*Projectile, particles *ParticleSystem) []*Projectile {
	// Point defense: a PDC round that passes close to a hostile missile chips its
	// health and is spent doing so. A missile shot down this way (health driven to
	// zero) detonates on the spot just as it would on impact — intercepting it
	// still sets off its blast, so the point is to catch it before it reaches you.
	// A ship's own rounds never touch its own missiles (they'd otherwise overtake
	// the slow round on launch), so only enemy fire can bring one down.
	consumed := make(map[*Projectile]bool)
	for _, m := range projectiles {
		if m.Kind != projectileMissile {
			continue
		}
		for _, r := range projectiles {
			if r.Kind == projectileMissile || consumed[r] || r.Owner == m.Owner {
				continue
			}
			if dist(r.Position, m.Position) > missileInterceptRadius {
				continue
			}
			consumed[r] = true
			m.Health -= projectileDamage
			if m.Health <= 0 {
				consumed[m] = true
				p.DetonateMissile(m, particles)
				break
			}
		}
	}

	live := projectiles[:0]
	for _, pr := range projectiles {
		if consumed[pr] {
			continue
		}
		if p.projectileHit(pr, particles) {
			continue
		}
		live = append(live, pr)
	}
	return live
}

func (p *Physics) projectileHit(pr *Projectile, particles *ParticleSystem) bool {
	if pr.Kind == projectileMissile {
		return p.missileHit(pr, particles)
	}
	// A spacewalking astronaut can be shot; a hit consumes the round and wounds them.
	if p.player != nil && dist(pr.Position, p.player.Position) <= playerRadius {
		p.damagePlayer(float64(pr.Damage()))
		return true
	}
	for _, sb := range p.ships {
		// A ship's own rounds fly through it without connecting.
		if sb.ship == pr.Owner {
			continue
		}
		if part := sb.ship.partAtWorld(pr.Position); part != nil {
			// The projectile is consumed on contact either way; in god mode the
			// player's ship simply shrugs it off without losing health.
			if !p.playerInvincible(sb) {
				part.Health -= pr.Damage()
				if part.Health < 0 {
					part.Health = 0
				}
			}
			return true
		}
	}
	// Loose debris is destructible too: a hit chips it and, if that finishes it off,
	// removes it on the spot (we're outside space.Step here, so mutating is safe).
	for i, l := range p.looseParts {
		// Un-rotate the hit into the debris's local frame and test its cell box, so a
		// tumbling part is hit where it actually is (mirrors GrabLoosePartAt).
		sin := float32(math.Sin(float64(l.Rotation)))
		cos := float32(math.Cos(float64(l.Rotation)))
		dx := pr.Position.X - l.Position.X
		dy := pr.Position.Y - l.Position.Y
		lx := dx*cos + dy*sin
		ly := -dx*sin + dy*cos
		if math.Abs(float64(lx)) > cellSize/2 || math.Abs(float64(ly)) > cellSize/2 {
			continue
		}
		l.Part.Health -= pr.Damage()
		if l.Part.Health <= 0 {
			p.removeLoosePartAt(i)
		}
		return true
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

// missileHit tests whether a missile has struck anything solid — a ship other
// than the one that fired it, loose debris, an asteroid, or the astronaut — and
// if so detonates it (an area blast; see missileBlast) and consumes it.
func (p *Physics) missileHit(pr *Projectile, particles *ParticleSystem) bool {
	hit := false
	for _, sb := range p.ships {
		if sb.ship == pr.Owner {
			continue
		}
		if sb.ship.partAtWorld(pr.Position) != nil {
			hit = true
			break
		}
	}
	if !hit {
		for _, l := range p.looseParts {
			if looseBlastDist(l, pr.Position) == 0 {
				hit = true
				break
			}
		}
	}
	if !hit {
		for _, a := range p.asteroids {
			dx := pr.Position.X - a.Position.X
			dy := pr.Position.Y - a.Position.Y
			if dx*dx+dy*dy <= a.Size*a.Size {
				hit = true
				break
			}
		}
	}
	if !hit && p.player != nil && dist(pr.Position, p.player.Position) <= playerRadius {
		hit = true
	}
	if !hit {
		return false
	}
	p.DetonateMissile(pr, particles)
	return true
}

// DetonateMissile sets off a missile: an area blast plus its explosion animation,
// centred where the missile is. Every way a missile can be destroyed — a direct
// impact, a PDC interception, or running out of fuel — routes through here so a
// missile always goes off rather than silently vanishing.
func (p *Physics) DetonateMissile(pr *Projectile, particles *ParticleSystem) {
	p.missileBlast(pr.Position)
	particles.SpawnExplosion(pr.Position, missileBlastRadius)
}

// missileBlast applies a missile's detonation at center: every ship part, piece
// of loose debris, and the astronaut within missileBlastRadius takes damage that
// falls off linearly with the distance from the blast center to the object's
// nearest point (so a directly hit part takes full damage), and every affected
// body is shoved away from the blast by an impulse with the same falloff.
// Dead parts and debris are swept up by the usual breakage/cull passes next step.
func (p *Physics) missileBlast(center rl.Vector2) {
	for _, sb := range p.ships {
		if p.playerInvincible(sb) {
			continue
		}
		nearest := float32(math.MaxFloat32)
		for c, part := range sb.ship.Parts {
			d := sb.ship.distToCell(center, c)
			if d < nearest {
				nearest = d
			}
			if d >= missileBlastRadius {
				continue
			}
			part.Health -= missileBlastDamage * (1 - d/missileBlastRadius)
			if part.Health < 0 {
				part.Health = 0
			}
		}
		if nearest < missileBlastRadius {
			applyBlastImpulse(sb.body, center, 1-nearest/missileBlastRadius)
		}
	}

	for i, l := range p.looseParts {
		d := looseBlastDist(l, center)
		if d >= missileBlastRadius {
			continue
		}
		falloff := 1 - d/missileBlastRadius
		l.Part.Health -= missileBlastDamage * falloff
		if l.Part.Health < 0 {
			l.Part.Health = 0
		}
		applyBlastImpulse(p.looseBodies[i], center, falloff)
	}

	if p.player != nil && p.playerBody != nil {
		d := dist(center, p.player.Position) - playerRadius
		if d < 0 {
			d = 0
		}
		if d < missileBlastRadius {
			falloff := 1 - d/missileBlastRadius
			p.damagePlayer(float64(missileBlastDamage * falloff))
			applyBlastImpulse(p.playerBody, center, falloff)
		}
	}
}

// applyBlastImpulse shoves body away from a blast at center, with strength
// missileBlastImpulse scaled by falloff (0..1). The impulse is applied at the
// body's center of gravity so it reads as a clean push rather than a spin.
func applyBlastImpulse(body *cp.Body, center rl.Vector2, falloff float32) {
	if falloff <= 0 {
		return
	}
	cog := body.LocalToWorld(body.CenterOfGravity())
	dx := cog.X - float64(center.X)
	dy := cog.Y - float64(center.Y)
	d := math.Hypot(dx, dy)
	if d < 1e-6 {
		return
	}
	mag := missileBlastImpulse * float64(falloff)
	body.ApplyImpulseAtWorldPoint(cp.Vector{X: dx / d * mag, Y: dy / d * mag}, cog)
}

// looseBlastDist returns the distance from world point p to the nearest point of
// loose debris l's (rotated) cell box, mirroring Ship.distToCell for a single
// free-floating cell. It is zero when p lies inside the box.
func looseBlastDist(l *LoosePart, p rl.Vector2) float32 {
	sin := float32(math.Sin(float64(l.Rotation)))
	cos := float32(math.Cos(float64(l.Rotation)))
	dx := p.X - l.Position.X
	dy := p.Y - l.Position.Y
	lx := dx*cos + dy*sin
	ly := -dx*sin + dy*cos
	half := float32(cellSize) / 2
	ex := clamp(lx, -half, half) - lx
	ey := clamp(ly, -half, half) - ly
	return float32(math.Sqrt(float64(ex*ex + ey*ey)))
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

	// Tow a grabbed part toward the cursor at a capped speed. Overwriting its
	// velocity each step (as we do for the astronaut's drive) keeps the drag
	// responsive while leaving the part a full physics body — it still collides with
	// and pushes whatever it meets. Pin its spin and align it to its facing so it
	// stays readable as a placement.
	if p.grab != nil {
		body := p.grab.body
		pos := body.Position()
		to := cp.Vector{X: p.grab.target.X - pos.X, Y: p.grab.target.Y - pos.Y}
		vel := cp.Vector{}
		if d := to.Length(); d > 1e-3 {
			speed := d / dt
			if speed > grabDragSpeed {
				speed = grabDragSpeed
			}
			vel = to.Mult(speed / d)
		}
		body.SetVelocityVector(vel)
		body.SetAngle(float64(p.grab.loose.Part.Facing.angle()))
		body.SetAngularVelocity(0)
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

		projectiles = append(projectiles, sb.ship.FireWeapons(float32(dt), sb.controls)...)
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
	// Sweep up any debris ground down to zero health by this step's collisions.
	p.cullDeadLooseParts()

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

// spawnLoosePart creates a free-floating body for the part at c and returns it (nil
// if the cell is empty). The debris inherits the velocity of that point on the ship
// (body velocity plus ω × r) so it flies off naturally. The caller still removes the
// part from the grid.
func (p *Physics) spawnLoosePart(sb *shipBody, c GridCoord) (*LoosePart, *cp.Body) {
	part, ok := sb.ship.Parts[c]
	if !ok {
		return nil, nil
	}
	worldPos := sb.ship.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)

	bodyPos := sb.body.Position()
	bodyVel := sb.body.Velocity()
	w := sb.body.AngularVelocity()
	rx := float64(worldPos.X) - bodyPos.X
	ry := float64(worldPos.Y) - bodyPos.Y
	vel := cp.Vector{X: bodyVel.X - w*ry, Y: bodyVel.Y + w*rx}

	return p.addLoosePart(part, worldPos, sb.ship.Direction, vel, w)
}

// addLoosePart creates a free-floating body for part at world position pos with
// the given rotation (radians), linear velocity, and spin, and records it in the
// parallel looseParts/looseBodies slices. It is the low-level constructor shared
// by spawnLoosePart (breakage / prying) and SeedLooseParts. Returns the created
// debris entry and its body so callers can immediately grab it.
func (p *Physics) addLoosePart(part *Part, pos rl.Vector2, rotation float32, vel cp.Vector, spin float64) (*LoosePart, *cp.Body) {
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
	shape.UserData = part

	loose := &LoosePart{
		Part:     part,
		Position: pos,
		Velocity: rl.NewVector2(float32(vel.X), float32(vel.Y)),
		Rotation: rotation,
	}
	p.looseParts = append(p.looseParts, loose)
	p.looseBodies = append(p.looseBodies, body)
	return loose, body
}

// removeLoosePartAt deletes the loose part at index i, removing its body and
// shape from the space and dropping it from the parallel loose slices. Shared by
// GrabLoosePartAt (picked up) and cullDeadLooseParts (smashed apart). If the part
// being removed is the one the player is dragging, the grab is dropped so it can't
// dangle on a freed body.
func (p *Physics) removeLoosePartAt(i int) {
	if p.grab != nil && p.grab.loose == p.looseParts[i] {
		p.grab = nil
	}
	body := p.looseBodies[i]
	body.EachShape(func(s *cp.Shape) { p.space.RemoveShape(s) })
	p.space.RemoveBody(body)
	p.looseParts = append(p.looseParts[:i], p.looseParts[i+1:]...)
	p.looseBodies = append(p.looseBodies[:i], p.looseBodies[i+1:]...)
}

// cullDeadLooseParts removes any debris whose health has run out (shot or ground
// down by repeated impacts). Iterates back-to-front so removals don't shift the
// indices still to be checked. Must run outside space.Step.
func (p *Physics) cullDeadLooseParts() {
	for i := len(p.looseParts) - 1; i >= 0; i-- {
		if p.looseParts[i].Part.Health <= 0 {
			p.removeLoosePartAt(i)
		}
	}
}

// scavengePartTypes are the part types scattered as free debris for the player to
// salvage — every type except the cockpit (a ship has exactly one, at {0,0}).
var scavengePartTypes = []PartType{PartBlock, PartArmor, PartEngine, PartControlThruster, PartPDC, PartMissileLauncher}

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

// GrabLoosePartAt begins dragging the loose part whose cell contains world point
// wp, provided it lies within scavengeReach of the astronaut at astronautPos. The
// part stays in the simulation — it's merely recorded as grabbed and thereafter
// towed toward the cursor by the grab handling in Update. Reports whether a part
// was grabbed.
func (p *Physics) GrabLoosePartAt(wp rl.Vector2, astronautPos rl.Vector2) bool {
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
		// Cursor is over this part; only grab it if it's within arm's reach.
		if dist(astronautPos, l.Position) > scavengeReach {
			return false
		}
		p.grab = &grabbedPart{loose: l, body: p.looseBodies[i], target: cp.Vector{X: float64(l.Position.X), Y: float64(l.Position.Y)}}
		return true
	}
	return false
}

// GrabbedPart returns the part the player is currently dragging, or nil.
func (p *Physics) GrabbedPart() *Part {
	if p.grab == nil {
		return nil
	}
	return p.grab.loose.Part
}

// GrabbedPos returns the world center of the part the player is dragging, and
// whether one is grabbed — the far anchor for the tractor beam Draw renders.
func (p *Physics) GrabbedPos() (rl.Vector2, bool) {
	if p.grab == nil {
		return rl.Vector2{}, false
	}
	return p.grab.loose.Position, true
}

// grabbedShape reports whether shape belongs to the part the player is currently
// dragging, so collision handlers can spare it (and its target) from damage.
func (p *Physics) grabbedShape(shape *cp.Shape) bool {
	return p.grab != nil && shape.Body() == p.grab.body
}

// UpdateGrab aims the current drag at the cursor for this frame, clamping the pull
// target to within scavengeReach of the astronaut so a grabbed part can't be towed
// out past the tool's reach. No-op when nothing is grabbed.
func (p *Physics) UpdateGrab(cursor rl.Vector2, astronautPos rl.Vector2) {
	if p.grab == nil {
		return
	}
	target := cursor
	if d := dist(astronautPos, cursor); d > scavengeReach && d > 0 {
		s := scavengeReach / d
		target = rl.NewVector2(
			astronautPos.X+(cursor.X-astronautPos.X)*s,
			astronautPos.Y+(cursor.Y-astronautPos.Y)*s,
		)
	}
	p.grab.target = cp.Vector{X: float64(target.X), Y: float64(target.Y)}
}

// ReleaseGrab lets go of the dragged part, leaving it loose in space with whatever
// velocity it carried. No-op when nothing is grabbed.
func (p *Physics) ReleaseGrab() {
	p.grab = nil
}

// AttachGrabbed pulls the grabbed part out of the loose field and bolts it onto
// ship at cell c, then clears the grab. The caller is responsible for having
// checked c is a valid attachment (empty and adjacent to the hull). No-op when
// nothing is grabbed.
func (p *Physics) AttachGrabbed(ship *Ship, c GridCoord) {
	if p.grab == nil {
		return
	}
	part := p.grab.loose.Part
	if i := p.looseIndexOf(p.grab.loose); i >= 0 {
		p.removeLoosePartAt(i) // also clears p.grab
	}
	p.grab = nil
	p.AttachPart(ship, c, part)
}

// looseIndexOf returns the index of l in the loose slices, or -1 if it's gone.
func (p *Physics) looseIndexOf(l *LoosePart) int {
	for i, x := range p.looseParts {
		if x == l {
			return i
		}
	}
	return -1
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

// PryTargetAt returns the ship and cell of a pryable part under world point wp,
// ok=false if nothing pryable is there. A part is pryable only if it's not the
// cockpit and it's on the hull's exterior (has an open side) — interior parts are
// walled in and can't be reached. Every simulated ship is a candidate — the
// player's own and any enemy — so a spacewalking player can strip parts off enemy
// hulls, not just their own. When ships overlap, the first in p.ships wins; that's
// rare enough not to matter for a dwell interaction.
func (p *Physics) PryTargetAt(wp rl.Vector2) (*Ship, GridCoord, bool) {
	for _, sb := range p.ships {
		c := sb.ship.gridAtWorld(wp)
		if part, ok := sb.ship.Parts[c]; ok && part.Type != PartCockpit && sb.ship.isExterior(c) {
			return sb.ship, c, true
		}
	}
	return nil, GridCoord{}, false
}

// DetachAndGrab pries the part at grid cell c off ship, drops it into space as
// debris, and immediately grabs it so the player can drag it away. It reports
// whether a part was pried (false if the cell is empty, holds the cockpit, or the
// ship isn't simulated here). Because the pried part may have been the only link
// between the cockpit and others, any parts thereby stranded are cut loose as
// debris too. It is the inverse of AttachGrabbed, and mirrors the stranding logic
// of handleBreakage.
func (p *Physics) DetachAndGrab(ship *Ship, c GridCoord) bool {
	for _, sb := range p.ships {
		if sb.ship != ship {
			continue
		}
		part, ok := sb.ship.Parts[c]
		if !ok || part.Type == PartCockpit {
			return false
		}
		// Spawn the pried part as debris where it sat and grab it, then unbolt it.
		loose, body := p.spawnLoosePart(sb, c)
		p.removeShipPart(sb, c)
		p.grab = &grabbedPart{loose: loose, body: body, target: body.Position()}

		if cockpit, hasCockpit := sb.ship.Cockpit(); hasCockpit {
			connected := sb.ship.connectedParts(cockpit)
			var stranded []GridCoord
			for gc := range sb.ship.Parts {
				if !connected[gc] {
					stranded = append(stranded, gc)
				}
			}
			for _, gc := range stranded {
				p.spawnLoosePart(sb, gc)
				p.removeShipPart(sb, gc)
			}
		}

		p.recomputeShipBody(sb)
		return true
	}
	return false
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
