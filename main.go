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

	target := rl.LoadRenderTexture(gameWidth, gameHeight)
	defer rl.UnloadRenderTexture(target)

	ship := DefaultShip(rl.NewVector2(0, 0))
	if err := ship.Validate(); err != nil {
		log.Printf("default ship is invalid: %v", err)
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
	spacewalking := false

	physics.AddShip(ship, PilotInput{spacewalking: &spacewalking})

	// Enemies arrive from offscreen: one at the start, then another every
	// enemySpawnInterval seconds.
	var enemies []*Ship
	spawnEnemy := func() {
		e, ai := SpawnEnemy(ship)
		physics.AddShip(e, ai)
		enemies = append(enemies, e)
	}
	spawnEnemy()
	enemySpawnTimer := float32(enemySpawnInterval)

	var projectiles []*Projectile

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()

		// Periodically send in another enemy from beyond the edge of the view.
		enemySpawnTimer -= dt
		if enemySpawnTimer <= 0 {
			spawnEnemy()
			enemySpawnTimer = enemySpawnInterval
		}

		if spacewalking {
			if player.NearCockpit(ship) && (rl.IsKeyPressed(rl.KeyLeftShift) || rl.IsKeyPressed(rl.KeyRightShift)) {
				physics.DetachPlayer()
				spacewalking = false
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
		}
		rl.EndMode2D()

		var minimapPlayer *Player
		hint := "F: spacewalk"
		if spacewalking {
			minimapPlayer = &player
			hint = "WASD: move"
			if player.NearCockpit(ship) {
				hint = "SHIFT: re-enter ship"
			}
		}
		DrawMinimap(ship, asteroids, enemies, minimapPlayer)
		rl.DrawText(hint, 6, gameHeight-14, 10, rl.RayWhite)
		rl.EndTextureMode()

		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		rl.DrawTexturePro(target.Texture, src, dst, rl.NewVector2(0, 0), 0, rl.White)
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
