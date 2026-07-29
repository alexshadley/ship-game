package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Game holds all of the running game's state. It was previously a tangle of
// locals in main(); collecting it here lets the frame be split into two ordered
// passes — Update mutates state, Draw renders it — with main() just sequencing
// the two each frame.
type Game struct {
	// Window and render targets. winW/winH are the windowed size, restored when
	// leaving fullscreen. target holds the game world at gameWidth×gameHeight;
	// uiTarget holds the menu/designer at the logical UI resolution. src/uiSrc are
	// the (vertically flipped) source rects used to blit each to the window.
	winW, winH int32
	target     rl.RenderTexture2D
	uiTarget   rl.RenderTexture2D
	src        rl.Rectangle
	uiSrc      rl.Rectangle

	ship   *Ship
	camera rl.Camera2D

	asteroids []*Asteroid
	physics   *Physics
	particles *ParticleSystem

	player       Player
	scavenger    Scavenger
	repair       RepairTool
	spacewalking bool

	// User-controlled piloting zoom, nudged by the scroll wheel. Scroll changes
	// snap in instantly; only entering/exiting the ship eases the zoom smoothly
	// between the piloting and spacewalk framing (tracked by zoomEasing).
	pilotZoom        float32
	prevSpacewalking bool
	zoomEasing       bool

	// gameOver freezes the simulation once the astronaut dies (or the cockpit is
	// lost while aboard); only the render pass keeps running so the banner stays up.
	gameOver bool

	// stageTimer counts down from stageDuration while piloting; reaching zero ends
	// the round and drops the player into the shop.
	stageTimer float32

	godMode     bool
	aiDebug     bool
	autoWeapons bool

	enemies         []*Ship
	enemyAIs        []*EnemyAI
	enemySpawnTimer float32

	projectiles []*Projectile

	state    GameState
	menu     Menu
	designer *Designer

	// money and inventory are shared (by pointer / by reference) with the shop, so
	// buying and fitting parts there carries back into the running game.
	money     int
	inventory map[PartType]int

	// quit is raised by the pause menu's Quit action; main's loop breaks on it.
	quit bool
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

// spawnEnemy sends in another enemy from beyond the edge of the view and wires it
// into the physics sim and the live enemy list.
func (g *Game) spawnEnemy() {
	e, ai := SpawnEnemy(g.ship, g.playerThreats)
	g.physics.AddShip(e, ai)
	g.enemies = append(g.enemies, e)
	g.enemyAIs = append(g.enemyAIs, ai)
}

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

// Update advances the game state by one frame. It dispatches to the per-mode
// update for the current state — piloting, the pause menu, or the designer/shop —
// so the loop runs only the logic that mode needs. It performs no drawing; Draw
// renders the resulting state afterward.
func (g *Game) Update(dt float32) {
	// F11 toggles fullscreen at any time, returning to the windowed size.
	if rl.IsKeyPressed(rl.KeyF11) {
		toggleFullscreen(g.winW, g.winH)
	}

	switch g.state {
	case StatePlaying:
		g.updatePlaying(dt)
	case StateMenu:
		g.updateMenu()
	case StateDesigner, StateShop:
		g.updateDesigner()
	}
}

// updateMenu runs the pause menu: it feeds live debug state into the toggle rows,
// applies the chosen action, and lets Escape resume the game.
func (g *Game) updateMenu() {
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

// updateDesigner runs one input frame of the designer or shop and applies the
// resulting state change when it signals it's done. The designer/shop is now
// cleanly split into an input pass (Update) and a render pass (Draw), so its
// result is picked up here immediately rather than deferred to the next frame.
func (g *Game) updateDesigner() {
	if !g.designer.Update() {
		return
	}
	if g.state == StateShop {
		if g.designer.shop.embark {
			// Embark launches the next round, which heals and recentres the
			// refitted ship (its rebuilt body picks up the shop's edits).
			g.startRound()
		} else {
			// Backed out (Esc / Leave) — the shop edited the live ship's parts, so
			// regenerate its physics body, then return to the pause menu.
			g.physics.RebuildShipBody(g.ship)
			g.state = StateMenu
		}
	} else {
		g.state = StateMenu
	}
}

// updatePlaying advances the live simulation by one frame: input, physics,
// projectiles, enemies, timers, and the camera. Escape opens the pause menu and
// the stage clock, on expiry, opens the shop — both hand off to another mode and
// return early.
func (g *Game) updatePlaying(dt float32) {
	if rl.IsKeyPressed(rl.KeyEscape) {
		g.state = StateMenu
		return
	}

	// Count down the stage clock while piloting. When it empties the round is
	// over: head to the shop to refit before embarking on the next round.
	if !g.gameOver {
		g.stageTimer -= dt
		if g.stageTimer <= 0 {
			g.stageTimer = 0
			g.designer = NewShop(g.ship, &g.money, g.inventory)
			g.state = StateShop
			return
		}
	}

	// Once the astronaut is gone the world stops simulating; only the render pass
	// keeps running so the GAME OVER banner stays on screen.
	if !g.gameOver {
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
		g.particles.Update(dt)

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
