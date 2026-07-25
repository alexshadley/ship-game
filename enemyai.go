package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	enemyTurnP = 1.5
	enemyTurnD = 0.5

	// enemyThrustAlignAngle is how close (radians) the nose must be to the player
	// before the enemy fires its engine. Thrust is applied along the ship's current
	// facing, so burning while still mid-turn just pushes the ship sideways and makes
	// it wobble. Gating thrust on alignment makes it turn first, then close.
	enemyThrustAlignAngle = 0.5
)

// AIWorld is the read-only snapshot of the world the enemy AI reads to decide its
// controls: the ship it flies (Self) and the ship it hunts (Target).
type AIWorld struct {
	Self   *Ship
	Target *Ship
}

// enemyControls is the enemy AI: a pure function from a snapshot of the world to
// the controls handed to the ship simulator. It always aims straight at the
// player, thrusting to close while out of weapon range and holding still to shoot
// once inside it. It also returns the goal heading (always toward the player) for
// the debug overlay.
func enemyControls(w AIWorld) (Controls, float32) {
	self, target := w.Self, w.Target
	dx := target.Position.X - self.Position.X
	dy := target.Position.Y - self.Position.Y
	desired := heading(dx, dy)
	dist := float32(math.Hypot(float64(dx), float64(dy)))

	// PD steering: turn proportional to heading error, braked by the turn rate.
	err := angleDiff(desired, self.Direction)
	turn := clamp(err*enemyTurnP-self.AngularVelocity*enemyTurnD, -1, 1)

	// Thrust only while farther than the ship's biggest engagement range, and only
	// once the nose is roughly on the player — thrust pushes along the current
	// facing, so burning mid-turn shoves the ship sideways and it wobbles instead of
	// closing. Inside range it holds still and lets the weapons work.
	var thrust float32
	if dist > maxEngagementRange(self) && float32(math.Abs(float64(err))) < enemyThrustAlignAngle {
		thrust = 1
	}

	// The enemy always holds its trigger and aims at the player's cockpit (the
	// target ship's origin is its cockpit cell, so the offset from our origin is
	// simply the position delta). Each weapon mount only actually fires once the
	// player is within that weapon's engagement range — see FireWeapons.
	return Controls{
		Thrust:                 thrust,
		Turn:                   turn,
		Fire:                   true,
		FireMissiles:           true,
		FireTarget:             rl.NewVector2(dx, dy),
		EnforceEngagementRange: true,
	}, desired
}

// maxEngagementRange is the largest engagement range across the ship's weapon
// mounts — the distance at which the ship's longest-reaching weapon opens fire,
// and the threshold the AI uses to stop closing and simply face the player.
// Returns 0 for an unarmed ship, which keeps it holding still.
func maxEngagementRange(ship *Ship) float32 {
	var maxRange float32
	for _, part := range ship.Parts {
		if !part.Type.isWeapon() {
			continue
		}
		if r := part.Type.engagementRange(); r > maxRange {
			maxRange = r
		}
	}
	return maxRange
}
