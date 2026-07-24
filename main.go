package main

import (
	"log"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	gameWidth  = 640
	gameHeight = 360

	windowWidth  = 1920
	windowHeight = 1080

	pilotingZoom      = 0.25
	spacewalkZoom     = 1.0
	zoomEaseSpeed     = 3.0
	cameraFollowSpeed = 20.0
)

func main() {
	rl.InitWindow(windowWidth, windowHeight, "Ship Game")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)
	// Escape opens the pause menu rather than closing the window (raylib's default).
	rl.SetExitKey(rl.KeyNull)

	target := rl.LoadRenderTexture(gameWidth, gameHeight)
	defer rl.UnloadRenderTexture(target)

	ship := LoadPlayerShip(rl.NewVector2(0, 0))
	if err := ship.Validate(); err != nil {
		log.Printf("player ship is invalid: %v", err)
	}

	// Target is nudged up half a cell because the ship's body extends forward of
	// the cockpit, so this reads as roughly centered.
	camera := rl.Camera2D{
		Target:   rl.NewVector2(0, -cellSize*0.5),
		Offset:   rl.NewVector2(gameWidth/2, gameHeight/2),
		Rotation: 0,
		Zoom:     pilotingZoom,
	}

	// Height is negative: OpenGL render textures have their origin at bottom-left,
	// so the source must be flipped vertically.
	src := rl.NewRectangle(0, 0, float32(target.Texture.Width), -float32(target.Texture.Height))
	dst := rl.NewRectangle(0, 0, windowWidth, windowHeight)

	asteroids := DefaultAsteroids()
	physics := NewPhysics(asteroids)
	particles := NewParticleSystem()

	var player Player
	var scavenger Scavenger
	spacewalking := false

	// Debug god mode (toggle with G): while on, the player's ship takes no damage,
	// so scavenging and other mechanics can be tested without dying.
	godMode := false

	physics.AddShip(ship, PilotInput{spacewalking: &spacewalking})

	// Enemies arrive from offscreen: one at the start, then another every
	// enemySpawnInterval seconds.
	var enemies []*Ship
	spawnEnemy := func() {
		e, ai := SpawnEnemy(ship, asteroids)
		physics.AddShip(e, ai)
		enemies = append(enemies, e)
	}
	spawnEnemy()
	enemySpawnTimer := float32(enemySpawnInterval)

	// Wire debug hooks: mark the player's ship and share the god-mode flag so the
	// damage handlers can spare it, then scatter salvageable parts to scavenge.
	physics.playerShip = ship
	physics.godMode = &godMode
	physics.SeedLooseParts(3)

	var projectiles []*Projectile

	state := StatePlaying
	var menu Menu
	var designer *Designer

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()

		switch state {
		case StatePlaying:
			if rl.IsKeyPressed(rl.KeyEscape) {
				state = StateMenu
			}
		case StateMenu:
			switch menu.Update() {
			case MenuResume:
				state = StatePlaying
			case MenuOpenDesigner:
				designer = NewDesigner()
				state = StateDesigner
			case MenuQuit:
				return
			}
			if state == StateMenu && rl.IsKeyPressed(rl.KeyEscape) {
				state = StatePlaying
			}
		}

		if state == StatePlaying {
			// Toggle debug god mode (player ship invincible) with G.
			if rl.IsKeyPressed(rl.KeyG) {
				godMode = !godMode
			}

			// Periodically send in another enemy from beyond the edge of the view.
			enemySpawnTimer -= dt
			if enemySpawnTimer <= 0 {
				spawnEnemy()
				enemySpawnTimer = enemySpawnInterval
			}

			if spacewalking {
				if player.NearCockpit(ship) && rl.IsKeyPressed(rl.KeyF) {
					// Drop whatever's in hand back into space before climbing aboard.
					scavenger.DropHeld(physics, &player)
					physics.DetachPlayer()
					spacewalking = false
				} else {
					// Grab, drag, and attach loose parts while out on the walk.
					scavenger.Update(physics, ship, &player, camera)
				}
			} else if rl.IsKeyPressed(rl.KeyF) {
				player.EjectFrom(ship)
				physics.AttachPlayer(&player)
				spacewalking = true
			}

			projectiles = append(projectiles, physics.Update(float64(dt), particles)...)
			particles.Update(dt)

			live := projectiles[:0]
			for _, pr := range projectiles {
				pr.Update(dt)
				if !pr.Expired() {
					live = append(live, pr)
				}
			}
			projectiles = live

			projectiles = physics.ResolveProjectiles(projectiles)

			targetZoom := float32(pilotingZoom)
			if spacewalking {
				targetZoom = spacewalkZoom
			}
			camera.Zoom += (targetZoom - camera.Zoom) * float32(1-math.Exp(-zoomEaseSpeed*float64(dt)))

			var followPoint rl.Vector2
			if spacewalking {
				followPoint = player.Position
			} else {
				followPoint = rl.NewVector2(ship.Position.X, ship.Position.Y-cellSize*0.5)
			}
			te := float32(1 - math.Exp(-cameraFollowSpeed*float64(dt)))
			camera.Target.X += (followPoint.X - camera.Target.X) * te
			camera.Target.Y += (followPoint.Y - camera.Target.Y) * te
		}

		if state == StatePlaying || state == StateMenu {
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
			for _, l := range physics.LooseParts() {
				l.Draw()
			}
			ship.Draw()
			if spacewalking {
				player.Draw()
				// The part being scavenged draws over the ship as a placement preview.
				scavenger.Draw(ship)
			}
			rl.EndMode2D()

			var minimapPlayer *Player
			hint := "F: spacewalk"
			if spacewalking {
				minimapPlayer = &player
				hint = "WASD: move  ·  hold LMB: grab part"
				if scavenger.Held != nil {
					hint = "R: rotate  ·  release LMB: attach"
				} else if player.NearCockpit(ship) {
					hint = "F: re-enter ship"
				}
			}
			DrawMinimap(ship, asteroids, enemies, physics.LooseParts(), minimapPlayer)
			rl.DrawText(hint, 6, gameHeight-14, 10, rl.RayWhite)
			// Debug indicator: show god-mode state and its toggle key in the corner.
			debugLabel := "G: god mode OFF"
			debugColor := rl.Gray
			if godMode {
				debugLabel = "G: GOD MODE ON"
				debugColor = rl.Lime
			}
			rl.DrawText(debugLabel, 6, 6, 10, debugColor)
			rl.EndTextureMode()
		}

		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		switch state {
		case StatePlaying, StateMenu:
			rl.DrawTexturePro(target.Texture, src, dst, rl.NewVector2(0, 0), 0, rl.White)
			if state == StateMenu {
				menu.Draw()
			}
		case StateDesigner:
			if designer.Frame() {
				state = StateMenu
			}
		}
		rl.EndDrawing()
	}
}

func emitExhaust(ship *Ship, controls Controls, particles *ParticleSystem) {
	if controls.Thrust > 0 {
		for _, src := range ship.EngineExhaustSources() {
			for i := 0; i < engineParticlesPerFrame; i++ {
				particles.Emit(src.Pos, src.Dir, ship.Velocity, engineExhaustColor)
			}
		}
	}

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
