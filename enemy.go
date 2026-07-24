package main

import (
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type EnemyAction int

const (
	ActionAttack EnemyAction = iota
	ActionStrafe
	ActionMoveCloser
	numEnemyActions
)

const (
	enemyActionMin = 1.0
	enemyActionMax = 3.0
	enemyTurnP     = 1.5
	enemyTurnD     = 0.5
	enemyFireAngle = 0.3
	// enemyFireRange is how close (world px) an enemy must be to the player before
	// it will fire. It's roughly the piloting view's visible half-extent (~1470 at
	// pilotingZoom) so enemies engage around the time they come onscreen, without
	// tying the rule to the exact viewport size.
	enemyFireRange = 1400
)

type EnemyAI struct {
	ship   *Ship
	target *Ship

	action     EnemyAction
	actionTime float32
	strafeSign float32
}

func NewEnemyAI(self, target *Ship) *EnemyAI {
	ai := &EnemyAI{ship: self, target: target}
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
	inFireRange := dx*dx+dy*dy <= enemyFireRange*enemyFireRange

	var desired float32
	var thrust float32
	var fire bool
	switch ai.action {
	case ActionAttack:
		desired = aimHeading
		fire = inFireRange && float32(math.Abs(float64(angleDiff(ai.ship.Direction, aimHeading)))) < enemyFireAngle
	case ActionMoveCloser:
		desired = aimHeading
		thrust = 1
	case ActionStrafe:
		desired = heading(-dy*ai.strafeSign, dx*ai.strafeSign)
		thrust = 1
	}

	// PD steering: turn proportional to heading error, braked by the turn rate.
	err := angleDiff(desired, ai.ship.Direction)
	turn := clamp(err*enemyTurnP-ai.ship.AngularVelocity*enemyTurnD, -1, 1)

	return Controls{Thrust: thrust, Turn: turn, Fire: fire}
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
	// enemySpawnRadius sits beyond the piloting view's visible corner so enemies
	// always arrive from offscreen.
	enemySpawnRadius   = 1600
	enemySpawnInterval = 30
)

func SpawnEnemy(target *Ship) (*Ship, Controller) {
	angle := rand.Float64() * 2 * math.Pi
	pos := rl.NewVector2(
		target.Position.X+float32(math.Cos(angle))*enemySpawnRadius,
		target.Position.Y+float32(math.Sin(angle))*enemySpawnRadius,
	)
	enemy := DefaultShip(pos)
	return enemy, NewEnemyAI(enemy, target)
}
