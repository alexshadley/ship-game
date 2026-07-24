package main

import (
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

// DefaultAsteroids returns a handful of asteroids of various sizes scattered
// around the screen, each drifting with its own velocity.
func DefaultAsteroids() []*Asteroid {
	return []*Asteroid{
		NewAsteroid(rl.NewVector2(120, 120), rl.NewVector2(15, 10), 40),
		NewAsteroid(rl.NewVector2(650, 180), rl.NewVector2(-20, 25), 25),
		NewAsteroid(rl.NewVector2(500, 450), rl.NewVector2(10, -18), 60),
		NewAsteroid(rl.NewVector2(220, 480), rl.NewVector2(-12, -8), 18),
	}
}
