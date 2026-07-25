package main

import (
	"math"

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
	missileHalfArc      = 0.15 * math.Pi
	missileLaunchSpeed  = 120.0
	missileCruiseSpeed  = 480.0
	missileAcceleration = 320.0
	missileLifespan     = 7.0
	missileHealth       = 15.0

	// missileEngagementRange is the fire-target distance (world px) within which a
	// missile launcher opens up when Controls.EnforceEngagementRange is set. A
	// missile self-propels to a cruise speed and reaches far further than a PDC
	// round, so it engages from about twice the PDC range.
	missileEngagementRange = 2 * pdcEngagementRange
)

var (
	projectileSize = rl.NewVector2(4, 12)
	missileSize    = rl.NewVector2(12, 34)
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
	Health float32
}

func NewProjectile(owner *Ship, pos, velocity rl.Vector2, rotation float32) *Projectile {
	return &Projectile{
		Position: pos,
		Velocity: velocity,
		Lifespan: pdcProjectileLifespan,
		Rotation: rotation,
		Size:     projectileSize,
		Owner:    owner,
		Kind:     projectilePDC,
	}
}

// NewMissile builds a missile launched from owner: it starts slow (velocity is
// the launch velocity along the aim) and accelerates toward a cruise speed in
// Update, and it carries health so it can be shot down before impact.
func NewMissile(owner *Ship, pos, velocity rl.Vector2, rotation float32) *Projectile {
	return &Projectile{
		Position: pos,
		Velocity: velocity,
		Lifespan: missileLifespan,
		Rotation: rotation,
		Size:     missileSize,
		Owner:    owner,
		Kind:     projectileMissile,
		Health:   missileHealth,
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
// round leaving the muzzle deals full projectileDamage, and drag bleeds that off
// in step with its speed until the round peters out. A round can't deal more than
// full damage, so a fast-moving shooter's rounds still cap at projectileDamage.
func (p *Projectile) Damage() float32 {
	speed := math.Hypot(float64(p.Velocity.X), float64(p.Velocity.Y))
	frac := speed / pdcMuzzleSpeed
	if frac > 1 {
		frac = 1
	}
	return projectileDamage * float32(frac)
}

func (p *Projectile) Draw() {
	rec := rl.NewRectangle(p.Position.X, p.Position.Y, p.Size.X, p.Size.Y)
	origin := rl.NewVector2(p.Size.X/2, p.Size.Y/2)
	color := rl.Yellow
	if p.Kind == projectileMissile {
		color = rl.Red
	}
	rl.DrawRectanglePro(rec, origin, p.Rotation*180/math.Pi, color)
}

// isWeapon reports whether a part is a firing mount handled by FireWeapons.
func (t PartType) isWeapon() bool {
	return t == PartPDC || t == PartSlowPDC || t == PartMissileLauncher
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
	default:
		return pdcFireInterval
	}
}

// halfArc is how far a mount may slew its aim to either side of its mount facing.
// A missile launcher has a much tighter arc than a PDC.
func (t PartType) halfArc() float32 {
	if t == PartMissileLauncher {
		return missileHalfArc
	}
	return pdcHalfArc
}

// engagementRange is how close (world px) the fire target must be for this mount
// to open fire when Controls.EnforceEngagementRange is set. A missile launcher
// reaches about twice as far as a PDC. The player fires without this gate.
func (t PartType) engagementRange() float32 {
	if t == PartMissileLauncher {
		return missileEngagementRange
	}
	return pdcEngagementRange
}

// FireWeapons advances each weapon mount's independent cooldown and returns the
// rounds from mounts ready to fire while the trigger is held. Each mount aims
// itself at the controls' fire target (a world-frame offset from the ship
// origin) as long as that target lies within the mount's arc; a mount whose
// target is outside its arc holds fire until the target swings back in. PDCs
// spit fast, short-ranged rounds; missile launchers loose a slow, accelerating,
// destructible missile.
func (s *Ship) FireWeapons(dt float32, controls Controls) []*Projectile {
	target := rl.NewVector2(
		s.Position.X+controls.FireTarget.X,
		s.Position.Y+controls.FireTarget.Y,
	)

	var shots []*Projectile
	for c, part := range s.Parts {
		if !part.Type.isWeapon() {
			continue
		}

		part.FireCooldown -= dt
		// PDCs fire on the main trigger; missile launchers have their own trigger
		// so the player can loose the two weapon types independently.
		triggered := controls.Fire
		if part.Type == PartMissileLauncher {
			triggered = controls.FireMissiles
		}
		if !triggered || part.FireCooldown > 0 {
			continue
		}

		// Aim from this mount's own cell so converging fire actually converges
		// on the target point rather than running parallel.
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		dx := target.X - center.X
		dy := target.Y - center.Y
		dist := float32(math.Hypot(float64(dx), float64(dy)))
		if dist == 0 {
			continue
		}
		// The AI holds its trigger continuously; each mount only opens up once the
		// target is within its own weapon's engagement range. The player fires
		// without this gate.
		if controls.EnforceEngagementRange && dist > part.Type.engagementRange() {
			continue
		}
		aim := heading(dx, dy)
		mount := s.Direction + part.Facing.angle()
		if math.Abs(float64(angleDiff(aim, mount))) > float64(part.Type.halfArc()) {
			continue
		}
		part.FireCooldown = part.Type.fireInterval()

		dirX, dirY := dx/dist, dy/dist
		pos := rl.NewVector2(center.X+dirX*cellSize*0.5, center.Y+dirY*cellSize*0.5)
		if part.Type == PartMissileLauncher {
			// A missile is self-propelled: it leaves the tube slowly along the aim
			// (not inheriting the ship's velocity) and accelerates from there.
			vel := rl.NewVector2(dirX*missileLaunchSpeed, dirY*missileLaunchSpeed)
			shots = append(shots, NewMissile(s, pos, vel, aim))
			continue
		}
		vel := rl.NewVector2(
			s.Velocity.X+dirX*pdcMuzzleSpeed,
			s.Velocity.Y+dirY*pdcMuzzleSpeed,
		)
		shots = append(shots, NewProjectile(s, pos, vel, aim))
	}
	return shots
}
