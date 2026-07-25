package main

import (
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type Asteroid struct {
	Position rl.Vector2
	Velocity rl.Vector2
	Size     float32
}

func NewAsteroid(pos, velocity rl.Vector2, size float32) *Asteroid {
	return &Asteroid{
		Position: pos,
		Velocity: velocity,
		Size:     size,
	}
}

func (a *Asteroid) Update(dt float32) {
	a.Position.X += a.Velocity.X * dt
	a.Position.Y += a.Velocity.Y * dt
}

func (a *Asteroid) Draw() {
	rl.DrawCircleV(a.Position, a.Size, rl.DarkGray)
	highlight := rl.NewVector2(a.Position.X-a.Size*0.3, a.Position.Y-a.Size*0.3)
	rl.DrawCircleV(highlight, a.Size*0.6, rl.Gray)
}

const (
	fieldRadius      = 3000
	fieldHugeCount   = 7
	fieldSmallCount  = 25
	spawnClearRadius = 700

	asteroidMinSize = 15
	asteroidMaxSize = 70
	asteroidHugeMin = 150
	asteroidHugeMax = 400
)

func DefaultAsteroids() []*Asteroid {
	asteroids := make([]*Asteroid, 0, fieldHugeCount+fieldSmallCount)

	spawn := func(count int, minSize, maxSize float32) {
		for spawned := 0; spawned < count; {
			pos := rl.NewVector2(
				(rand.Float32()*2-1)*fieldRadius,
				(rand.Float32()*2-1)*fieldRadius,
			)
			// Keep the ship's spawn (and the enemies near it) clear of rocks.
			if pos.X*pos.X+pos.Y*pos.Y < spawnClearRadius*spawnClearRadius {
				continue
			}

			size := minSize + rand.Float32()*(maxSize-minSize)
			asteroids = append(asteroids, NewAsteroid(pos, rl.NewVector2(0, 0), size))
			spawned++
		}
	}

	spawn(fieldHugeCount, asteroidHugeMin, asteroidHugeMax)
	spawn(fieldSmallCount, asteroidMinSize, asteroidMaxSize)

	return asteroids
}
