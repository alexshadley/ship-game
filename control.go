package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

type Controls struct {
	Thrust float32
	Turn   float32
	// Fire holds the PDC trigger; FireMissiles holds the missile-launcher
	// trigger. They fire independently (AIs may loose one without the other);
	// the player's right mouse fires PDCs, left mouse fires missiles, and shift
	// also fires missiles. PDCTarget and MissileTarget are where each weapon type
	// aims: a world-frame offset from the ship's origin (the cockpit cell) to the
	// aim point. The player aims both at the cursor, so they coincide; enemy AIs
	// aim their missiles at the target ship but retask their PDCs onto nearer
	// threats (inbound missiles, an exposed astronaut) when one is in range.
	Fire          bool
	FireMissiles  bool
	PDCTarget     rl.Vector2
	MissileTarget rl.Vector2
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
// right mouse button to fire PDCs at the cursor, and the held left mouse button
// to loose missiles (shift also fires missiles). It needs the camera to unproject the cursor into
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
	c.Fire = rl.IsMouseButtonDown(rl.MouseRightButton)
	c.FireMissiles = rl.IsMouseButtonDown(rl.MouseLeftButton) || rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
	if c.Fire || c.FireMissiles {
		m := mouseWorld(*p.camera)
		aim := rl.NewVector2(m.X-p.ship.Position.X, m.Y-p.ship.Position.Y)
		// The player aims both weapon types at the cursor.
		c.PDCTarget = aim
		c.MissileTarget = aim
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
