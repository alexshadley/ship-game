package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// scavengeSnapRange is how close, in world pixels, the dragged part's center must
// be to one of the ship's part centers for placement to snap onto the ship's
// grid. Beyond this the part just trails the cursor freely.
const scavengeSnapRange = cellSize * 2

// scavengePryDuration is how long, in seconds, the player must hold left click
// over one of the ship's own parts before it pries loose into their hands.
const scavengePryDuration = 1.0

// Scavenger is the spacewalk part-scavenging tool: while outside the ship the
// player holds left click over a loose part to pick it up, drags it to the ship
// (where it snaps to the grid), presses R to rotate it, and releases to attach.
// It owns the currently held part and the placement it resolved this frame, which
// Draw renders as a ghost preview (red when the spot is invalid).
type Scavenger struct {
	// Held is the part being dragged, or nil when nothing is grabbed.
	Held *Part

	// prying tracks the dwell-to-detach interaction: while the player holds left
	// click over one of the ship's own parts, pryTimer accumulates against
	// scavengePryDuration; on completion that part detaches into Held. pryCoord is
	// the ship cell currently being pried.
	prying   bool
	pryCoord GridCoord
	pryTimer float32

	// The placement resolved by the most recent Update, consumed by Draw and by
	// the release handler. When snapped, the part locks to snapCoord on the ship;
	// otherwise it floats at pos. valid is only meaningful while snapped.
	snapped   bool
	snapCoord GridCoord
	valid     bool
	pos       rl.Vector2 // world center of the free-floating preview
	baseAngle float32    // world rotation of the preview's frame (radians)
}

// mouseWorld converts the OS mouse position into world coordinates, accounting
// for the render texture being drawn at gameWidth×gameHeight and scaled up to the
// window, then unprojecting through the game camera.
func mouseWorld(camera rl.Camera2D) rl.Vector2 {
	m := rl.GetMousePosition()
	tex := rl.NewVector2(m.X*float32(gameWidth)/windowWidth, m.Y*float32(gameHeight)/windowHeight)
	return rl.GetScreenToWorld2D(tex, camera)
}

// Update runs one frame of the scavenging interaction: grab a loose part on left
// press, resolve where a held part would land (snapping to the ship's grid when
// near it), rotate it 90° on R, and on release either attach it (valid snap) or
// drop it back into the debris field. It only does anything while spacewalking.
func (sc *Scavenger) Update(physics *Physics, ship *Ship, player *Player, camera rl.Camera2D, dt float32) {
	wp := mouseWorld(camera)

	if sc.Held == nil {
		// A press grabs a loose part outright; with none under the cursor, fall
		// through to the dwell-to-pry interaction against the ship's own parts.
		if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
			if grabbed := physics.GrabLoosePart(wp); grabbed != nil {
				sc.Held = grabbed
				sc.prying = false
				return
			}
		}
		sc.updatePry(physics, ship, wp, dt)
		return
	}

	// Press R to rotate the held part 90° clockwise (cycling its facing).
	if rl.IsKeyPressed(rl.KeyR) {
		sc.Held.Facing = (sc.Held.Facing + 1) % 4
	}

	sc.resolvePlacement(ship, wp)

	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		sc.release(physics, ship, player)
	}
}

// updatePry advances the dwell-to-detach interaction while nothing is held.
// Holding left click over one of the ship's own (non-cockpit) parts for
// scavengePryDuration pries it loose into Held; that removal can sever other
// parts from the cockpit, which break off as debris. Moving to a different cell,
// pointing off the ship, or releasing resets the timer.
func (sc *Scavenger) updatePry(physics *Physics, ship *Ship, wp rl.Vector2, dt float32) {
	if !rl.IsMouseButtonDown(rl.MouseButtonLeft) {
		sc.prying = false
		return
	}

	c := ship.gridAtWorld(wp)
	if part, ok := ship.Parts[c]; !ok || part.Type == PartCockpit {
		sc.prying = false
		return
	}

	if !sc.prying || sc.pryCoord != c {
		sc.prying = true
		sc.pryCoord = c
		sc.pryTimer = 0
	}

	sc.pryTimer += dt
	if sc.pryTimer >= scavengePryDuration {
		sc.Held = physics.DetachPart(ship, c)
		sc.prying = false
		sc.pryTimer = 0
		if sc.Held != nil {
			sc.resolvePlacement(ship, wp)
		}
	}
}

// resolvePlacement decides where the held part would land this frame: snapped to
// the nearest ship cell when the cursor is within snap range of the ship (with a
// validity check for red feedback), or trailing the cursor otherwise. Results are
// stored for Draw and release.
func (sc *Scavenger) resolvePlacement(ship *Ship, wp rl.Vector2) {
	sc.snapped = false
	sc.pos = wp
	sc.baseAngle = 0 // screen-aligned while free; only the facing indicator turns

	near := false
	for c := range ship.Parts {
		center := ship.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		if dist(wp, center) <= scavengeSnapRange {
			near = true
			break
		}
	}
	if !near {
		return
	}

	sc.snapped = true
	sc.snapCoord = ship.gridAtWorld(wp)
	sc.valid = ship.canAttachAt(sc.snapCoord)
	sc.pos = ship.worldPoint(float32(sc.snapCoord.X)*cellSize, float32(sc.snapCoord.Y)*cellSize)
	sc.baseAngle = ship.Direction
}

// release ends a drag: attach the part when it's snapped to a valid cell,
// otherwise return it to the loose-part field where the preview sat (moving with
// the astronaut so it stays in reach). Clears the held part either way.
func (sc *Scavenger) release(physics *Physics, ship *Ship, player *Player) {
	if sc.snapped && sc.valid {
		physics.AttachPart(ship, sc.snapCoord, sc.Held)
	} else {
		physics.DropPart(sc.Held, sc.pos, sc.baseAngle, player.Velocity)
	}
	sc.Held = nil
}

// DropHeld returns any held part to the debris field without placing it, used
// when the player re-enters the ship mid-drag. No-op when nothing is held.
func (sc *Scavenger) DropHeld(physics *Physics, player *Player) {
	if sc.Held == nil {
		return
	}
	physics.DropPart(sc.Held, sc.pos, sc.baseAngle, player.Velocity)
	sc.Held = nil
}

// comNeutralColor marks the center of mass when there are no engines to balance;
// balancedColor / unbalancedColor mark the center of mass and the engine thrust
// line green when the thrust passes close enough to the center of mass to fly
// straight (within engineStraightTolerance) and red when it doesn't.
var (
	comNeutralColor = rl.NewColor(60, 60, 60, 255)
	balancedColor   = rl.NewColor(0, 180, 70, 255)
	unbalancedColor = rl.NewColor(220, 50, 40, 255)
)

// Draw renders the held part as a ghost preview at its resolved placement: normal
// color when it's free or snapped to a valid cell, red when snapped to an invalid
// one. While a part is held it also draws the ship's center of mass and engine
// thrust line, colored green when the thrust is balanced through the center of
// mass and red otherwise. When snapped to a valid cell it evaluates the ship as it
// *would* be with the part attached (and ghosts the current balance point so the
// shift is visible), letting the pilot judge the spot before releasing. It draws
// nothing when no part is held.
func (sc *Scavenger) Draw(ship *Ship) {
	if sc.Held == nil {
		if sc.prying {
			sc.drawPryProgress(ship)
		}
		return
	}
	fill := partSpecs[sc.Held.Type].color
	if sc.snapped && !sc.valid {
		fill = rl.Red
	}
	drawPartColored(sc.pos, sc.baseAngle, sc.Held, fill)

	// Evaluate the previewed configuration when snapped to a valid cell, otherwise
	// the ship as it stands.
	var extra *Part
	var coord GridCoord
	if sc.snapped && sc.valid {
		extra = sc.Held
		coord = sc.snapCoord
	}

	com := centerOfMass(ship.Parts, extra, coord)
	tl := engineThrustAbout(ship.Parts, extra, coord, com)

	col := comNeutralColor
	if tl.ok {
		if tl.offset <= engineStraightTolerance {
			col = balancedColor
		} else {
			col = unbalancedColor
		}
		half := thrustLineHalfLength(ship.Parts, extra, coord, tl.point)
		drawThrustLine(ship, tl, half, col)
	}

	// Ghost the current center of mass while previewing so the shift is visible.
	if extra != nil {
		cur := ship.CenterOfMass()
		drawCenterOfMassGhost(ship.worldPoint(cur.X, cur.Y))
	}
	drawCenterOfMassMarker(ship.worldPoint(com.X, com.Y), col)
}

// drawPryProgress rings the part being pried with an arc that fills as the hold
// approaches scavengePryDuration, so the player sees the detach charging up.
func (sc *Scavenger) drawPryProgress(ship *Ship) {
	center := ship.worldPoint(float32(sc.pryCoord.X)*cellSize, float32(sc.pryCoord.Y)*cellSize)
	frac := sc.pryTimer / scavengePryDuration
	if frac > 1 {
		frac = 1
	}
	const r = cellSize * 0.45
	rl.DrawCircleSector(center, r, -90, -90+360*frac, 24, rl.NewColor(255, 210, 40, 150))
	rl.DrawCircleLines(int32(center.X), int32(center.Y), r, rl.NewColor(255, 210, 40, 230))
}
