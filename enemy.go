package main

import (
	"math"
	"math/rand"
	"path/filepath"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	enemyTurnP = 1.5
	enemyTurnD = 0.5

	// enemyThrustAlignAngle is how close (radians) the nose must be to the goal
	// heading before the enemy fires its engine. Thrust is applied along the ship's
	// current facing, not the goal, so burning while still mid-turn just pushes the
	// ship sideways and makes it wobble. Gating thrust on alignment makes it turn
	// first, then close the distance.
	enemyThrustAlignAngle = 0.5

	// avoidLookahead is how far ahead (world px) beyond an obstacle's clearance an
	// enemy starts reacting to it.
	avoidLookahead = 700
	// avoidMargin is extra clearance added to each obstacle on top of the combined
	// physical extents of the obstacle and the enemy ship.
	avoidMargin = 60
	// avoidStrength scales obstacle repulsion relative to the unit goal vector; >1
	// lets a near obstacle override the goal so the enemy steers clear.
	avoidStrength = 1.6
)

type EnemyAI struct {
	ship      *Ship
	target    *Ship
	asteroids []*Asteroid

	// desired is the goal heading the AI steered toward on the last Controls call:
	// toward the target either way, but bent around obstacles by avoidHeading while
	// navigating. Stored so the AI debug overlay can draw it. Note the ship rotates
	// toward this but thrusts along its current facing, so the two only coincide
	// once it finishes turning.
	desired float32
}

func NewEnemyAI(self, target *Ship, asteroids []*Asteroid) *EnemyAI {
	return &EnemyAI{ship: self, target: target, asteroids: asteroids}
}

func (ai *EnemyAI) Controls(dt float32) Controls {
	dx := ai.target.Position.X - ai.ship.Position.X
	dy := ai.target.Position.Y - ai.ship.Position.Y
	aimHeading := heading(dx, dy)
	dist := float32(math.Hypot(float64(dx), float64(dy)))

	// Two modes, switched purely on range:
	//   navigate — farther than the ship's biggest engagement range, so close the
	//              distance by thrusting toward the player (avoiding obstacles).
	//   face     — within that range, so stop and simply hold the nose on the player
	//              while the weapons do the work.
	// Always aim at the player; only the thrust/heading differ between the modes.
	desired := aimHeading
	navigating := dist > ai.maxEngagementRange()
	if navigating {
		// While navigating, bend the goal heading around nearby asteroids and the
		// player so we steer around them on the way in.
		desired = ai.avoidHeading(desired)
	}

	ai.desired = desired

	// PD steering: turn proportional to heading error, braked by the turn rate.
	err := angleDiff(desired, ai.ship.Direction)
	turn := clamp(err*enemyTurnP-ai.ship.AngularVelocity*enemyTurnD, -1, 1)

	// Thrust only while navigating, and only once the nose is roughly on the goal
	// heading — thrust pushes along the current facing, so burning mid-turn shoves
	// the ship sideways and it wobbles instead of closing. Also refuse to thrust
	// straight into whatever's dead ahead.
	var thrust float32
	if navigating && float32(math.Abs(float64(err))) < enemyThrustAlignAngle && !ai.pathBlocked(ai.ship.Direction) {
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
	}
}

// maxEngagementRange is the largest engagement range across the ship's weapon
// mounts — the distance at which the ship's longest-reaching weapon opens fire,
// and the threshold the AI uses to switch from navigating toward the player to
// simply facing it. Returns 0 for an unarmed ship, which keeps it in face mode.
func (ai *EnemyAI) maxEngagementRange() float32 {
	var maxRange float32
	for _, part := range ai.ship.Parts {
		if !part.Type.isWeapon() {
			continue
		}
		if r := part.Type.engagementRange(); r > maxRange {
			maxRange = r
		}
	}
	return maxRange
}

// DrawDebug overlays this enemy's AI state for the AI debug mode: a ring at each
// weapon's engagement range (the distance at which that mount opens fire) and a line
// along the goal heading the AI is steering toward this frame. The direction line is
// the heading the ship is trying to turn toward — toward the player when closing,
// perpendicular when strafing, bent around obstacles by avoidHeading. Because the
// ship thrusts along its current facing, this only matches its actual travel once it
// has finished rotating. Drawn in world space, so call it inside BeginMode2D.
func (ai *EnemyAI) DrawDebug() {
	pos := ai.ship.Position

	// One ring per distinct weapon range the ship actually mounts. Currently PDCs
	// and missiles share a range, so this is usually a single circle. The largest
	// is the mode-switch threshold (see maxEngagementRange).
	maxRange := ai.maxEngagementRange()
	seen := map[float32]bool{}
	for _, part := range ai.ship.Parts {
		if !part.Type.isWeapon() {
			continue
		}
		r := part.Type.engagementRange()
		if seen[r] {
			continue
		}
		seen[r] = true
		rl.DrawCircleLines(int32(pos.X), int32(pos.Y), r, rl.NewColor(255, 120, 40, 110))
	}

	// Target-direction line along ai.desired. Reach it out to the firing ring so the
	// two overlays read together; fall back to a fixed length for an unarmed ship.
	length := maxRange
	if length == 0 {
		length = 700
	}
	nx := float32(math.Sin(float64(ai.desired)))
	ny := -float32(math.Cos(float64(ai.desired)))
	end := rl.NewVector2(pos.X+nx*length, pos.Y+ny*length)
	rl.DrawLineEx(pos, end, 4, rl.NewColor(80, 200, 255, 230))
	rl.DrawCircleV(end, 8, rl.NewColor(80, 200, 255, 230))
}

// avoidHeading blends repulsion from nearby asteroids and the player into the unit
// goal direction and returns a heading to steer toward. Repulsion grows as the ship
// nears an obstacle, so a close rock can override the goal and route the enemy around.
func (ai *EnemyAI) avoidHeading(goal float32) float32 {
	steerX := float32(math.Sin(float64(goal)))
	steerY := -float32(math.Cos(float64(goal)))

	sx, sy := ai.ship.Position.X, ai.ship.Position.Y
	selfR := ai.ship.Radius()

	for _, a := range ai.asteroids {
		rx, ry := steerAway(sx, sy, a.Position.X, a.Position.Y, a.Size+selfR+avoidMargin)
		steerX += rx
		steerY += ry
	}
	// The player is deliberately not treated as an obstacle here: navigate mode
	// exists to close on it, and face mode already halts the enemy at engagement
	// range (well outside the player's hull), so it never rams.

	// If repulsion cancels the goal, keep the original heading rather than spin.
	if steerX*steerX+steerY*steerY < 1e-4 {
		return goal
	}
	return heading(steerX, steerY)
}

// steerAway returns a repulsion vector pushing a ship at (sx,sy) away from an
// obstacle at (ox,oy) with clearance radius r. It is zero beyond r+avoidLookahead
// and grows quadratically to avoidStrength at the clearance boundary.
func steerAway(sx, sy, ox, oy, r float32) (float32, float32) {
	dx := sx - ox
	dy := sy - oy
	d := float32(math.Hypot(float64(dx), float64(dy)))
	influence := r + avoidLookahead
	if d >= influence || d == 0 {
		return 0, 0
	}
	var t float32
	if d <= r {
		t = 1
	} else {
		t = (influence - d) / (influence - r)
	}
	w := t * t * avoidStrength
	return dx / d * w, dy / d * w
}

// pathBlocked reports whether thrusting along heading dir would drive the enemy
// into an obstacle within avoidLookahead — thrust is applied along the ship's
// current facing, so callers pass ai.ship.Direction.
func (ai *EnemyAI) pathBlocked(dir float32) bool {
	dx := float32(math.Sin(float64(dir)))
	dy := -float32(math.Cos(float64(dir)))
	sx, sy := ai.ship.Position.X, ai.ship.Position.Y
	selfR := ai.ship.Radius()

	// Only asteroids block the path — the player is the goal, not an obstacle, so
	// pointing at it must never cut thrust (that would strand the enemy just outside
	// range, facing the player and unable to close).
	for _, a := range ai.asteroids {
		if rayHitsCircle(sx, sy, dx, dy, a.Position.X, a.Position.Y, a.Size+selfR+avoidMargin) {
			return true
		}
	}
	return false
}

// rayHitsCircle reports whether the segment from (px,py) along unit (dx,dy) for
// avoidLookahead world px passes within radius r of the circle centered at (cx,cy).
func rayHitsCircle(px, py, dx, dy, cx, cy, r float32) bool {
	t := (cx-px)*dx + (cy-py)*dy
	if t < 0 {
		t = 0
	} else if t > avoidLookahead {
		t = avoidLookahead
	}
	nx := px + dx*t - cx
	ny := py + dy*t - cy
	return nx*nx+ny*ny <= r*r
}

// heading returns the ship heading that points the nose along world direction
// (vx, vy). The nose vector at heading h is (sin h, -cos h), hence the -vy.
func heading(vx, vy float32) float32 {
	return float32(math.Atan2(float64(vx), float64(-vy)))
}

func angleDiff(a, b float32) float32 {
	d := float64(a - b)
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	return float32(d)
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

const (
	// enemySpawnRadius sits well beyond the piloting view's visible corner so
	// enemies always arrive from offscreen.
	enemySpawnRadius   = 2400
	enemySpawnInterval = 30
)

func SpawnEnemy(target *Ship, asteroids []*Asteroid) (*Ship, *EnemyAI) {
	angle := rand.Float64() * 2 * math.Pi
	pos := rl.NewVector2(
		target.Position.X+float32(math.Cos(angle))*enemySpawnRadius,
		target.Position.Y+float32(math.Sin(angle))*enemySpawnRadius,
	)
	enemy := rosterEnemyShip(pos)
	return enemy, NewEnemyAI(enemy, target, asteroids)
}

// enemyRoster lists the ship designs (files in ships/) that spawn as enemies.
// Add more entries here to widen the rotation; a random one is chosen per spawn.
var enemyRoster = []string{"raider.json", "l_ship.json", "lancelot.json", "fatboy.json"}

// rosterEnemyShip loads a random valid design from enemyRoster. The files are
// re-read on every spawn, so edits saved in the designer take effect without a
// restart. It falls back to the built-in EnemyShip if nothing valid loads.
func rosterEnemyShip(pos rl.Vector2) *Ship {
	roster := append([]string(nil), enemyRoster...)
	rand.Shuffle(len(roster), func(i, j int) { roster[i], roster[j] = roster[j], roster[i] })
	for _, name := range roster {
		ship, err := LoadShipFile(filepath.Join(shipsDir, name), pos)
		if err != nil || ship.Validate() != nil {
			continue
		}
		return ship
	}
	return EnemyShip(pos)
}
