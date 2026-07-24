package main

import (
	"log"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	// Internal resolution the game is rendered at.
	gameWidth  = 640
	gameHeight = 360

	// Window size the internal resolution is scaled up to.
	windowWidth  = 1920
	windowHeight = 1080
)

func main() {
	rl.InitWindow(windowWidth, windowHeight, "Ship Game")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	// Render the game at a low internal resolution, then scale it up to fill
	// the window.
	target := rl.LoadRenderTexture(gameWidth, gameHeight)
	defer rl.UnloadRenderTexture(target)

	// Place the ship at the world origin; the camera keeps it centered.
	ship := DefaultShip(rl.NewVector2(0, 0))
	if err := ship.Validate(); err != nil {
		log.Printf("default ship is invalid: %v", err)
	}

	// Camera for the game view. Zoom 0.25 shows four times as much world as 1:1,
	// i.e. we're zoomed out 4x for the piloting view. Spacewalk mode will later
	// use zoom 1.0 (parts drawn at their full cellSize pixel size). The offset
	// centers the camera target on screen. Each frame Target is set to the ship's
	// position (nudged down one cell so the body, which extends behind the
	// cockpit, reads as roughly centered) so the camera follows the ship.
	camera := rl.Camera2D{
		Target:   rl.NewVector2(0, cellSize),
		Offset:   rl.NewVector2(gameWidth/2, gameHeight/2),
		Rotation: 0,
		Zoom:     0.25,
	}

	// Source is the render texture; it's flipped vertically because OpenGL
	// textures have their origin at the bottom-left.
	src := rl.NewRectangle(0, 0, float32(target.Texture.Width), -float32(target.Texture.Height))
	dst := rl.NewRectangle(0, 0, windowWidth, windowHeight)

	asteroids := DefaultAsteroids()
	physics := NewPhysics(asteroids)
	particles := NewParticleSystem()

	// The player and the AI enemies are all run by the physics from Controls; only
	// the source of those controls (keyboard vs. AI) differs.
	physics.AddShip(ship, PlayerInput{})
	enemies, enemyAIs := DefaultEnemies(ship)
	for i, e := range enemies {
		physics.AddShip(e, enemyAIs[i])
	}

	var projectiles []*Projectile

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()

		// Step the whole simulation. Every ship pulls its own Controls, thrusts and
		// turns under the same physics, spawns exhaust for whatever it's firing, and
		// any that fired their cannons return projectiles.
		projectiles = append(projectiles, physics.Update(float64(dt), particles)...)
		particles.Update(dt)

		// Advance projectiles and drop any that have expired.
		live := projectiles[:0]
		for _, pr := range projectiles {
			pr.Update(dt)
			if !pr.Expired() {
				live = append(live, pr)
			}
		}
		projectiles = live

		// Follow the ship: keep it centered (with the one-cell downward nudge).
		camera.Target = rl.NewVector2(ship.Position.X, ship.Position.Y+cellSize)

		// Draw the game to the low-resolution render texture.
		rl.BeginTextureMode(target)
		rl.ClearBackground(rl.Black)
		rl.BeginMode2D(camera)
		for _, a := range asteroids {
			a.Draw()
		}
		// Exhaust draws before the ship so plumes read as coming out from under it.
		particles.Draw()
		for _, pr := range projectiles {
			pr.Draw()
		}
		for _, e := range enemies {
			e.Draw()
		}
		// Loose parts (broken-off debris) drift in the same world frame as the ship.
		for _, l := range physics.LooseParts() {
			l.Draw()
		}
		ship.Draw()
		rl.EndMode2D()

		// Overlay the minimap in screen (texture) space, on top of the world.
		DrawMinimap(ship, asteroids, enemies)
		rl.EndTextureMode()

		// Scale the render texture up to the full window.
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		rl.DrawTexturePro(target.Texture, src, dst, rl.NewVector2(0, 0), 0, rl.White)
		rl.EndDrawing()
	}
}

// emitExhaust spawns exhaust particles for each engine and control thruster a
// ship is firing this frame, mirroring its Controls: Thrust fires the engines and
// Turn fires the control thrusters. It reads the same signals the physics uses to
// move the ship, so any ship — player or AI — plumes exactly when it maneuvers.
func emitExhaust(ship *Ship, controls Controls, particles *ParticleSystem) {
	if controls.Thrust > 0 {
		for _, src := range ship.EngineExhaustSources() {
			for i := 0; i < engineParticlesPerFrame; i++ {
				particles.Emit(src.Pos, src.Dir, ship.Velocity, engineExhaustColor)
			}
		}
	}

	// Turn's sign picks which thrusters fire; zero (or A+D cancelling) plumes none.
	turn := 0
	if controls.Turn < 0 {
		turn = -1
	} else if controls.Turn > 0 {
		turn = 1
	}
	if turn != 0 {
		for _, src := range ship.ControlThrusterExhaustSources(turn) {
			for i := 0; i < thrusterParticlesPerFrame; i++ {
				particles.Emit(src.Pos, src.Dir, ship.Velocity, thrusterExhaustColor)
			}
		}
	}
}
