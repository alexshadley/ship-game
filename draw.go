package main

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Draw renders the current game state to the window. It performs no state
// mutation; Update already ran and settled every mode's state, so Draw just picks
// the render for the current mode. main() calls it after Update each frame.
func (g *Game) Draw() {
	// The piloting and pause-menu views both render the world into the low-res
	// target (the menu overlays a frozen frame of it).
	if g.state == StatePlaying || g.state == StateMenu {
		rl.BeginTextureMode(g.target)
		g.drawWorld()
		rl.EndTextureMode()
	}

	// The menu, designer, and shop overlays render into the UI texture (their own
	// resolution), which is blitted over the world below.
	switch g.state {
	case StateMenu:
		rl.BeginTextureMode(g.uiTarget)
		// Transparent so the frozen game frame shows through behind the overlay.
		rl.ClearBackground(rl.Blank)
		g.menu.Draw()
		rl.EndTextureMode()
	case StateDesigner, StateShop:
		rl.BeginTextureMode(g.uiTarget)
		g.designer.Draw()
		rl.EndTextureMode()
	}

	// The single letterboxed viewport frames both logical targets (both 16:9).
	vp := currentViewport()
	origin := rl.NewVector2(0, 0)
	rl.BeginDrawing()
	rl.ClearBackground(rl.Black)
	switch g.state {
	case StatePlaying, StateMenu:
		rl.DrawTexturePro(g.target.Texture, g.src, vp, origin, 0, rl.White)
		if g.state == StateMenu {
			rl.DrawTexturePro(g.uiTarget.Texture, g.uiSrc, vp, origin, 0, rl.White)
		}
	case StateDesigner, StateShop:
		rl.DrawTexturePro(g.uiTarget.Texture, g.uiSrc, vp, origin, 0, rl.White)
	}
	rl.EndDrawing()
}

// drawWorld renders the game world and its HUD into the low-res target: the play
// field, ships, projectiles, particles, minimap, and overlays. Called inside a
// BeginTextureMode block for both the piloting and pause-menu views.
func (g *Game) drawWorld() {
	rl.ClearBackground(rl.Black)
	rl.BeginMode2D(g.camera)
	// The boundary wall draws first, behind everything, framing the play area.
	DrawWorldBounds()
	for _, a := range g.asteroids {
		a.Draw()
	}
	// Exhaust draws before the ship so plumes read as coming out from under it.
	g.particles.Draw()
	for _, e := range g.enemies {
		e.Draw()
	}
	// AI debug overlay: engagement rings and steering-goal lines for each
	// enemy. Drawn under the ships' guns/projectiles but over the hulls.
	if g.aiDebug {
		for _, ai := range g.enemyAIs {
			ai.DrawDebug()
		}
	}
	for _, l := range g.physics.LooseParts() {
		l.Draw()
	}
	g.ship.Draw()
	for _, e := range g.enemies {
		e.DrawShields()
	}
	g.ship.DrawShields()
	// While piloting, overlay each PDC's firing arc, lighting the ones that
	// would bear on the cursor. Skip it on a spacewalk, when WASD/mouse drive
	// the astronaut rather than the guns.
	if !g.spacewalking {
		g.ship.DrawFiringArcs(mouseWorld(g.camera), g.autoWeapons)
	}
	// Projectiles draw after the ships so they read as flying over them.
	for _, pr := range g.projectiles {
		pr.Draw()
	}
	// Railgun beams draw over the ships as well — a fading white streak.
	drawBeams(g)
	if g.spacewalking {
		// The repair beam draws under the astronaut so it reads as coming from
		// the suit rather than over it.
		g.repair.Draw()
		g.player.Draw()
		// The part being scavenged draws over the ship as a placement preview.
		g.scavenger.Draw(g.ship, g.physics, &g.player)
	}
	// While piloting, mark where the PDCs are aiming. Hidden on spacewalks
	// (LMB grabs parts, not fire) and once the run is over.
	if g.state == StatePlaying && !g.spacewalking && !g.gameOver {
		drawFireCrosshair(g.camera)
	}
	rl.EndMode2D()

	var minimapPlayer *Player
	autoHint := "T: auto-turrets"
	if g.autoWeapons {
		autoHint = "T: auto-turrets (ON)"
	}
	hint := "hold LMB: fire PDCs  ·  " + autoHint + "  ·  F: spacewalk  ·  -/=: zoom"
	if g.spacewalking {
		minimapPlayer = &g.player
		hint = "WASD: move  ·  hold LMB: grab / pry off part  ·  hold RMB: repair"
		if g.physics.GrabbedPart() != nil {
			hint = "R: rotate  ·  release LMB: attach"
		} else if g.scavenger.prying {
			hint = "hold to pry part loose…"
		} else if g.player.NearCockpit(g.ship) {
			hint = "F: re-enter ship"
		}
	}
	DrawMinimap(g.ship, g.asteroids, g.enemies, g.projectiles, g.physics.LooseParts(), minimapPlayer)
	// Player's money, top-right, tucked just below the stage-timer bar so the
	// two don't overlap.
	moneyText := fmt.Sprintf("$%d", g.money)
	mw := rl.MeasureText(moneyText, 10)
	rl.DrawText(moneyText, gameWidth-mw-6, 16, 10, rl.Gold)
	rl.DrawText(hint, 6, gameHeight-14, 10, rl.RayWhite)
	// While out on a walk, show the astronaut's health as a top-left readout.
	if g.spacewalking {
		drawPlayerHealthHUD(g.player.Health)
	}
	// Stage countdown: a bar in the top-right that drains as time runs out.
	drawStageTimer(g.stageTimer / stageDuration)
	if g.state == StatePlaying && g.spacewalking && !g.gameOver {
		if p := hoveredWorldPart(g.physics, g.ship, g.enemies, g.camera); p != nil {
			m := rl.GetMousePosition()
			anchor := rl.NewVector2(m.X*float32(gameWidth)/windowWidth, m.Y*float32(gameHeight)/windowHeight)
			drawTooltip(partTooltipLines(p), anchor, 10, gameWidth, gameHeight)
		}
	}
	if g.gameOver {
		drawGameOver()
	}
}

// DrawBeams renders the lingering railgun beams as white lines that fade out over
// their lifetime, plus the red warm-up telegraph for any railgun still charging.
// Called from the main render pass in world space.
func drawBeams(g *Game) {
	p := g.physics
	// Warm-up telegraphs first, so a fired beam draws over the last frame of its
	// own charge. Each is two red lines parallel to the locked shot, offset to
	// either side and sliding together onto the beam line as the charge completes.
	for _, c := range p.charges {
		prog := c.Progress
		if prog < 0 {
			prog = 0
		} else if prog > 1 {
			prog = 1
		}
		// Endpoints of the beam line itself: the full reach the shot will travel.
		far := rl.NewVector2(c.Origin.X+c.Dir.X*railgunRange, c.Origin.Y+c.Dir.Y*railgunRange)
		// Perpendicular to the shot; the two lines sit at ±offset and close to 0.
		perpX, perpY := -c.Dir.Y, c.Dir.X
		offset := railgunTelegraphSpread * (1 - prog)
		// Brighten as the two lines close in so the imminent shot reads clearly.
		red := rl.NewColor(255, 40, 40, uint8(120+135*prog))
		for _, s := range []float32{offset, -offset} {
			a := rl.NewVector2(c.Origin.X+perpX*s, c.Origin.Y+perpY*s)
			b := rl.NewVector2(far.X+perpX*s, far.Y+perpY*s)
			rl.DrawLineEx(a, b, 3, red)
		}
	}

	for _, b := range p.beams {
		frac := b.TTL / railgunBeamDuration
		if frac < 0 {
			frac = 0
		}
		// A thick, straight, solid white line that fades out over its lifetime.
		white := rl.NewColor(255, 255, 255, uint8(255*frac))
		rl.DrawLineEx(b.Start, b.End, 6, white)
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
