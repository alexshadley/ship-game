package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

type Controls struct {
	Thrust float32
	Turn   float32
	Fire   bool
}

type Controller interface {
	Controls(dt float32) Controls
}

type PlayerInput struct{}

func (PlayerInput) Controls(dt float32) Controls {
	var c Controls
	if rl.IsKeyDown(rl.KeyW) {
		c.Thrust = 1
	}
	if rl.IsKeyDown(rl.KeyA) {
		c.Turn -= 1
	}
	if rl.IsKeyDown(rl.KeyD) {
		c.Turn += 1
	}
	if rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift) {
		c.Fire = true
	}
	return c
}

type PilotInput struct {
	spacewalking *bool
}

func (p PilotInput) Controls(dt float32) Controls {
	// While spacewalking the ship coasts: WASD/Shift drive the astronaut, not the ship.
	if p.spacewalking != nil && *p.spacewalking {
		return Controls{}
	}
	return PlayerInput{}.Controls(dt)
}
