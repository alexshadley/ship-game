package main

import (
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

func DefaultAsteroids() []*Asteroid {
	return []*Asteroid{
		NewAsteroid(rl.NewVector2(120, 120), rl.NewVector2(15, 10), 40),
		NewAsteroid(rl.NewVector2(650, 180), rl.NewVector2(-20, 25), 25),
		NewAsteroid(rl.NewVector2(500, 450), rl.NewVector2(10, -18), 60),
		NewAsteroid(rl.NewVector2(220, 480), rl.NewVector2(-12, -8), 18),
	}
}
