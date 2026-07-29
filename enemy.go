package main

import (
	"math"
	"math/rand"
	"path/filepath"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// EnemyAI adapts the enemy AI decision (enemyControls) to the Controller
// interface the physics loop drives each frame. It holds the world references the
// AI reads and caches the goal heading from the last decision for the debug
// overlay.
type EnemyAI struct {
	ship   *Ship
	target *Ship

	// threats yields the world points this enemy's PDC should prefer over the
	// target ship each frame — the player's in-flight missiles and the exposed
	// spacewalking astronaut. It reads live game state, so it's queried per frame;
	// nil means no threats (the PDC stays on the target ship).
	threats func() []rl.Vector2

	// desired is the goal heading enemyControls steered toward on the last Controls
	// call, cached so the AI debug overlay can draw it. The ship rotates toward this
	// but thrusts along its current facing, so the two only coincide once it
	// finishes turning.
	desired float32
}

func NewEnemyAI(self, target *Ship, threats func() []rl.Vector2) *EnemyAI {
	return &EnemyAI{ship: self, target: target, threats: threats}
}

func (ai *EnemyAI) Controls(dt float32) Controls {
	var threats []rl.Vector2
	if ai.threats != nil {
		threats = ai.threats()
	}
	c, desired := enemyControls(AIWorld{Self: ai.ship, Target: ai.target, Threats: threats})
	ai.desired = desired
	return c
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
	maxRange := maxEngagementRange(ai.ship)
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

const (
	// enemySpawnRadius sits well beyond the piloting view's visible corner so
	// enemies always arrive from offscreen.
	enemySpawnRadius   = 5400
	enemySpawnInterval = 45
)

func SpawnEnemy(target *Ship, threats func() []rl.Vector2) (*Ship, *EnemyAI) {
	angle := rand.Float64() * 2 * math.Pi
	pos := rl.NewVector2(
		target.Position.X+float32(math.Cos(angle))*enemySpawnRadius,
		target.Position.Y+float32(math.Sin(angle))*enemySpawnRadius,
	)
	enemy := rosterEnemyShip(pos)
	return enemy, NewEnemyAI(enemy, target, threats)
}

// spawnEnemy sends in another enemy from beyond the edge of the view and wires it
// into the physics sim and the live enemy list.
func (g *Game) spawnEnemy() {
	e, ai := SpawnEnemy(g.ship, g.playerThreats)
	g.physics.AddShip(e, ai)
	g.enemies = append(g.enemies, e)
	g.enemyAIs = append(g.enemyAIs, ai)
}

// playerThreats lists the world points an enemy PDC should prefer over the
// player's hull each frame: the player's in-flight missiles and, while
// spacewalking, the exposed astronaut. Enemies retask their guns to swat these
// down once they drift into PDC range.
func (g *Game) playerThreats() []rl.Vector2 {
	var pts []rl.Vector2
	for _, pr := range g.projectiles {
		if pr.Kind == projectileMissile && pr.Owner == g.ship {
			pts = append(pts, pr.Position)
		}
	}
	if g.spacewalking && !g.player.Dead() {
		pts = append(pts, g.player.Position)
	}
	return pts
}

// enemyRoster lists the ship designs (files in ships/) that spawn as enemies.
// Add more entries here to widen the rotation; a random one is chosen per spawn.
var enemyRoster = []string{"raider.json", "l_ship.json", "lancelot.json", "fatboy.json", "railrotor2.json", "lorge.json", "ouch.json", "shieldy.json"}

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
