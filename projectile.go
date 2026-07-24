package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	cannonMuzzleSpeed  = 1200.0
	cannonFireInterval = 0.18
	// weakCannonFireInterval is the junk cannon's cadence: a third the fire rate of a
	// standard cannon.
	weakCannonFireInterval = cannonFireInterval * 3
	projectileLifespan     = 15.0
)

var projectileSize = rl.NewVector2(4, 12)

type Projectile struct {
	Position rl.Vector2
	Velocity rl.Vector2
	Lifespan float32
	Rotation float32
	Size     rl.Vector2
}

func NewProjectile(pos, velocity rl.Vector2, rotation float32) *Projectile {
	return &Projectile{
		Position: pos,
		Velocity: velocity,
		Lifespan: projectileLifespan,
		Rotation: rotation,
		Size:     projectileSize,
	}
}

func (p *Projectile) Update(dt float32) {
	p.Position.X += p.Velocity.X * dt
	p.Position.Y += p.Velocity.Y * dt
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

// fireInterval is the cadence between this cannon's shots: a weak junk cannon
// fires at a third the rate of a standard cannon. Each cannon keeps its own
// cadence, so a slow cannon never drags down the ship's other cannons.
func (t PartType) fireInterval() float32 {
	if t == PartWeakCannon {
		return weakCannonFireInterval
	}
	return cannonFireInterval
}

// FireCannons advances each cannon's independent cooldown and returns shots from
// the cannons that are ready to fire while firing is held.
func (s *Ship) FireCannons(dt float32, firing bool) []*Projectile {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))

	var shots []*Projectile
	for c, part := range s.Parts {
		if part.Type != PartCannon && part.Type != PartWeakCannon {
			continue
		}

		part.FireCooldown -= dt
		if !firing || part.FireCooldown > 0 {
			continue
		}
		part.FireCooldown = part.Type.fireInterval()

		off := part.Facing.offset()
		ldx, ldy := float32(off.X), float32(off.Y)
		wdx := ldx*cos - ldy*sin
		wdy := ldx*sin + ldy*cos

		lx := float32(c.X)*cellSize + ldx*cellSize*0.5
		ly := float32(c.Y)*cellSize + ldy*cellSize*0.5
		pos := s.worldPoint(lx, ly)

		vel := rl.NewVector2(
			s.Velocity.X+wdx*cannonMuzzleSpeed,
			s.Velocity.Y+wdy*cannonMuzzleSpeed,
		)
		rotation := s.Direction + part.Facing.angle()
		shots = append(shots, NewProjectile(pos, vel, rotation))
	}
	return shots
}
