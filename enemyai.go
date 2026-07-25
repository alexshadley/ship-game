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
// controls: the ship it flies (Self), the ship it hunts (Target), and Threats —
// world points its PDC should prefer over the target ship (the player's in-flight
// missiles and, while spacewalking, the exposed astronaut).
type AIWorld struct {
	Self    *Ship
	Target  *Ship
	Threats []rl.Vector2
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

	// The enemy always holds both triggers. The missile launcher aims at the
	// player's cockpit (the target ship's origin is its cockpit cell, so the
	// offset from our origin is simply the position delta). The PDC aims there too
	// by default, but if an inbound player missile or the spacewalking astronaut
	// has drifted within PDC range it retasks onto the nearest such threat (no
	// lead) to swat the immediate danger. Each mount only actually fires once its
	// target is within that weapon's engagement range — see FireWeapons.
	missileTarget := rl.NewVector2(dx, dy)
	pdcTarget := missileTarget
	if threat, ok := nearestPDCThreat(self.Position, w.Threats); ok {
		pdcTarget = rl.NewVector2(threat.X-self.Position.X, threat.Y-self.Position.Y)
	}
	return Controls{
		Thrust:                 thrust,
		Turn:                   turn,
		Fire:                   true,
		FireMissiles:           true,
		PDCTarget:              pdcTarget,
		MissileTarget:          missileTarget,
		EnforceEngagementRange: true,
	}, desired
}

// nearestPDCThreat returns the closest threat point within pdcEngagementRange of
// origin, if any. The enemy retasks its PDC onto it — an inbound player missile
// or the spacewalking astronaut — to swat the immediate danger instead of
// peppering the target's hull. Threats farther than a PDC round can reach are
// ignored, so distant missiles don't pull the guns off the ship.
func nearestPDCThreat(origin rl.Vector2, threats []rl.Vector2) (rl.Vector2, bool) {
	var best rl.Vector2
	found := false
	bestDist := float32(pdcEngagementRange)
	for _, t := range threats {
		if d := dist(origin, t); d <= bestDist {
			bestDist = d
			best = t
			found = true
		}
	}
	return best, found
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
