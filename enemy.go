package main

import (
	"math"
	"math/rand"
	"path/filepath"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type EnemyAction int

const (
	ActionStrafe EnemyAction = iota
	ActionMoveCloser
	numEnemyActions
)

const (
	enemyActionMin = 1.0
	enemyActionMax = 3.0
	enemyTurnP = 1.5
	enemyTurnD = 0.5

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

	action     EnemyAction
	actionTime float32
	strafeSign float32
}

func NewEnemyAI(self, target *Ship, asteroids []*Asteroid) *EnemyAI {
	ai := &EnemyAI{ship: self, target: target, asteroids: asteroids}
	ai.pickAction()
	return ai
}

func (ai *EnemyAI) pickAction() {
	ai.action = EnemyAction(rand.Intn(int(numEnemyActions)))
	ai.actionTime = enemyActionMin + rand.Float32()*(enemyActionMax-enemyActionMin)
	if rand.Intn(2) == 0 {
		ai.strafeSign = 1
	} else {
		ai.strafeSign = -1
	}
}

func (ai *EnemyAI) Controls(dt float32) Controls {
	ai.actionTime -= dt
	if ai.actionTime <= 0 {
		ai.pickAction()
	}

	dx := ai.target.Position.X - ai.ship.Position.X
	dy := ai.target.Position.Y - ai.ship.Position.Y
	aimHeading := heading(dx, dy)

	var desired float32
	var thrust float32
	switch ai.action {
	case ActionMoveCloser:
		desired = aimHeading
		thrust = 1
	case ActionStrafe:
		desired = heading(-dy*ai.strafeSign, dx*ai.strafeSign)
		thrust = 1
	}

	// When driving somewhere, bend the goal heading around nearby asteroids and the
	// player, and refuse to thrust straight into whatever's dead ahead.
	if thrust != 0 {
		desired = ai.avoidHeading(desired)
		if ai.pathBlocked(ai.ship.Direction) {
			thrust = 0
		}
	}

	// PD steering: turn proportional to heading error, braked by the turn rate.
	err := angleDiff(desired, ai.ship.Direction)
	turn := clamp(err*enemyTurnP-ai.ship.AngularVelocity*enemyTurnD, -1, 1)

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
	// Push off the player too so enemies don't ram it, using its physical extent so
	// they can still close to firing range before the repulsion bites.
	rx, ry := steerAway(sx, sy, ai.target.Position.X, ai.target.Position.Y, ai.target.Radius()+selfR+avoidMargin)
	steerX += rx
	steerY += ry

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

	for _, a := range ai.asteroids {
		if rayHitsCircle(sx, sy, dx, dy, a.Position.X, a.Position.Y, a.Size+selfR+avoidMargin) {
			return true
		}
	}
	return rayHitsCircle(sx, sy, dx, dy, ai.target.Position.X, ai.target.Position.Y, ai.target.Radius()+selfR+avoidMargin)
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
	enemySpawnRadius   = 4800
	enemySpawnInterval = 30
)

func SpawnEnemy(target *Ship, asteroids []*Asteroid) (*Ship, Controller) {
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
