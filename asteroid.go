package main

import (
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Asteroid is a free-floating rock in the world. It has a position, a velocity
// carrying it across the world each second, and a size (its radius in pixels).
// For now it is rendered as a simple shaded sphere.
type Asteroid struct {
	Position rl.Vector2 // world position of the asteroid's center
	Velocity rl.Vector2 // world-space velocity in pixels/sec
	Size     float32    // radius in pixels
}

// NewAsteroid returns an asteroid at pos moving with the given velocity.
func NewAsteroid(pos, velocity rl.Vector2, size float32) *Asteroid {
	return &Asteroid{
		Position: pos,
		Velocity: velocity,
		Size:     size,
	}
}

// Update advances the asteroid's position by its velocity over dt seconds.
func (a *Asteroid) Update(dt float32) {
	a.Position.X += a.Velocity.X * dt
	a.Position.Y += a.Velocity.Y * dt
}

// Draw renders the asteroid as a shaded sphere: a dark base disc with a lighter
// highlight offset toward the upper-left to give a sense of volume.
func (a *Asteroid) Draw() {
	rl.DrawCircleV(a.Position, a.Size, rl.DarkGray)
	highlight := rl.NewVector2(a.Position.X-a.Size*0.3, a.Position.Y-a.Size*0.3)
	rl.DrawCircleV(highlight, a.Size*0.6, rl.Gray)
}

// Asteroid field generation tuning. The field is a large square region centered
// on the world origin (where the ship spawns); asteroids are scattered randomly
// within it but kept clear of the spawn point.
const (
	// fieldRadius is the half-extent of the asteroid field in world pixels, so
	// the field spans 2*fieldRadius on each axis around the origin.
	fieldRadius = 3000
	// fieldCount is how many asteroids to scatter through the field.
	fieldCount = 60
	// spawnClearRadius keeps asteroids from spawning on top of the ship (and the
	// enemies that start near the origin).
	spawnClearRadius = 700

	// Asteroid sizes (radius in pixels). Most rocks fall in the small/medium
	// range; a small fraction are boulders that are much larger.
	asteroidMinSize    = 15
	asteroidMaxSize    = 70
	asteroidHugeMin    = 150
	asteroidHugeMax    = 400
	asteroidHugeChance = 0.12
)

// DefaultAsteroids randomly scatters a large field of stationary asteroids
// around the world origin. Sizes vary, with a fraction being much larger
// boulders. Asteroids start at rest (zero velocity) so the field is still at
// game start, and are kept clear of the ship's spawn point.
func DefaultAsteroids() []*Asteroid {
	asteroids := make([]*Asteroid, 0, fieldCount)
	for len(asteroids) < fieldCount {
		pos := rl.NewVector2(
			(rand.Float32()*2-1)*fieldRadius,
			(rand.Float32()*2-1)*fieldRadius,
		)
		// Reject positions too close to the origin so the ship (and nearby
		// enemies) don't start buried inside a rock.
		if pos.X*pos.X+pos.Y*pos.Y < spawnClearRadius*spawnClearRadius {
			continue
		}

		var size float32
		if rand.Float32() < asteroidHugeChance {
			size = asteroidHugeMin + rand.Float32()*(asteroidHugeMax-asteroidHugeMin)
		} else {
			size = asteroidMinSize + rand.Float32()*(asteroidMaxSize-asteroidMinSize)
		}

		asteroids = append(asteroids, NewAsteroid(pos, rl.NewVector2(0, 0), size))
	}
	return asteroids
}
