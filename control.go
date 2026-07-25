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

	// AutoFire arms the auto-turrets (PartAutoTurret), which ignore the Fire and
	// FireMissiles triggers above. While AutoFire is set and HasAutoTarget is true
	// each auto-turret slews onto AutoTarget — a world-frame offset from the ship
	// origin to the nearest enemy — and fires on its own. The player flips AutoFire
	// with a hotkey; the enemy AI leaves it on and points it at the player.
	AutoFire      bool
	HasAutoTarget bool
	AutoTarget    rl.Vector2
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
	// autoFire is the shared auto-turret toggle, flipped by a hotkey in the game
	// loop. While set, the ship's auto-turrets track and fire on the nearest enemy
	// on their own; enemies returns the current enemy ships to pick that target
	// from. Both may be nil (e.g. an enemy or a test ship has no auto-turret UI).
	autoFire *bool
	enemies  func() []*Ship
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
	// Auto-turrets aim themselves at the nearest enemy while the toggle is armed,
	// independent of the mouse triggers above.
	if p.autoFire != nil && *p.autoFire {
		c.AutoFire = true
		if p.enemies != nil {
			if tp, ok := nearestShip(p.ship.Position, p.enemies()); ok {
				c.AutoTarget = rl.NewVector2(tp.X-p.ship.Position.X, tp.Y-p.ship.Position.Y)
				c.HasAutoTarget = true
			}
		}
	}
	return c
}

// nearestShip returns the position of the ship in candidates closest to origin,
// skipping any that are nil or already destroyed. Auto-turrets use it to lock
// onto the nearest enemy.
func nearestShip(origin rl.Vector2, candidates []*Ship) (rl.Vector2, bool) {
	var best rl.Vector2
	found := false
	var bestDist float32
	for _, s := range candidates {
		if s == nil || s.Destroyed {
			continue
		}
		if d := dist(origin, s.Position); !found || d < bestDist {
			bestDist = d
			best = s.Position
			found = true
		}
	}
	return best, found
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
