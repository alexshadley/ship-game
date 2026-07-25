package main

import (
	"flag"
	"fmt"
	"log"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	gameWidth  = 640
	gameHeight = 360

	// Logical UI resolution the menu and designer lay out against. The actual
	// window can be any size (see the -res/-fullscreen flags); the UI renders into
	// a texture at this resolution and is scaled to fit, so this stays fixed.
	windowWidth  = 1920
	windowHeight = 1080

	pilotingZoom      = 0.15
	spacewalkZoom     = 0.7
	zoomEaseSpeed     = 3.0
	cameraFollowSpeed = 20.0

	// While piloting, the scroll wheel and the -/= keys adjust the zoom between
	// these bounds. The default (pilotingZoom) sits between them so the view can be
	// pulled both further out (toward pilotZoomMin) and in (toward pilotZoomMax).
	pilotZoomMin  = 0.05
	pilotZoomMax  = 0.4
	pilotZoomStep = 0.01
	// Per-second zoom rate while a zoom key is held (trackpad users have no wheel).
	pilotZoomKeyRate = 0.25

	// A stage lasts this many seconds; survive to the end to clear it.
	stageDuration = 180.0
)

func main() {
	// Window sizing: -res WxH picks an explicit window size, -fullscreen starts
	// fullscreen at the monitor's native resolution. With neither, the window is
	// fitted to the current monitor so it never opens larger than the screen.
	resFlag := flag.String("res", "", "window resolution as WxH (e.g. 1280x720); default fits the monitor")
	fullscreenFlag := flag.Bool("fullscreen", false, "start in fullscreen at the monitor's native resolution")
	flag.Parse()

	// Resolve the windowed size: explicit -res, else a fit to the current monitor.
	// Probe the monitor with a throwaway window first, then create the real window
	// once at the final size. Resizing a window after InitWindow desyncs the GL
	// framebuffer from the drawable on HiDPI Macs and renders the scene into a
	// corner, so we deliberately avoid SetWindowSize.
	rl.InitWindow(windowWidth, windowHeight, "Ship Game")
	mon := rl.GetCurrentMonitor()
	monW, monH := rl.GetMonitorWidth(mon), rl.GetMonitorHeight(mon)
	winW, winH := fitToMonitor(int32(monW), int32(monH))
	if w, h, ok := parseResolution(*resFlag); ok {
		winW, winH = w, h
	} else if *resFlag != "" {
		log.Printf("ignoring invalid -res %q; expected WxH like 1280x720", *resFlag)
	}
	rl.CloseWindow()

	rl.InitWindow(winW, winH, "Ship Game")
	defer rl.CloseWindow()
	// Center the window on the monitor.
	rl.SetWindowPosition((monW-int(winW))/2, (monH-int(winH))/2)
	if *fullscreenFlag {
		toggleFullscreen(winW, winH)
	}

	rl.SetTargetFPS(60)
	// Escape opens the pause menu rather than closing the window (raylib's default).
	rl.SetExitKey(rl.KeyNull)

	target := rl.LoadRenderTexture(gameWidth, gameHeight)
	defer rl.UnloadRenderTexture(target)

	// The menu and designer draw at the logical UI resolution into this texture,
	// which is then scaled to the window alongside the game world.
	uiTarget := rl.LoadRenderTexture(windowWidth, windowHeight)
	defer rl.UnloadRenderTexture(uiTarget)

	ship, err := LoadPlayerShip(rl.NewVector2(0, 0))
	if err != nil {
		log.Fatalf("failed to load player ship: %v", err)
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
	// so the source must be flipped vertically. The destination (viewport) is
	// computed each frame from the current window size.
	src := rl.NewRectangle(0, 0, float32(target.Texture.Width), -float32(target.Texture.Height))
	uiSrc := rl.NewRectangle(0, 0, float32(uiTarget.Texture.Width), -float32(uiTarget.Texture.Height))

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

	// Counts down from stageDuration while piloting. Surviving until it hits zero
	// ends the round and drops the player into the shop to refit before embarking
	// on the next one.
	stageTimer := float32(stageDuration)

	// Debug god mode (toggle with G or from the pause menu): while on, the player's
	// ship takes no damage, so scavenging and other mechanics can be tested without
	// dying.
	godMode := false

	// AI debug mode (toggle from the pause menu): overlays each enemy's engagement
	// ranges and the goal heading its AI is steering toward.
	aiDebug := false

	physics.AddShip(ship, PilotInput{
		spacewalking: &spacewalking,
		player:       PlayerInput{camera: &camera, ship: ship},
	})

	var projectiles []*Projectile

	// playerThreats lists the world points an enemy PDC should prefer over the
	// player's hull each frame: the player's in-flight missiles and, while
	// spacewalking, the exposed astronaut. Enemies retask their guns to swat these
	// down once they drift into PDC range.
	playerThreats := func() []rl.Vector2 {
		var pts []rl.Vector2
		for _, pr := range projectiles {
			if pr.Kind == projectileMissile && pr.Owner == ship {
				pts = append(pts, pr.Position)
			}
		}
		if spacewalking && !player.Dead() {
			pts = append(pts, player.Position)
		}
		return pts
	}

	// Enemies arrive from offscreen: one at the start, then another every
	// enemySpawnInterval seconds.
	var enemies []*Ship
	var enemyAIs []*EnemyAI
	spawnEnemy := func() {
		e, ai := SpawnEnemy(ship, playerThreats)
		physics.AddShip(e, ai)
		enemies = append(enemies, e)
		enemyAIs = append(enemyAIs, ai)
	}
	spawnEnemy()
	enemySpawnTimer := float32(enemySpawnInterval)

	// Wire debug hooks: mark the player's ship and share the god-mode flag so the
	// damage handlers can spare it, then scatter salvageable parts to scavenge.
	physics.playerShip = ship
	physics.godMode = &godMode
	physics.SeedLooseParts(3)

	state := StatePlaying
	var menu Menu
	var designer *Designer

	// The player's wallet and the parts they own. Both are shared with the shop
	// (by pointer / by reference), so buying and fitting parts there carries back
	// into the running game. Inventory holds only unplaced parts — bought or pulled
	// off the ship — so it starts empty (the starting ship is already assembled) and
	// does not persist between games. Money starts at zero; sell parts to earn some.
	money := 0
	inventory := map[PartType]int{}

	// startRound resets the game for a fresh round: the previous round's enemies and
	// projectiles are cleared, the refitted ship is healed to full and set down
	// upright and at rest in the middle of the field, a new enemy warps in, and the
	// stage timer refills. The player's money and inventory carry over.
	startRound := func() {
		physics.ClearEnemyShips()
		enemies = nil
		enemyAIs = nil
		projectiles = nil
		// If the player embarked while out on a spacewalk, pull them back aboard
		// before the new ship is set down.
		if spacewalking {
			physics.DetachPlayer()
			spacewalking = false
		}
		ship.RestoreFullHealth()
		physics.ResetShip(ship, rl.NewVector2(0, 0))
		gameOver = false
		stageTimer = stageDuration
		enemySpawnTimer = enemySpawnInterval
		spawnEnemy()
		state = StatePlaying
	}

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()

		// F11 toggles fullscreen at any time, returning to the windowed size.
		if rl.IsKeyPressed(rl.KeyF11) {
			toggleFullscreen(winW, winH)
		}

		switch state {
		case StatePlaying:
			if rl.IsKeyPressed(rl.KeyEscape) {
				state = StateMenu
			}
		case StateMenu:
			// Feed live debug state in so the toggle rows render their ON/OFF label.
			menu.GodMode = godMode
			menu.AIDebug = aiDebug
			switch menu.Update() {
			case MenuResume:
				state = StatePlaying
			case MenuOpenDesigner:
				designer = NewDesigner()
				state = StateDesigner
			case MenuOpenShop:
				designer = NewShop(ship, &money, inventory)
				state = StateShop
			case MenuToggleGodMode:
				godMode = !godMode
			case MenuToggleAIDebug:
				aiDebug = !aiDebug
			case MenuQuit:
				return
			}
			if state == StateMenu && rl.IsKeyPressed(rl.KeyEscape) {
				state = StatePlaying
			}
		}

		// Count down the stage clock while piloting. When it empties the round is
		// over: head to the shop to refit before embarking on the next round.
		if state == StatePlaying && !gameOver {
			stageTimer -= dt
			if stageTimer <= 0 {
				stageTimer = 0
				designer = NewShop(ship, &money, inventory)
				state = StateShop
			}
		}

		// Once the astronaut is gone the world stops simulating; only the render
		// pass below keeps running so the GAME OVER banner stays on screen. (When
		// the timer just expired the state is now StateShop, so this is skipped.)
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
			liveAIs := enemyAIs[:0]
			for i, e := range enemies {
				if !e.Destroyed {
					liveEnemies = append(liveEnemies, e)
					liveAIs = append(liveAIs, enemyAIs[i])
				}
			}
			enemies = liveEnemies
			enemyAIs = liveAIs

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
			// Losing the cockpit while still aboard (not out on a walk) is just as
			// fatal.
			died := (spacewalking && player.Dead()) || (!spacewalking && ship.Destroyed)
			if died {
				gameOver = true
			}

			// Scrolling or holding the -/= keys while piloting zooms the view; both
			// are disabled on a spacewalk, where the framing is fixed. The keys are
			// the primary control for trackpads, which don't emit wheel events.
			if !spacewalking {
				if wheel := rl.GetMouseWheelMove(); wheel != 0 {
					pilotZoom = clamp(pilotZoom+wheel*pilotZoomStep, pilotZoomMin, pilotZoomMax)
				}
				if rl.IsKeyDown(rl.KeyEqual) || rl.IsKeyDown(rl.KeyKpAdd) {
					pilotZoom = clamp(pilotZoom+pilotZoomKeyRate*dt, pilotZoomMin, pilotZoomMax)
				}
				if rl.IsKeyDown(rl.KeyMinus) || rl.IsKeyDown(rl.KeyKpSubtract) {
					pilotZoom = clamp(pilotZoom-pilotZoomKeyRate*dt, pilotZoomMin, pilotZoomMax)
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
			// The boundary wall draws first, behind everything, framing the play area.
			DrawWorldBounds()
			for _, a := range asteroids {
				a.Draw()
			}
			// Exhaust draws before the ship so plumes read as coming out from under it.
			particles.Draw()
			for _, e := range enemies {
				e.Draw()
			}
			// AI debug overlay: engagement rings and steering-goal lines for each
			// enemy. Drawn under the ships' guns/projectiles but over the hulls.
			if aiDebug {
				for _, ai := range enemyAIs {
					ai.DrawDebug()
				}
			}
			for _, l := range physics.LooseParts() {
				l.Draw()
			}
			ship.Draw()
			for _, e := range enemies {
				e.DrawShields()
			}
			ship.DrawShields()
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
			// Railgun beams draw over the ships as well — a fading white streak.
			physics.DrawBeams()
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
			hint := "hold LMB: fire PDCs  ·  F: spacewalk  ·  -/=: zoom"
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
			// Player's money, top-right, tucked just below the stage-timer bar so the
			// two don't overlap.
			moneyText := fmt.Sprintf("$%d", money)
			mw := rl.MeasureText(moneyText, 10)
			rl.DrawText(moneyText, gameWidth-mw-6, 16, 10, rl.Gold)
			rl.DrawText(hint, 6, gameHeight-14, 10, rl.RayWhite)
			// While out on a walk, show the astronaut's health as a top-left readout.
			if spacewalking {
				drawPlayerHealthHUD(player.Health)
			}
			// Stage countdown: a bar in the top-right that drains as time runs out.
			drawStageTimer(stageTimer / stageDuration)
			if state == StatePlaying && spacewalking && !gameOver {
				if p := hoveredWorldPart(physics, ship, enemies, camera); p != nil {
					m := rl.GetMousePosition()
					anchor := rl.NewVector2(m.X*float32(gameWidth)/windowWidth, m.Y*float32(gameHeight)/windowHeight)
					drawTooltip(partTooltipLines(p), anchor, 10, gameWidth, gameHeight)
				}
			}
			if gameOver {
				drawGameOver()
			}
			rl.EndTextureMode()
		}

		// The menu and designer draw into the UI texture (input + draw), outside
		// BeginDrawing so their render-texture pass doesn't nest inside it. Any
		// state transition they request is applied after this frame is presented.
		designerDone := false
		switch state {
		case StateMenu:
			rl.BeginTextureMode(uiTarget)
			// Transparent so the frozen game frame shows through behind the overlay.
			rl.ClearBackground(rl.Blank)
			menu.Draw()
			rl.EndTextureMode()
		case StateDesigner, StateShop:
			rl.BeginTextureMode(uiTarget)
			designerDone = designer.Frame()
			rl.EndTextureMode()
		}

		// The single letterboxed viewport frames both logical targets (both 16:9).
		vp := currentViewport()
		origin := rl.NewVector2(0, 0)
		rl.BeginDrawing()
		rl.ClearBackground(rl.Black)
		switch state {
		case StatePlaying, StateMenu:
			rl.DrawTexturePro(target.Texture, src, vp, origin, 0, rl.White)
			if state == StateMenu {
				rl.DrawTexturePro(uiTarget.Texture, uiSrc, vp, origin, 0, rl.White)
			}
		case StateDesigner, StateShop:
			rl.DrawTexturePro(uiTarget.Texture, uiSrc, vp, origin, 0, rl.White)
		}
		rl.EndDrawing()

		if designerDone {
			if state == StateShop {
				if designer.shop.embark {
					// Embark launches the next round, which heals and recentres the
					// refitted ship (its rebuilt body picks up the shop's edits).
					startRound()
				} else {
					// Backed out (Esc / Leave) — the shop edited the live ship's parts,
					// so regenerate its physics body, then return to the pause menu.
					physics.RebuildShipBody(ship)
					state = StateMenu
				}
			} else {
				state = StateMenu
			}
		}
	}
}

// drawPlayerHealthHUD draws the astronaut's spacewalk health as a labeled bar in
// the top-left corner.
func drawPlayerHealthHUD(health float32) {
	frac := health / playerMaxHealth
	if frac < 0 {
		frac = 0
	}
	const barX, barY int32 = 6, 6
	const barWidth, barHeight int32 = 60, 6
	rl.DrawRectangle(barX, barY, barWidth, barHeight, rl.NewColor(0, 0, 0, 160))
	rl.DrawRectangle(barX, barY, int32(float32(barWidth)*frac), barHeight, healthColor(frac))
	rl.DrawRectangleLines(barX, barY, barWidth, barHeight, rl.NewColor(255, 255, 255, 120))
	rl.DrawText(fmt.Sprintf("HP %d", int(math.Ceil(float64(health)))), barX+barWidth+4, barY-2, 10, rl.RayWhite)
}

// worldBoundColor tints the world boundary — the wall drawn in the field and its
// dotted frame on the minimap — a muted red reading as the edge of the safe area.
var worldBoundColor = rl.NewColor(210, 70, 70, 160)

// DrawWorldBounds outlines the square play area (see worldBound) so the otherwise
// invisible walls that pen ships, asteroids, and debris in are visible. Drawn in
// world space, so it must be called inside BeginMode2D. The line is thick in world
// units so it stays legible at the zoomed-out piloting view.
func DrawWorldBounds() {
	rl.DrawRectangleLinesEx(
		rl.NewRectangle(-worldBound, -worldBound, worldBound*2, worldBound*2),
		16, worldBoundColor,
	)
}

// drawStageTimer draws the stage countdown as a small bar in the top-right of the
// HUD. frac is the fraction of time remaining (1 at the start, 0 when it ends);
// the filled portion drains from the right as the clock winds down.
func drawStageTimer(frac float32) {
	if frac < 0 {
		frac = 0
	}
	const barWidth, barHeight int32 = 90, 6
	const barY, barMargin int32 = 6, 6
	barX := gameWidth - barWidth - barMargin
	rl.DrawRectangle(barX, barY, barWidth, barHeight, rl.NewColor(0, 0, 0, 160))
	rl.DrawRectangle(barX, barY, int32(float32(barWidth)*frac), barHeight, rl.NewColor(120, 180, 255, 255))
	rl.DrawRectangleLines(barX, barY, barWidth, barHeight, rl.NewColor(255, 255, 255, 120))
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
