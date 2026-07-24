package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Controls is the per-frame command set for a single ship. The physics runs
// every ship from these three signals, regardless of whether they come from
// player input or from AI.
type Controls struct {
	Thrust float32 // forward throttle in [0,1] (fires the engines)
	Turn   float32 // steering in [-1,1]; negative turns left, positive right
	Fire   bool    // whether to fire the cannons this frame
}

// Controller produces a ship's Controls each frame. PlayerInput reads the
// keyboard; EnemyAI computes them from the world state.
type Controller interface {
	Controls(dt float32) Controls
}

// PlayerInput is the Controller that maps the keyboard to a ship's controls:
//
//	W     - forward thrust
//	A / D - turn left / right
//	Shift - fire cannons
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

// PilotInput is the player ship's Controller. It reads the keyboard while the
// pilot is aboard, but yields no controls while they're out on a spacewalk (the
// ship coasts and holds fire, and WASD/Shift drive the astronaut instead). It
// reads the shared spacewalking flag owned by the main loop.
type PilotInput struct {
	spacewalking *bool
}

func (p PilotInput) Controls(dt float32) Controls {
	if p.spacewalking != nil && *p.spacewalking {
		return Controls{}
	}
	return PlayerInput{}.Controls(dt)
}
