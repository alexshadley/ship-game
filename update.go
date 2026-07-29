package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// startRound resets the game for a fresh round: the previous round's enemies and
// projectiles are cleared, the refitted ship is healed to full and set down
// upright and at rest in the middle of the field, a new enemy warps in, and the
// stage timer refills. The player's money and inventory carry over.
func (g *Game) startRound() {
	g.physics.ClearEnemyShips()
	g.enemies = nil
	g.enemyAIs = nil
	g.projectiles = nil
	// If the player embarked while out on a spacewalk, pull them back aboard
	// before the new ship is set down.
	if g.spacewalking {
		g.physics.DetachPlayer()
		g.spacewalking = false
	}
	g.ship.RestoreFullHealth()
	g.physics.ResetShip(g.ship, rl.NewVector2(0, 0))
	g.gameOver = false
	g.stageTimer = stageDuration
	g.enemySpawnTimer = enemySpawnInterval
	g.spawnEnemy()
	g.state = StatePlaying
}

// Update advances the game state by one frame: input, physics, projectiles,
// enemies, timers, camera, and state transitions. It performs no drawing; Draw
// renders the resulting state afterward.
func (g *Game) Update(dt float32) {
	// Apply any state transition the designer/shop requested on the previous
	// frame. The designer is an immediate-mode UI whose Frame() both reads input
	// and renders in one pass (run from Draw), so its result is picked up here at
	// the top of the next update rather than mid-draw.
	if g.designerDone {
		g.designerDone = false
		if g.state == StateShop {
			if g.designer.shop.embark {
				// Embark launches the next round, which heals and recentres the
				// refitted ship (its rebuilt body picks up the shop's edits).
				g.startRound()
			} else {
				// Backed out (Esc / Leave) — the shop edited the live ship's parts,
				// so regenerate its physics body, then return to the pause menu.
				g.physics.RebuildShipBody(g.ship)
				g.state = StateMenu
			}
		} else {
			g.state = StateMenu
		}
	}

	// F11 toggles fullscreen at any time, returning to the windowed size.
	if rl.IsKeyPressed(rl.KeyF11) {
		toggleFullscreen(g.winW, g.winH)
	}

	switch g.state {
	case StatePlaying:
		if rl.IsKeyPressed(rl.KeyEscape) {
			g.state = StateMenu
		}
	case StateMenu:
		// Feed live debug state in so the toggle rows render their ON/OFF label.
		g.menu.GodMode = g.godMode
		g.menu.AIDebug = g.aiDebug
		switch g.menu.Update() {
		case MenuResume:
			g.state = StatePlaying
		case MenuOpenDesigner:
			g.designer = NewDesigner()
			g.state = StateDesigner
		case MenuOpenShop:
			g.designer = NewShop(g.ship, &g.money, g.inventory)
			g.state = StateShop
		case MenuToggleGodMode:
			g.godMode = !g.godMode
		case MenuToggleAIDebug:
			g.aiDebug = !g.aiDebug
		case MenuQuit:
			g.quit = true
			return
		}
		if g.state == StateMenu && rl.IsKeyPressed(rl.KeyEscape) {
			g.state = StatePlaying
		}
	}

	// Count down the stage clock while piloting. When it empties the round is
	// over: head to the shop to refit before embarking on the next round.
	if g.state == StatePlaying && !g.gameOver {
		g.stageTimer -= dt
		if g.stageTimer <= 0 {
			g.stageTimer = 0
			g.designer = NewShop(g.ship, &g.money, g.inventory)
			g.state = StateShop
		}
	}

	// Once the astronaut is gone the world stops simulating; only the render pass
	// keeps running so the GAME OVER banner stays on screen. (When the timer just
	// expired the state is now StateShop, so this is skipped.)
	if g.state == StatePlaying && !g.gameOver {
		// Toggle debug god mode (player ship invincible) with G.
		if rl.IsKeyPressed(rl.KeyG) {
			g.godMode = !g.godMode
		}

		// T arms/disarms the auto-turrets: while on, PartAutoTurret mounts track
		// and fire on the nearest enemy without the manual trigger.
		if rl.IsKeyPressed(rl.KeyT) {
			g.autoWeapons = !g.autoWeapons
		}

		// Periodically send in another enemy from beyond the edge of the view.
		g.enemySpawnTimer -= dt
		if g.enemySpawnTimer <= 0 {
			g.spawnEnemy()
			g.enemySpawnTimer = enemySpawnInterval
		}
		if g.spacewalking {
			if g.player.NearCockpit(g.ship) && rl.IsKeyPressed(rl.KeyF) {
				// Drop whatever's in hand back into space before climbing aboard.
				g.scavenger.DropHeld(g.physics)
				g.physics.DetachPlayer()
				g.spacewalking = false
			} else {
				// Grab, drag, and attach loose parts while out on the walk.
				g.scavenger.Update(g.physics, g.ship, &g.player, g.camera, dt)
				// Right click mends nearby parts, but not mid-drag (both use the
				// mouse), so only when nothing is in hand.
				if g.physics.GrabbedPart() == nil {
					g.repair.Update(g.ship, &g.player, g.camera, dt)
				}
			}
		} else if rl.IsKeyPressed(rl.KeyF) {
			g.player.EjectFrom(g.ship)
			g.physics.AttachPlayer(&g.player)
			g.spacewalking = true
		}

		g.projectiles = append(g.projectiles, g.physics.Update(float64(dt), g.particles)...)

		// physics.Update drops destroyed ships from the simulation; drop them from
		// our enemy list too so they stop drawing and vanish from the minimap.
		liveEnemies := g.enemies[:0]
		liveAIs := g.enemyAIs[:0]
		for i, e := range g.enemies {
			if !e.Destroyed {
				liveEnemies = append(liveEnemies, e)
				liveAIs = append(liveAIs, g.enemyAIs[i])
			}
		}
		g.enemies = liveEnemies
		g.enemyAIs = liveAIs

		live := g.projectiles[:0]
		for _, pr := range g.projectiles {
			pr.Update(dt)
			if pr.Expired() {
				// A missile that runs out of fuel goes off where it dies rather
				// than quietly disappearing.
				if pr.Kind == projectileMissile {
					g.physics.DetonateMissile(pr, g.particles)
				}
				continue
			}
			if pr.Kind == projectileMissile {
				pr.EmitExhaust(g.particles)
				pr.EmitTrail(g.particles)
			}
			live = append(live, pr)
		}
		g.projectiles = live

		g.projectiles = g.physics.ResolveProjectiles(g.projectiles, g.particles)

		// A spacewalk that runs the astronaut out of health ends the game. Losing
		// the cockpit while still aboard (not out on a walk) is just as fatal.
		died := (g.spacewalking && g.player.Dead()) || (!g.spacewalking && g.ship.Destroyed)
		if died {
			g.gameOver = true
		}

		// Scrolling or holding the -/= keys while piloting zooms the view; both are
		// disabled on a spacewalk, where the framing is fixed. The keys are the
		// primary control for trackpads, which don't emit wheel events.
		if !g.spacewalking {
			if wheel := rl.GetMouseWheelMove(); wheel != 0 {
				g.pilotZoom = clamp(g.pilotZoom+wheel*pilotZoomStep, pilotZoomMin, pilotZoomMax)
			}
			if rl.IsKeyDown(rl.KeyEqual) || rl.IsKeyDown(rl.KeyKpAdd) {
				g.pilotZoom = clamp(g.pilotZoom+pilotZoomKeyRate*dt, pilotZoomMin, pilotZoomMax)
			}
			if rl.IsKeyDown(rl.KeyMinus) || rl.IsKeyDown(rl.KeyKpSubtract) {
				g.pilotZoom = clamp(g.pilotZoom-pilotZoomKeyRate*dt, pilotZoomMin, pilotZoomMax)
			}
		}

		targetZoom := g.pilotZoom
		if g.spacewalking {
			targetZoom = spacewalkZoom
		}
		// Entering/exiting the ship changes the framing mode: ease smoothly across
		// that transition. Scroll-wheel zoom changes, by contrast, snap in abruptly.
		if g.spacewalking != g.prevSpacewalking {
			g.zoomEasing = true
		}
		g.prevSpacewalking = g.spacewalking
		if g.zoomEasing {
			g.camera.Zoom += (targetZoom - g.camera.Zoom) * float32(1-math.Exp(-zoomEaseSpeed*float64(dt)))
			if math.Abs(float64(targetZoom-g.camera.Zoom)) < 0.001 {
				g.camera.Zoom = targetZoom
				g.zoomEasing = false
			}
		} else {
			g.camera.Zoom = targetZoom
		}

		var followPoint rl.Vector2
		if g.spacewalking {
			followPoint = g.player.Position
		} else {
			followPoint = rl.NewVector2(g.ship.Position.X, g.ship.Position.Y-cellSize*0.5)
		}
		te := float32(1 - math.Exp(-cameraFollowSpeed*float64(dt)))
		g.camera.Target.X += (followPoint.X - g.camera.Target.X) * te
		g.camera.Target.Y += (followPoint.Y - g.camera.Target.Y) * te
	}
}
