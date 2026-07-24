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

	var projectiles []*Projectile
	// Time until the cannons may fire again while Shift is held.
	var fireCooldown float32

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()
		physics.Update(float64(dt))

		// Hold Shift to fire the cannons on a fixed interval. Releasing the key
		// resets the cooldown so the first shot on the next press is immediate.
		if rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift) {
			fireCooldown -= dt
			if fireCooldown <= 0 {
				projectiles = append(projectiles, ship.FireCannons()...)
				fireCooldown = cannonFireInterval
			}
		} else {
			fireCooldown = 0
		}

		// Advance projectiles and drop any that have expired.
		live := projectiles[:0]
		for _, pr := range projectiles {
			pr.Update(dt)
			if !pr.Expired() {
				live = append(live, pr)
			}
		}
		projectiles = live

		// Draw the game to the low-resolution render texture.
		rl.BeginTextureMode(target)
		rl.ClearBackground(rl.RayWhite)
		rl.BeginMode2D(camera)
		for _, a := range asteroids {
			a.Draw()
		}
		for _, pr := range projectiles {
			pr.Draw()
		}
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
