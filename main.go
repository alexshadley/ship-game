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

	// Camera for the game view. Zoom 0.5 shows twice as much world as 1:1,
	// i.e. we're zoomed out 2x for the piloting view. Spacewalk mode will later
	// use zoom 1.0 (parts drawn at their full cellSize pixel size). The offset
	// centers the camera target on screen; nudge up one cell so the body (which
	// extends behind the cockpit) reads as roughly centered.
	camera := rl.Camera2D{
		Target:   rl.NewVector2(0, cellSize),
		Offset:   rl.NewVector2(gameWidth/2, gameHeight/2),
		Rotation: 0,
		Zoom:     0.5,
	}

	// Source is the render texture; it's flipped vertically because OpenGL
	// textures have their origin at the bottom-left.
	src := rl.NewRectangle(0, 0, float32(target.Texture.Width), -float32(target.Texture.Height))
	dst := rl.NewRectangle(0, 0, windowWidth, windowHeight)

	asteroids := DefaultAsteroids()
	physics := NewPhysics(ship)
	particles := NewParticleSystem()

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()
		physics.Update(float64(dt))

		// Spawn exhaust from whatever is firing this frame, then advance all
		// particles. Emission reads the same keys the physics uses for thrust.
		emitExhaust(ship, particles)
		particles.Update(dt)

		// Draw the game to the low-resolution render texture.
		rl.BeginTextureMode(target)
		rl.ClearBackground(rl.RayWhite)
		rl.BeginMode2D(camera)
		for _, a := range asteroids {
			a.Draw()
		}
		// Exhaust draws before the ship so plumes read as coming out from under it.
		particles.Draw()
		ship.Draw()
		rl.EndMode2D()
		rl.EndTextureMode()

		// Scale the render texture up to the full window.
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		rl.DrawTexturePro(target.Texture, src, dst, rl.NewVector2(0, 0), 0, rl.White)
		rl.EndDrawing()
	}
}

// emitExhaust spawns exhaust particles for each engine and control thruster that
// is firing this frame, mirroring the thrust controls: W fires the engines and
// A/D fire the control thrusters.
func emitExhaust(ship *Ship, particles *ParticleSystem) {
	if rl.IsKeyDown(rl.KeyW) {
		for _, src := range ship.EngineExhaustSources() {
			for i := 0; i < engineParticlesPerFrame; i++ {
				particles.Emit(src.Pos, src.Dir, ship.Velocity, engineExhaustColor)
			}
		}
	}

	// A and D held together cancel out to no turn, so emit no thruster plume.
	turn := 0
	if a, d := rl.IsKeyDown(rl.KeyA), rl.IsKeyDown(rl.KeyD); a && !d {
		turn = -1
	} else if d && !a {
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
