package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	pdcMuzzleSpeed  = 1200.0
	pdcFireInterval = 0.18
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
)

var projectileSize = rl.NewVector2(4, 12)

type Projectile struct {
	Position rl.Vector2
	Velocity rl.Vector2
	Lifespan float32
	Rotation float32
	Size     rl.Vector2
	// Owner is the ship that fired the round; rounds pass harmlessly through
	// their own ship but strike everything else.
	Owner *Ship
}

func NewProjectile(owner *Ship, pos, velocity rl.Vector2, rotation float32) *Projectile {
	return &Projectile{
		Position: pos,
		Velocity: velocity,
		Lifespan: pdcProjectileLifespan,
		Rotation: rotation,
		Size:     projectileSize,
		Owner:    owner,
	}
}

func (p *Projectile) Update(dt float32) {
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

func (p *Projectile) Draw() {
	rec := rl.NewRectangle(p.Position.X, p.Position.Y, p.Size.X, p.Size.Y)
	origin := rl.NewVector2(p.Size.X/2, p.Size.Y/2)
	rl.DrawRectanglePro(rec, origin, p.Rotation*180/math.Pi, rl.Yellow)
}

// fireInterval is the cadence between this PDC's shots: a slow junk PDC fires
// at a third the rate of a standard PDC. Each PDC keeps its own cadence, so a
// slow mount never drags down the ship's other mounts.
func (t PartType) fireInterval() float32 {
	if t == PartSlowPDC {
		return slowPDCFireInterval
	}
	return pdcFireInterval
}

// FirePDCs advances each PDC's independent cooldown and returns rounds from the
// mounts ready to fire while the trigger is held. Each mount aims itself at the
// controls' fire target (a world-frame offset from the ship origin) as long as
// that target lies within pdcHalfArc of the mount's facing; a mount whose
// target is outside its arc holds fire until the target swings back in.
func (s *Ship) FirePDCs(dt float32, controls Controls) []*Projectile {
	target := rl.NewVector2(
		s.Position.X+controls.FireTarget.X,
		s.Position.Y+controls.FireTarget.Y,
	)

	var shots []*Projectile
	for c, part := range s.Parts {
		if part.Type != PartPDC && part.Type != PartSlowPDC {
			continue
		}

		part.FireCooldown -= dt
		if !controls.Fire || part.FireCooldown > 0 {
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
		aim := heading(dx, dy)
		mount := s.Direction + part.Facing.angle()
		if math.Abs(float64(angleDiff(aim, mount))) > pdcHalfArc {
			continue
		}
		part.FireCooldown = part.Type.fireInterval()

		dirX, dirY := dx/dist, dy/dist
		pos := rl.NewVector2(center.X+dirX*cellSize*0.5, center.Y+dirY*cellSize*0.5)
		vel := rl.NewVector2(
			s.Velocity.X+dirX*pdcMuzzleSpeed,
			s.Velocity.Y+dirY*pdcMuzzleSpeed,
		)
		shots = append(shots, NewProjectile(s, pos, vel, aim))
	}
	return shots
}
