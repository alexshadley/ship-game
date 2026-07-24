package main

import (
	"log"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	// Internal resolution the game is rendered at.
	gameWidth  = 640
	gameHeight = 360

	// Window size the internal resolution is scaled up to.
	windowWidth  = 1920
	windowHeight = 1080

	// Camera zoom for each mode. Piloting is zoomed out 2x to show the surrounding
	// space; a spacewalk zooms all the way in to 1:1 (parts at full pixel size).
	pilotingZoom  = 0.5
	spacewalkZoom = 1.0
	// zoomEaseSpeed controls how quickly the camera zoom eases toward its target
	// when switching modes (~1 second to settle); cameraFollowSpeed does the same
	// for the follow target, kept snappy so piloting stays responsive.
	zoomEaseSpeed     = 3.0
	cameraFollowSpeed = 20.0
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

	// Camera for the game view. It starts zoomed out for piloting and eases toward
	// spacewalkZoom while the pilot is outside. The offset centers the follow
	// target on screen; the target is nudded up half a cell so the ship's body,
	// which now extends forward of the cockpit, reads as roughly centered.
	camera := rl.Camera2D{
		Target:   rl.NewVector2(0, -cellSize*0.5),
		Offset:   rl.NewVector2(gameWidth/2, gameHeight/2),
		Rotation: 0,
		Zoom:     pilotingZoom,
	}

	// Source is the render texture; it's flipped vertically because OpenGL
	// textures have their origin at the bottom-left.
	src := rl.NewRectangle(0, 0, float32(target.Texture.Width), -float32(target.Texture.Height))
	dst := rl.NewRectangle(0, 0, windowWidth, windowHeight)

	asteroids := DefaultAsteroids()
	physics := NewPhysics(ship, asteroids)

	var projectiles []*Projectile
	// Time until the cannons may fire again while Shift is held.
	var fireCooldown float32

	// Spacewalk state: when spacewalking, the pilot is out of the ship as a
	// free-floating astronaut (WASD to thrust) and the ship coasts uncontrolled.
	var player Player
	spacewalking := false

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()

		if spacewalking {
			// Press Shift while close to the cockpit to climb back in and resume
			// piloting.
			if player.NearCockpit(ship) && (rl.IsKeyPressed(rl.KeyLeftShift) || rl.IsKeyPressed(rl.KeyRightShift)) {
				physics.DetachPlayer()
				spacewalking = false
			}
		} else if rl.IsKeyPressed(rl.KeyF) {
			// Press F to pop out the back of the cockpit and begin a spacewalk.
			player.EjectFrom(ship)
			physics.AttachPlayer(&player)
			spacewalking = true
		}

		// The ship only takes control input while being piloted; the simulation
		// runs regardless so the ship and asteroids keep drifting during a spacewalk.
		physics.Update(float64(dt), !spacewalking)

		// Hold Shift to fire the cannons on a fixed interval (piloting only).
		// Releasing the key resets the cooldown so the first shot on the next press
		// is immediate.
		if !spacewalking && (rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)) {
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

		// Ease the zoom toward the current mode's level so entering/leaving a
		// spacewalk zooms gradually rather than snapping.
		targetZoom := float32(pilotingZoom)
		if spacewalking {
			targetZoom = spacewalkZoom
		}
		camera.Zoom += (targetZoom - camera.Zoom) * float32(1-math.Exp(-zoomEaseSpeed*float64(dt)))

		// Follow the ship while piloting, the astronaut while spacewalking. Easing
		// the target keeps the handoff between the two smooth.
		var followPoint rl.Vector2
		if spacewalking {
			followPoint = player.Position
		} else {
			followPoint = rl.NewVector2(ship.Position.X, ship.Position.Y-cellSize*0.5)
		}
		te := float32(1 - math.Exp(-cameraFollowSpeed*float64(dt)))
		camera.Target.X += (followPoint.X - camera.Target.X) * te
		camera.Target.Y += (followPoint.Y - camera.Target.Y) * te

		// Draw the game to the low-resolution render texture.
		rl.BeginTextureMode(target)
		rl.ClearBackground(rl.Black)
		rl.BeginMode2D(camera)
		for _, a := range asteroids {
			a.Draw()
		}
		for _, pr := range projectiles {
			pr.Draw()
		}
		ship.Draw()
		if spacewalking {
			player.Draw()
		}
		rl.EndMode2D()

		// Overlay the minimap and a control hint in screen (texture) space, on top
		// of the world.
		var minimapPlayer *Player
		hint := "F: spacewalk"
		if spacewalking {
			minimapPlayer = &player
			hint = "WASD: move"
			if player.NearCockpit(ship) {
				hint = "SHIFT: re-enter ship"
			}
		}
		DrawMinimap(ship, asteroids, minimapPlayer)
		rl.DrawText(hint, 6, gameHeight-14, 10, rl.RayWhite)
		rl.EndTextureMode()

		// Scale the render texture up to the full window.
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		rl.DrawTexturePro(target.Texture, src, dst, rl.NewVector2(0, 0), 0, rl.White)
		rl.EndDrawing()
	}
}
