package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

type Controls struct {
	Thrust float32
	Turn   float32
	// Fire holds the PDC trigger; FireMissiles holds the missile-launcher
	// trigger. They fire independently so the player can loose PDCs and missiles
	// separately (left mouse vs shift). FireTarget is where both weapon types
	// should aim: a world-frame offset from the ship's origin (the cockpit cell)
	// to the aim point. The player aims at the cursor; AIs aim at their target's
	// cockpit.
	Fire         bool
	FireMissiles bool
	FireTarget   rl.Vector2
	// EnforceEngagementRange gates each weapon mount's fire on that weapon's
	// engagement range (see PartType.engagementRange). The AI sets it so enemies
	// hold the trigger continuously but only open up once a shot can reach; the
	// player fires freely.
	EnforceEngagementRange bool
}

type Controller interface {
	Controls(dt float32) Controls
}

// PlayerInput reads the piloting controls: WASD for thrust and turn, the held
// left mouse button to fire PDCs at the cursor, and held shift to loose
// missiles at the cursor. It needs the camera to unproject the cursor into
// world space and the ship to express that aim point relative to the ship's
// origin.
type PlayerInput struct {
	camera *rl.Camera2D
	ship   *Ship
}

func (p PlayerInput) Controls(dt float32) Controls {
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
	c.Fire = rl.IsMouseButtonDown(rl.MouseLeftButton)
	c.FireMissiles = rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	if c.Fire || c.FireMissiles {
		m := mouseWorld(*p.camera)
		c.FireTarget = rl.NewVector2(m.X-p.ship.Position.X, m.Y-p.ship.Position.Y)
	}
	return c
}

type PilotInput struct {
	spacewalking *bool
	player       PlayerInput
}

func (p PilotInput) Controls(dt float32) Controls {
	// While spacewalking the ship coasts: WASD and the mouse drive the
	// astronaut (movement, grabbing, repairs), not the ship.
	if p.spacewalking != nil && *p.spacewalking {
		return Controls{}
	}
	return p.player.Controls(dt)
}
