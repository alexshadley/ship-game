package main

import (
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	pdcMuzzleSpeed  = 1200.0
	pdcFireInterval = 0.09
	// slowPDCFireInterval is the junk PDC's cadence: a third the fire rate of a
	// standard PDC.
	slowPDCFireInterval = pdcFireInterval * 3

	// pdcHalfArc is how far (radians) a PDC can slew its aim to either side of
	// its mount facing — a total arc a bit under a half circle. A mount whose
	// fire target falls outside the arc holds its fire.
	pdcHalfArc = 0.44 * math.Pi

	// pdcSpread scatters each PDC round: its fired heading is jittered by up to
	// pdcSpread radians to either side of the aim so a stream of fire fans out
	// into a cone rather than a laser-straight line.
	pdcSpread = 0.03 * math.Pi

	// PDC rounds are short-ranged. Drag bleeds their speed off exponentially
	// (pdcProjectileDrag per second) and they despawn after
	// pdcProjectileLifespan; together these cap a round's reach at roughly
	// muzzleSpeed/drag ≈ 900 world px, with rounds visibly petering out first.
	pdcProjectileDrag     = 1.2
	pdcProjectileLifespan = 1.8

	// pdcEngagementRange is how close (world px) the fire target must be before a
	// PDC mount opens up when Controls.EnforceEngagementRange is set. It sits at
	// the practical reach of a PDC round so the AI only fires shots that arrive.
	pdcEngagementRange = 900

	// Missiles are a heavy, slow-firing weapon fired from a PartMissileLauncher.
	// A launcher spits one roughly every missileFireInterval seconds and only
	// within a tight missileHalfArc of its mount. The round leaves the tube slowly
	// (missileLaunchSpeed) and its motor then builds speed at missileAcceleration
	// up to a missileCruiseSpeed cruise. Unlike a PDC round a missile has health
	// (missileHealth) and can be shot down in flight; on impact it detonates for
	// area damage (see Physics.missileBlast).
	missileFireInterval = 2.2
	missileHalfArc      = 0.22 * math.Pi
	missileLaunchSpeed  = 120.0
	missileCruiseSpeed  = 480.0
	missileAcceleration = 320.0
	missileLifespan     = 7.0
	missileHealth       = 15.0

	// missileEngagementRange is the fire-target distance (world px) within which a
	// missile launcher opens up when Controls.EnforceEngagementRange is set. A
	// missile self-propels to a cruise speed, so it reaches about twice as far as a
	// PDC and launchers open fire at a correspondingly longer range.
	missileEngagementRange = 1800

	// A railgun is a heavy hitscan weapon (see PartRailgun). It fires within a very
	// narrow arc on a slow reload and strikes instantly along a straight line — no
	// projectile — dealing railgunDamage to the first thing the beam meets within
	// railgunRange. Firing kicks the shooter back a little (railgunRecoilImpulse)
	// and shoves the struck body a lot more (railgunImpactImpulse); the two impulses
	// deliberately don't balance. The white beam lingers railgunBeamDuration seconds
	// so an instantaneous shot still reads on screen.
	railgunDamage          = 600.0
	railgunFireInterval    = 3.5
	railgunHalfArc         = 0.07 * math.Pi
	railgunEngagementRange = 2700
	railgunRange           = 4000.0
	railgunRecoilImpulse   = 4000.0
	railgunImpactImpulse   = 22000.0
	railgunBeamDuration    = 0.12
	// railgunWarmup is how long (seconds) a railgun mount spends charging before it
	// looses its shot. While it charges its aim is locked and two red telegraph
	// lines, parallel to the shot, slide together from either side; the shot fires
	// the instant they touch on the beam line.
	railgunWarmup = 1.0
	// railgunTelegraphSpread is how far to each side of the locked beam the two
	// warm-up lines start (world px) before sliding in to meet it.
	railgunTelegraphSpread = cellSize * 1.5
)

var (
	projectileSize = rl.NewVector2(4, 12)
	missileSize    = rl.NewVector2(12, 34)

	// A missile reads as a grey body with a red nose cap pointed the way it flies.
	missileBodyColor = rl.NewColor(150, 150, 158, 255)
	missileTipColor  = rl.Red
)

// ProjectileKind distinguishes an ordinary PDC round from a missile, which
// accelerates to a cruise speed, carries health, and detonates for area damage.
type ProjectileKind int

const (
	projectilePDC ProjectileKind = iota
	projectileMissile
)

// drawFireCrosshair marks where the player's PDCs are aiming — a gapped tactical
// reticle at the cursor's world position, so the mouse reads as a weapon sight
// now that holding LMB fires. Drawn in world space to line up exactly with the
// FireTarget the PDCs slew toward.
func drawFireCrosshair(camera rl.Camera2D) {
	c := mouseWorld(camera)
	const (
		r   = cellSize * 0.35 // ring radius
		gap = cellSize * 0.14 // gap between ring and tick marks
		arm = cellSize * 0.22 // length of each tick mark
	)
	col := rl.NewColor(255, 90, 70, 220)
	rl.DrawCircleLines(int32(c.X), int32(c.Y), r, col)
	// Four tick marks pointing inward, leaving the center open.
	rl.DrawLineEx(rl.NewVector2(c.X-r-gap-arm, c.Y), rl.NewVector2(c.X-r-gap, c.Y), 1, col)
	rl.DrawLineEx(rl.NewVector2(c.X+r+gap, c.Y), rl.NewVector2(c.X+r+gap+arm, c.Y), 1, col)
	rl.DrawLineEx(rl.NewVector2(c.X, c.Y-r-gap-arm), rl.NewVector2(c.X, c.Y-r-gap), 1, col)
	rl.DrawLineEx(rl.NewVector2(c.X, c.Y+r+gap), rl.NewVector2(c.X, c.Y+r+gap+arm), 1, col)
	// A center dot pinpoints the exact aim.
	rl.DrawCircle(int32(c.X), int32(c.Y), 1, col)
}

type Projectile struct {
	Position rl.Vector2
	Velocity rl.Vector2
	Lifespan float32
	Rotation float32
	Size     rl.Vector2
	// Owner is the ship that fired the round; rounds pass harmlessly through
	// their own ship but strike everything else.
	Owner *Ship
	Kind  ProjectileKind
	// Health is only meaningful for missiles: a missile shot down in flight (its
	// health driven to zero by an enemy's rounds) is destroyed without detonating.
	Health     float32
	BaseDamage float32
}

func NewProjectile(owner *Ship, pos, velocity rl.Vector2, rotation, damage float32) *Projectile {
	return &Projectile{
		Position:   pos,
		Velocity:   velocity,
		Lifespan:   pdcProjectileLifespan,
		Rotation:   rotation,
		Size:       projectileSize,
		Owner:      owner,
		Kind:       projectilePDC,
		BaseDamage: damage,
	}
}

// NewMissile builds a missile launched from owner: it starts slow (velocity is
// the launch velocity along the aim) and accelerates toward a cruise speed in
// Update, and it carries health so it can be shot down before impact.
func NewMissile(owner *Ship, pos, velocity rl.Vector2, rotation, blastDamage float32) *Projectile {
	return &Projectile{
		Position:   pos,
		Velocity:   velocity,
		Lifespan:   missileLifespan,
		Rotation:   rotation,
		Size:       missileSize,
		Owner:      owner,
		Kind:       projectileMissile,
		Health:     missileHealth,
		BaseDamage: blastDamage,
	}
}

func (p *Projectile) Update(dt float32) {
	if p.Kind == projectileMissile {
		// The motor builds speed along the current heading up to the cruise cap;
		// scaling the existing velocity keeps the direction fixed (missiles fly
		// straight along their launch aim).
		speed := float32(math.Hypot(float64(p.Velocity.X), float64(p.Velocity.Y)))
		if speed > 0 {
			newSpeed := speed + missileAcceleration*dt
			if newSpeed > missileCruiseSpeed {
				newSpeed = missileCruiseSpeed
			}
			scale := newSpeed / speed
			p.Velocity.X *= scale
			p.Velocity.Y *= scale
		}
		p.Position.X += p.Velocity.X * dt
		p.Position.Y += p.Velocity.Y * dt
		p.Lifespan -= dt
		return
	}

	p.Position.X += p.Velocity.X * dt
	p.Position.Y += p.Velocity.Y * dt
	decay := float32(math.Exp(float64(-pdcProjectileDrag * dt)))
	p.Velocity.X *= decay
	p.Velocity.Y *= decay
	p.Lifespan -= dt
}

func (p *Projectile) Expired() bool {
	return p.Lifespan <= 0
}

// Damage is the hit damage this round deals, scaled linearly by its speed: a
// round leaving the muzzle deals its full BaseDamage, and drag bleeds that off
// in step with its speed until the round peters out. A round can't deal more than
// full damage, so a fast-moving shooter's rounds still cap at BaseDamage.
func (p *Projectile) Damage() float32 {
	speed := math.Hypot(float64(p.Velocity.X), float64(p.Velocity.Y))
	frac := speed / pdcMuzzleSpeed
	if frac > 1 {
		frac = 1
	}
	return p.BaseDamage * float32(frac)
}

func (p *Projectile) Draw() {
	if p.Kind == projectileMissile {
		p.drawMissile()
		return
	}
	rec := rl.NewRectangle(p.Position.X, p.Position.Y, p.Size.X, p.Size.Y)
	origin := rl.NewVector2(p.Size.X/2, p.Size.Y/2)
	rl.DrawRectanglePro(rec, origin, p.Rotation*180/math.Pi, rl.Yellow)
}

// drawMissile renders the missile as a grey body with a short red nose cap on
// the leading end (the end pointing along its heading).
func (p *Projectile) drawMissile() {
	deg := p.Rotation * 180 / math.Pi

	body := rl.NewRectangle(p.Position.X, p.Position.Y, p.Size.X, p.Size.Y)
	rl.DrawRectanglePro(body, rl.NewVector2(p.Size.X/2, p.Size.Y/2), deg, missileBodyColor)

	// The nose points along the heading (sin, -cos); cap the front ~quarter red.
	fx, fy := p.forward()
	tipLen := p.Size.Y * 0.28
	tipCenter := rl.NewVector2(
		p.Position.X+fx*(p.Size.Y/2-tipLen/2),
		p.Position.Y+fy*(p.Size.Y/2-tipLen/2),
	)
	tip := rl.NewRectangle(tipCenter.X, tipCenter.Y, p.Size.X, tipLen)
	rl.DrawRectanglePro(tip, rl.NewVector2(p.Size.X/2, tipLen/2), deg, missileTipColor)
}

// forward is the unit vector pointing along the missile's heading.
func (p *Projectile) forward() (float32, float32) {
	return float32(math.Sin(float64(p.Rotation))), -float32(math.Cos(float64(p.Rotation)))
}

// EmitExhaust spawns a small exhaust plume out the missile's tail, drifting with
// the missile the way an engine plume drifts with its ship.
func (p *Projectile) EmitExhaust(ps *ParticleSystem) {
	fx, fy := p.forward()
	pos := rl.NewVector2(p.Position.X-fx*p.Size.Y/2, p.Position.Y-fy*p.Size.Y/2)
	dir := rl.NewVector2(-fx, -fy)
	ps.EmitMissile(pos, dir, p.Velocity)
}

// RailgunShot is one hitscan shot a railgun mount loosed this frame: a straight
// beam from Origin along the unit vector Dir, resolved instantly against the world
// (see Physics.fireRailgun). Owner is the ship that fired it, so the beam passes
// through its own hull.
type RailgunShot struct {
	Origin rl.Vector2
	Dir    rl.Vector2
	Owner  *Ship
	Damage float32
}

// RailgunCharge is the telegraph for a railgun mount still warming up this frame:
// the locked shot runs from Origin along the unit vector Dir for railgunRange world
// px, and Progress (0..1) is how far along the railgunWarmup the mount is. It is
// drawn as two red lines parallel to Dir, one on each side, sliding together until
// they touch on the beam line exactly when the shot fires (see DrawBeams).
type RailgunCharge struct {
	Origin   rl.Vector2
	Dir      rl.Vector2
	Progress float32
}

// isWeapon reports whether a part is a firing mount handled by FireWeapons.
func (t PartType) isWeapon() bool {
	return t == PartPDC || t == PartSlowPDC || t == PartMissileLauncher || t == PartRailgun
}

func (t PartType) baseDamage() float32 {
	switch t {
	case PartMissileLauncher:
		return missileBlastDamage
	case PartRailgun:
		return railgunDamage
	default:
		return projectileDamage
	}
}

// fireInterval is the cadence between this mount's shots: a slow junk PDC fires
// at a third the rate of a standard PDC, and a missile launcher fires far more
// slowly still. Each mount keeps its own cadence, so one slow mount never drags
// down the ship's others.
func (t PartType) fireInterval() float32 {
	switch t {
	case PartSlowPDC:
		return slowPDCFireInterval
	case PartMissileLauncher:
		return missileFireInterval
	case PartRailgun:
		return railgunFireInterval
	default:
		return pdcFireInterval
	}
}

// halfArc is how far a mount may slew its aim to either side of its mount facing.
// A missile launcher has a much tighter arc than a PDC.
func (t PartType) halfArc() float32 {
	switch t {
	case PartMissileLauncher:
		return missileHalfArc
	case PartRailgun:
		return railgunHalfArc
	default:
		return pdcHalfArc
	}
}

// engagementRange is how close (world px) the fire target must be for this mount
// to open fire when Controls.EnforceEngagementRange is set. A missile launcher
// reaches about twice as far as a PDC. The player fires without this gate.
func (t PartType) engagementRange() float32 {
	switch t {
	case PartMissileLauncher:
		return missileEngagementRange
	case PartRailgun:
		return railgunEngagementRange
	default:
		return pdcEngagementRange
	}
}

// FireWeapons advances each weapon mount's independent cooldown and returns the
// rounds from mounts ready to fire while the trigger is held. Each mount aims
// itself at the controls' fire target (a world-frame offset from the ship
// origin) as long as that target lies within the mount's arc; a mount whose
// target is outside its arc holds fire until the target swings back in. PDCs
// spit fast, short-ranged rounds; missile launchers loose a slow, accelerating,
// destructible missile.
func (s *Ship) FireWeapons(dt float32, controls Controls) ([]*Projectile, []RailgunShot, []RailgunCharge) {
	var shots []*Projectile
	var rails []RailgunShot
	var charges []RailgunCharge
	for c, part := range s.Parts {
		if !part.Type.isWeapon() {
			continue
		}

		part.FireCooldown -= dt
		// PDCs fire on the main trigger and aim at PDCTarget; missile launchers and
		// railguns share their own trigger and aim at MissileTarget, so the two
		// weapon groups fire independently and can point at different things (an
		// enemy PDC swats an inbound missile while its launcher stays on the target).
		triggered := controls.Fire
		fireTarget := controls.PDCTarget
		if part.Type == PartMissileLauncher || part.Type == PartRailgun {
			triggered = controls.FireMissiles
			fireTarget = controls.MissileTarget
		}
		if !triggered || part.FireCooldown > 0 {
			// A railgun that loses its trigger (or is still on cooldown) abandons any
			// warm-up in progress and starts over next time it lines up.
			part.RailgunCharge = 0
			continue
		}

		// A railgun already warming up has locked its aim: it commits to the
		// direction captured when the charge began and ignores any new target until
		// it fires, so the shot can't be re-aimed mid-charge. The muzzle still
		// follows the mount cell as the ship moves.
		if part.Type == PartRailgun && part.RailgunCharge > 0 {
			part.RailgunCharge += dt
			center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
			// The bearing is locked relative to the ship, so re-derive the world
			// direction from the hull's current facing — a spin swings the shot.
			worldAim := s.Direction + part.RailgunAim
			dir := rl.NewVector2(float32(math.Sin(float64(worldAim))), float32(-math.Cos(float64(worldAim))))
			pos := rl.NewVector2(center.X+dir.X*cellSize*0.5, center.Y+dir.Y*cellSize*0.5)
			if part.RailgunCharge < railgunWarmup {
				charges = append(charges, RailgunCharge{
					Origin:   pos,
					Dir:      dir,
					Progress: part.RailgunCharge / railgunWarmup,
				})
				continue
			}
			part.RailgunCharge = 0
			part.FireCooldown = part.weaponFireInterval()
			rails = append(rails, RailgunShot{Origin: pos, Dir: dir, Owner: s, Damage: part.weaponDamage()})
			continue
		}

		target := rl.NewVector2(
			s.Position.X+fireTarget.X,
			s.Position.Y+fireTarget.Y,
		)

		// Aim from this mount's own cell so converging fire actually converges
		// on the target point rather than running parallel.
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		dx := target.X - center.X
		dy := target.Y - center.Y
		dist := float32(math.Hypot(float64(dx), float64(dy)))
		if dist == 0 {
			part.RailgunCharge = 0
			continue
		}
		// The AI holds its trigger continuously; each mount only opens up once the
		// target is within its own weapon's engagement range. The player fires
		// without this gate.
		if controls.EnforceEngagementRange && dist > part.Type.engagementRange() {
			part.RailgunCharge = 0
			continue
		}
		aim := heading(dx, dy)
		mount := s.Direction + part.Facing.angle()
		if math.Abs(float64(angleDiff(aim, mount))) > float64(part.Type.halfArc()) {
			part.RailgunCharge = 0
			continue
		}

		dirX, dirY := dx/dist, dy/dist
		pos := rl.NewVector2(center.X+dirX*cellSize*0.5, center.Y+dirY*cellSize*0.5)
		if part.Type == PartRailgun {
			// A railgun doesn't fire the instant it's ready: it locks its aim on the
			// current target and warms up for railgunWarmup, telegraphing the shot. The
			// locked aim is then honored by the "already warming up" block above until
			// the beam looses; the shot itself fires from there.
			part.RailgunAim = aim - s.Direction
			part.RailgunCharge += dt
			charges = append(charges, RailgunCharge{
				Origin:   pos,
				Dir:      rl.NewVector2(dirX, dirY),
				Progress: part.RailgunCharge / railgunWarmup,
			})
			continue
		}
		part.FireCooldown = part.weaponFireInterval()
		if part.Type == PartMissileLauncher {
			// A missile is self-propelled: it leaves the tube slowly along the aim
			// (not inheriting the ship's velocity) and accelerates from there.
			vel := rl.NewVector2(dirX*missileLaunchSpeed, dirY*missileLaunchSpeed)
			shots = append(shots, NewMissile(s, pos, vel, aim, part.weaponDamage()))
			continue
		}
		// Scatter each round within pdcSpread of the aim so sustained fire fans
		// into a cone. The arc check above used the true aim; only the fired
		// round is jittered.
		fired := aim + (rand.Float32()*2-1)*pdcSpread
		fdx, fdy := float32(math.Sin(float64(fired))), float32(-math.Cos(float64(fired)))
		vel := rl.NewVector2(
			s.Velocity.X+fdx*pdcMuzzleSpeed,
			s.Velocity.Y+fdy*pdcMuzzleSpeed,
		)
		shots = append(shots, NewProjectile(s, pos, vel, fired, part.weaponDamage()))
	}
	return shots, rails, charges
}
