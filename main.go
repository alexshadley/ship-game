package main

import (
	"fmt"
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
	spacewalkZoom     = 0.7
	zoomEaseSpeed     = 3.0
	cameraFollowSpeed = 20.0

	// While piloting, the scroll wheel adjusts the zoom between these bounds.
	// pilotZoomMin frames the widest swath of the battlefield (most zoomed out);
	// pilotZoomMax matches the starting pilotingZoom, so the default view is the
	// most zoomed-in you can get and the wheel only pulls the camera back out.
	pilotZoomMin  = 0.1
	pilotZoomMax  = pilotingZoom
	pilotZoomStep = 0.005
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
	var repair RepairTool
	spacewalking := false

	// User-controlled piloting zoom, nudged by the scroll wheel. Scroll changes
	// snap in instantly; only entering/exiting the ship eases the zoom smoothly
	// between the piloting and spacewalk framing (tracked by zoomEasing).
	pilotZoom := float32(pilotingZoom)
	prevSpacewalking := spacewalking
	zoomEasing := false

	// Set once the astronaut's health runs out on a spacewalk. The simulation
	// freezes and a GAME OVER banner takes over the HUD.
	gameOver := false

	// Debug god mode (toggle with G): while on, the player's ship takes no damage,
	// so scavenging and other mechanics can be tested without dying.
	godMode := false

	physics.AddShip(ship, PilotInput{
		spacewalking: &spacewalking,
		player:       PlayerInput{camera: &camera, ship: ship},
	})

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

		// Once the astronaut is gone the world stops simulating; only the render
		// pass below keeps running so the GAME OVER banner stays on screen.
		if state == StatePlaying && !gameOver {
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
					scavenger.DropHeld(physics)
					physics.DetachPlayer()
					spacewalking = false
				} else {
					// Grab, drag, and attach loose parts while out on the walk.
					scavenger.Update(physics, ship, &player, camera, dt)
					// Right click mends nearby parts, but not mid-drag (both use the
					// mouse), so only when nothing is in hand.
					if physics.GrabbedPart() == nil {
						repair.Update(ship, &player, camera, dt)
					}
				}
			} else if rl.IsKeyPressed(rl.KeyF) {
				player.EjectFrom(ship)
				physics.AttachPlayer(&player)
				spacewalking = true
			}

			projectiles = append(projectiles, physics.Update(float64(dt), particles)...)
			particles.Update(dt)

			// physics.Update drops destroyed ships from the simulation; drop them
			// from our enemy list too so they stop drawing and vanish from the
			// minimap.
			liveEnemies := enemies[:0]
			for _, e := range enemies {
				if !e.Destroyed {
					liveEnemies = append(liveEnemies, e)
				}
			}
			enemies = liveEnemies

			live := projectiles[:0]
			for _, pr := range projectiles {
				pr.Update(dt)
				if pr.Expired() {
					// A missile that runs out of fuel goes off where it dies rather
					// than quietly disappearing.
					if pr.Kind == projectileMissile {
						physics.DetonateMissile(pr, particles)
					}
					continue
				}
				if pr.Kind == projectileMissile {
					pr.EmitExhaust(particles)
				}
				live = append(live, pr)
			}
			projectiles = live

			projectiles = physics.ResolveProjectiles(projectiles, particles)

			// A spacewalk that runs the astronaut out of health ends the game.
			if spacewalking && player.Dead() {
				gameOver = true
			}

			// Scrolling while piloting zooms the view; scrolling in and out is
			// disabled on a spacewalk, where the framing is fixed.
			if !spacewalking {
				if wheel := rl.GetMouseWheelMove(); wheel != 0 {
					pilotZoom = clamp(pilotZoom+wheel*pilotZoomStep, pilotZoomMin, pilotZoomMax)
				}
			}

			targetZoom := pilotZoom
			if spacewalking {
				targetZoom = spacewalkZoom
			}
			// Entering/exiting the ship changes the framing mode: ease smoothly
			// across that transition. Scroll-wheel zoom changes, by contrast, snap
			// in abruptly.
			if spacewalking != prevSpacewalking {
				zoomEasing = true
			}
			prevSpacewalking = spacewalking
			if zoomEasing {
				camera.Zoom += (targetZoom - camera.Zoom) * float32(1-math.Exp(-zoomEaseSpeed*float64(dt)))
				if math.Abs(float64(targetZoom-camera.Zoom)) < 0.001 {
					camera.Zoom = targetZoom
					zoomEasing = false
				}
			} else {
				camera.Zoom = targetZoom
			}

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
			for _, e := range enemies {
				e.Draw()
			}
			for _, l := range physics.LooseParts() {
				l.Draw()
			}
			ship.Draw()
			// While piloting, overlay each PDC's firing arc, lighting the ones that
			// would bear on the cursor. Skip it on a spacewalk, when WASD/mouse drive
			// the astronaut rather than the guns.
			if !spacewalking {
				ship.DrawFiringArcs(mouseWorld(camera))
			}
			// Projectiles draw after the ships so they read as flying over them.
			for _, pr := range projectiles {
				pr.Draw()
			}
			if spacewalking {
				// The repair beam draws under the astronaut so it reads as coming from
				// the suit rather than over it.
				repair.Draw()
				player.Draw()
				// The part being scavenged draws over the ship as a placement preview.
				scavenger.Draw(ship, physics, &player)
			}
			// While piloting, mark where the PDCs are aiming. Hidden on spacewalks
			// (LMB grabs parts, not fire) and once the run is over.
			if state == StatePlaying && !spacewalking && !gameOver {
				drawFireCrosshair(camera)
			}
			rl.EndMode2D()

			var minimapPlayer *Player
			hint := "hold LMB: fire PDCs  ·  F: spacewalk"
			if spacewalking {
				minimapPlayer = &player
				hint = "WASD: move  ·  hold LMB: grab / pry off part  ·  hold RMB: repair"
				if physics.GrabbedPart() != nil {
					hint = "R: rotate  ·  release LMB: attach"
				} else if scavenger.prying {
					hint = "hold to pry part loose…"
				} else if player.NearCockpit(ship) {
					hint = "F: re-enter ship"
				}
			}
			DrawMinimap(ship, asteroids, enemies, projectiles, physics.LooseParts(), minimapPlayer)
			rl.DrawText(hint, 6, gameHeight-14, 10, rl.RayWhite)
			// While out on a walk, show the astronaut's health as a top-left readout.
			if spacewalking {
				drawPlayerHealthHUD(player.Health)
			}
			// Debug indicator: show god-mode state and its toggle key in the corner.
			debugLabel := "G: god mode OFF"
			debugColor := rl.Gray
			if godMode {
				debugLabel = "G: GOD MODE ON"
				debugColor = rl.Lime
			}
			rl.DrawText(debugLabel, 6, 6, 10, debugColor)
			if gameOver {
				drawGameOver()
			}
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

// drawPlayerHealthHUD draws the astronaut's spacewalk health as a labeled bar in
// the top-left, below the debug indicator.
func drawPlayerHealthHUD(health float32) {
	frac := health / playerMaxHealth
	if frac < 0 {
		frac = 0
	}
	const barX, barY int32 = 6, 20
	const barWidth, barHeight int32 = 60, 6
	rl.DrawRectangle(barX, barY, barWidth, barHeight, rl.NewColor(0, 0, 0, 160))
	rl.DrawRectangle(barX, barY, int32(float32(barWidth)*frac), barHeight, healthColor(frac))
	rl.DrawRectangleLines(barX, barY, barWidth, barHeight, rl.NewColor(255, 255, 255, 120))
	rl.DrawText(fmt.Sprintf("HP %d", int(math.Ceil(float64(health)))), barX+barWidth+4, barY-2, 10, rl.RayWhite)
}

// drawGameOver dims the frozen scene and centers a GAME OVER banner over it.
func drawGameOver() {
	rl.DrawRectangle(0, 0, gameWidth, gameHeight, rl.NewColor(0, 0, 0, 140))
	const fontSize int32 = 40
	w := rl.MeasureText("GAME OVER", fontSize)
	rl.DrawText("GAME OVER", (gameWidth-w)/2, gameHeight/2-fontSize/2, fontSize, rl.Red)
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
