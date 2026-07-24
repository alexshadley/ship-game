package main

import (
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// EnemyAction is one of the behaviors an AI enemy ship can choose to perform.
type EnemyAction int

const (
	// ActionAttack holds facing toward the player and fires the cannons.
	ActionAttack EnemyAction = iota
	// ActionStrafe points tangent to the player and thrusts, circling them.
	ActionStrafe
	// ActionMoveCloser points at the player and thrusts to close the distance.
	ActionMoveCloser
	numEnemyActions
)

// Enemy AI tuning constants.
const (
	// enemyActionMin and enemyActionMax bound the random duration of each chosen
	// action, in seconds.
	enemyActionMin = 1.0
	enemyActionMax = 3.0
	// enemyTurnP and enemyTurnD are the proportional and derivative gains of the
	// steering controller: turn ∝ heading error, damped by the current turn rate
	// so the ship settles onto its target heading instead of spinning past it.
	enemyTurnP = 1.5
	enemyTurnD = 0.5
	// enemyFireAngle is how closely (radians) an enemy must be aimed at the player
	// before it will fire.
	enemyFireAngle = 0.3
)

// EnemyAI is a Controller that drives an enemy ship. It reuses the same Controls
// (thrust/turn/fire) and physics as the player; only the source of the signals
// differs. Its behavior is a simple loop: pick a random action for a random
// duration, translate that action into controls each frame, then pick again.
type EnemyAI struct {
	ship   *Ship // the enemy ship this AI drives (its own position/heading)
	target *Ship // the ship to hunt (the player)

	action     EnemyAction
	actionTime float32 // remaining time on the current action, seconds
	strafeSign float32 // +1 or -1: which way this strafe circles the player
}

// NewEnemyAI returns an AI controller that drives self to hunt target.
func NewEnemyAI(self, target *Ship) *EnemyAI {
	ai := &EnemyAI{ship: self, target: target}
	ai.pickAction()
	return ai
}

// pickAction chooses a fresh random action and duration for the enemy.
func (ai *EnemyAI) pickAction() {
	ai.action = EnemyAction(rand.Intn(int(numEnemyActions)))
	ai.actionTime = enemyActionMin + rand.Float32()*(enemyActionMax-enemyActionMin)
	if rand.Intn(2) == 0 {
		ai.strafeSign = 1
	} else {
		ai.strafeSign = -1
	}
}

// Controls advances the AI by dt and returns the ship controls for this frame.
func (ai *EnemyAI) Controls(dt float32) Controls {
	ai.actionTime -= dt
	if ai.actionTime <= 0 {
		ai.pickAction()
	}

	// Vector from the enemy to the player, and the heading that aims along it.
	// Heading 0 faces "up" (-Y) and increases clockwise, so a world direction
	// (vx, vy) is faced with heading atan2(vx, -vy).
	dx := ai.target.Position.X - ai.ship.Position.X
	dy := ai.target.Position.Y - ai.ship.Position.Y
	aimHeading := heading(dx, dy)

	var desired float32 // heading to steer toward
	var thrust float32
	var fire bool
	switch ai.action {
	case ActionAttack:
		desired = aimHeading
		fire = float32(math.Abs(float64(angleDiff(ai.ship.Direction, aimHeading)))) < enemyFireAngle
	case ActionMoveCloser:
		desired = aimHeading
		thrust = 1
	case ActionStrafe:
		// Face tangent to the circle around the player, then thrust to orbit.
		desired = heading(-dy*ai.strafeSign, dx*ai.strafeSign)
		thrust = 1
	}

	// PD steering: turn proportional to heading error, braked by the turn rate.
	err := angleDiff(desired, ai.ship.Direction)
	turn := clamp(err*enemyTurnP-ai.ship.AngularVelocity*enemyTurnD, -1, 1)

	return Controls{Thrust: thrust, Turn: turn, Fire: fire}
}

// heading returns the ship heading that points a nose along world direction
// (vx, vy). The nose vector at heading h is (sin h, -cos h), so h = atan2(vx, -vy).
func heading(vx, vy float32) float32 {
	return float32(math.Atan2(float64(vx), float64(-vy)))
}

// angleDiff returns the smallest signed difference a-b wrapped to [-π, π].
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

// clamp constrains v to the range [lo, hi].
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
	// enemySpawnRadius is how far from the target an enemy appears when spawned.
	// It sits beyond the piloting view's visible corner (~1470 world px at
	// pilotingZoom), so enemies always arrive from offscreen.
	enemySpawnRadius = 1600
	// enemySpawnInterval is the delay, in seconds, between enemy spawns.
	enemySpawnInterval = 30
)

// SpawnEnemy creates a single enemy ship at a random point offscreen around
// target, returning it with an AI controller set to hunt target.
func SpawnEnemy(target *Ship) (*Ship, Controller) {
	angle := rand.Float64() * 2 * math.Pi
	pos := rl.NewVector2(
		target.Position.X+float32(math.Cos(angle))*enemySpawnRadius,
		target.Position.Y+float32(math.Sin(angle))*enemySpawnRadius,
	)
	enemy := DefaultShip(pos)
	return enemy, NewEnemyAI(enemy, target)
}
