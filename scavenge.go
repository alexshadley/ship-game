package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// scavengeSnapRange is how close, in world pixels, the dragged part's center must
// be to one of the ship's part centers for placement to snap onto the ship's
// grid. Beyond this the part just trails the cursor freely.
const scavengeSnapRange = cellSize * 2

// scavengePryDuration is how long, in seconds, the player must hold left click
// over an exterior part before it pries loose into their hands.
const scavengePryDuration = 1.0

// scavengeReach is how far, in world pixels, the astronaut can reach to grab a
// loose part or pry one off a hull — twice the repair tool's reach. Grabbing and
// prying are gated on it, and a part being dragged is towed only toward points
// within it, so nothing can be salvaged or hauled from beyond arm's length.
const scavengeReach = repairRange * 2

// Scavenger is the spacewalk part-scavenging tool: while outside the ship the
// player holds left click over a loose part to grab it (the part stays a physics
// body, towed toward the cursor), drags it to the ship where it snaps to the grid,
// presses R to rotate it, and releases to attach. The grabbed part itself lives in
// the physics sim (see Physics.grab); the Scavenger owns only the dwell-to-pry
// state and the placement it resolved this frame, which Draw renders as a ghost.
type Scavenger struct {
	// prying tracks the dwell-to-detach interaction: while the player holds left
	// click over an exterior part of any ship (their own or an enemy's), pryTimer
	// accumulates against scavengePryDuration; on completion that part detaches into
	// the player's grip. pryShip/pryCoord are the ship and cell currently being pried.
	prying   bool
	pryShip  *Ship
	pryCoord GridCoord
	pryTimer float32

	// The placement resolved by the most recent Update, consumed by Draw and the
	// release handler. Only meaningful while a part is grabbed: snapped is true when
	// the cursor is over a grid cell to attach to, snapCoord/valid describe it, and
	// pos/baseAngle place the ghost preview there.
	snapped   bool
	snapCoord GridCoord
	valid     bool
	pos       rl.Vector2 // world center of the snap ghost
	baseAngle float32    // world rotation of the ghost's frame (radians)
}

// mouseWorld converts the OS mouse position into world coordinates, accounting
// for the render texture being drawn at gameWidth×gameHeight and scaled into the
// letterboxed viewport, then unprojecting through the game camera.
func mouseWorld(camera rl.Camera2D) rl.Vector2 {
	tex := toLogical(rl.GetMousePosition(), gameWidth, gameHeight)
	return rl.GetScreenToWorld2D(tex, camera)
}

// Update runs one frame of the scavenging interaction: grab a loose part on left
// press (within reach), tow whatever's grabbed toward the cursor, resolve where it
// would attach (snapping to the ship's grid when near it), rotate it 90° on R, and
// on release either attach it (valid snap) or let it go loose. It only does
// anything while spacewalking.
func (sc *Scavenger) Update(physics *Physics, ship *Ship, player *Player, camera rl.Camera2D, dt float32) {
	wp := mouseWorld(camera)

	if physics.GrabbedPart() == nil {
		// A press grabs a loose part within reach; with none there, fall through to
		// the dwell-to-pry interaction against nearby exterior parts.
		if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
			if physics.GrabLoosePartAt(wp, player.Position) {
				sc.prying = false
				return
			}
		}
		sc.updatePry(physics, ship, player, wp, dt)
		return
	}

	// Press R to rotate the grabbed part 90° clockwise (cycling its facing).
	if held := physics.GrabbedPart(); rl.IsKeyPressed(rl.KeyR) {
		held.Facing = (held.Facing + 1) % 4
	}

	// Snapping is judged by where the part actually is (it lags the cursor), but the
	// tow still aims at the cursor.
	if partPos, ok := physics.GrabbedPos(); ok {
		sc.resolvePlacement(ship, partPos)
	}
	physics.UpdateGrab(wp, player.Position)

	if rl.IsMouseButtonReleased(rl.MouseButtonLeft) {
		sc.release(physics, ship)
	}
}

// updatePry advances the dwell-to-detach interaction while nothing is grabbed.
// Holding left click over any ship's exterior (non-cockpit) part within reach —
// the player's own or an enemy's — for scavengePryDuration pries it loose into the
// player's grip; that removal can sever other parts from that ship's cockpit, which
// break off as debris. Moving to a different cell or ship, pointing off every
// reachable part, or releasing resets the timer. Pried enemy parts still snap onto
// the player's own ship (via resolvePlacement).
func (sc *Scavenger) updatePry(physics *Physics, ship *Ship, player *Player, wp rl.Vector2, dt float32) {
	if !rl.IsMouseButtonDown(rl.MouseButtonLeft) {
		sc.prying = false
		return
	}

	target, c, ok := physics.PryTargetAt(wp)
	if !ok || target.distToCell(player.Position, c) > scavengeReach {
		sc.prying = false
		return
	}

	if !sc.prying || sc.pryShip != target || sc.pryCoord != c {
		sc.prying = true
		sc.pryShip = target
		sc.pryCoord = c
		sc.pryTimer = 0
	}

	sc.pryTimer += dt
	if sc.pryTimer >= scavengePryDuration {
		physics.DetachAndGrab(sc.pryShip, c)
		sc.prying = false
		sc.pryTimer = 0
		if partPos, ok := physics.GrabbedPos(); ok {
			sc.resolvePlacement(ship, partPos)
		}
	}
}

// resolvePlacement decides whether the grabbed part would snap onto the ship this
// frame, keyed on the part's own towed position (not the cursor): snapped to the
// cell the part is over when the part itself is within snap range of the hull (with
// a validity check for red feedback), otherwise not snapped. Because the drag is
// slow, this means you must actually haul the part up to the ship — pointing the
// cursor at the hull from afar won't do. Results are stored for Draw and release.
func (sc *Scavenger) resolvePlacement(ship *Ship, partPos rl.Vector2) {
	sc.snapped = false

	near := false
	for c := range ship.Parts {
		center := ship.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		if dist(partPos, center) <= scavengeSnapRange {
			near = true
			break
		}
	}
	if !near {
		return
	}

	sc.snapped = true
	sc.snapCoord = ship.gridAtWorld(partPos)
	sc.valid = ship.canAttachAt(sc.snapCoord)
	sc.pos = ship.worldPoint(float32(sc.snapCoord.X)*cellSize, float32(sc.snapCoord.Y)*cellSize)
	sc.baseAngle = ship.Direction
}

// release ends a drag: attach the grabbed part when it's snapped to a valid cell,
// otherwise let go of it so it stays loose in space (carrying whatever velocity the
// tow gave it).
func (sc *Scavenger) release(physics *Physics, ship *Ship) {
	if sc.snapped && sc.valid {
		physics.AttachGrabbed(ship, sc.snapCoord)
	} else {
		physics.ReleaseGrab()
	}
}

// DropHeld lets go of any grabbed part without placing it, used when the player
// re-enters the ship mid-drag. No-op when nothing is grabbed.
func (sc *Scavenger) DropHeld(physics *Physics) {
	physics.ReleaseGrab()
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

// Draw overlays placement feedback while a part is being dragged. The grabbed part
// draws itself as loose debris (it's a physics body now); on top of that, when the
// cursor is over a snap cell, a ghost of the part previews where it would attach —
// normal color when valid, red when not. While a part is grabbed it also draws the
// ship's center of mass and engine thrust line, green when the thrust is balanced
// through the center of mass and red otherwise; over a valid cell it evaluates the
// ship as it *would* be with the part attached (and ghosts the current balance
// point so the shift is visible), letting the pilot judge the spot before
// releasing. It draws nothing (beyond the pry ring) when no part is grabbed.
func (sc *Scavenger) Draw(ship *Ship, physics *Physics, player *Player) {
	held := physics.GrabbedPart()
	if held == nil {
		// While dwelling to pry a part loose, tether the astronaut to it with the same
		// tractor beam the drag uses, so working a part free reads as an active pull.
		if sc.prying && sc.pryShip != nil {
			center := sc.pryShip.worldPoint(float32(sc.pryCoord.X)*cellSize, float32(sc.pryCoord.Y)*cellSize)
			drawGrabBeam(player.Position, center)
			sc.drawPryProgress()
		}
		return
	}

	// A wobbly tractor beam tethers the astronaut to the part they're hauling, so the
	// slow drag reads as an active tow rather than the part drifting on its own.
	if pos, ok := physics.GrabbedPos(); ok {
		drawGrabBeam(player.Position, pos)
	}

	// Ghost the part at the snap cell so "release here to attach" reads clearly; the
	// real part is off near the cursor where physics has towed it.
	if sc.snapped {
		fill := partSpecs[held.Type].color
		if !sc.valid {
			fill = rl.Red
		}
		drawPartColored(sc.pos, sc.baseAngle, held, fill)
	}

	// Evaluate the previewed configuration when snapped to a valid cell, otherwise
	// the ship as it stands.
	var extra *Part
	var coord GridCoord
	if sc.snapped && sc.valid {
		extra = held
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
// approaches scavengePryDuration, so the player sees the detach charging up. It
// tracks the pried ship (own or enemy), so the ring follows a moving target.
func (sc *Scavenger) drawPryProgress() {
	if sc.pryShip == nil {
		return
	}
	center := sc.pryShip.worldPoint(float32(sc.pryCoord.X)*cellSize, float32(sc.pryCoord.Y)*cellSize)
	frac := sc.pryTimer / scavengePryDuration
	if frac > 1 {
		frac = 1
	}
	const r = cellSize * 0.45
	rl.DrawCircleSector(center, r, -90, -90+360*frac, 24, rl.NewColor(255, 210, 40, 150))
	rl.DrawCircleLines(int32(center.X), int32(center.Y), r, rl.NewColor(255, 210, 40, 230))
}

// grabBeamColor is the tractor-beam cyan the grab tether is drawn in.
var grabBeamColor = rl.NewColor(120, 200, 255, 255)

// drawGrabBeam draws a wobbly tractor beam from the astronaut (from) to the part
// being dragged (to). The beam is a chain of short segments displaced sideways by a
// sine wave that travels along its length over time and swells toward the middle
// (pinned at both ends), so it reads as an energized tether hauling the part in.
func drawGrabBeam(from, to rl.Vector2) {
	dx := to.X - from.X
	dy := to.Y - from.Y
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length < 1 {
		return
	}
	// Unit vector along the beam and its perpendicular, for the sideways wobble.
	ux, uy := dx/length, dy/length
	px, py := -uy, ux

	// Phase travels with wall-clock time so the wobble ripples toward the part.
	t := float32(rl.GetTime())
	const (
		segments = 14
		amp      = 5.0 // peak sideways displacement, world px
		waves    = 2.5 // sine humps spanning the beam
	)
	prev := from
	for i := 1; i <= segments; i++ {
		f := float32(i) / segments
		// Envelope pins the wobble to zero at the hand and the part, bulging between.
		env := float32(math.Sin(float64(f) * math.Pi))
		off := amp * env * float32(math.Sin(float64(f*waves*2*math.Pi-t*7)))
		cur := rl.NewVector2(
			from.X+ux*length*f+px*off,
			from.Y+uy*length*f+py*off,
		)
		// Flicker the alpha a touch so the tether looks live.
		c := grabBeamColor
		c.A = uint8(clamp(170+40*float32(math.Sin(float64(t*20+f*10))), 0, 255))
		rl.DrawLineEx(prev, cur, 2, c)
		prev = cur
	}
}
