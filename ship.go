package main

import (
	"errors"
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const cellSize = 36

type Ship struct {
	Parts map[GridCoord]*Part

	Position        rl.Vector2
	Direction       float32
	Velocity        rl.Vector2
	AngularVelocity float32

	Destroyed bool
}

func NewShip(pos rl.Vector2) *Ship {
	return &Ship{
		Parts:    make(map[GridCoord]*Part),
		Position: pos,
	}
}

func (s *Ship) AddPart(c GridCoord, p *Part) {
	s.Parts[c] = p
}

func (s *Ship) Cockpit() (GridCoord, bool) {
	for c, p := range s.Parts {
		if p.Type == PartCockpit {
			return c, true
		}
	}
	return GridCoord{}, false
}

func (s *Ship) Mass() float32 {
	var total float32
	for _, p := range s.Parts {
		total += p.Weight
	}
	return total
}

// CenterOfMass returns the ship's weight-weighted centroid in local (cell-pixel)
// coordinates — the same frame drawPart positions parts in, so worldPoint maps it
// to screen space. Returns the cockpit origin when the ship has no mass.
func (s *Ship) CenterOfMass() rl.Vector2 {
	return centerOfMass(s.Parts, nil, GridCoord{})
}

// centerOfMass computes the weight-weighted centroid of parts in local pixel
// coordinates. When extra is non-nil it's folded in at coord, letting callers
// preview where the center of mass would move if that part were attached there.
func centerOfMass(parts map[GridCoord]*Part, extra *Part, coord GridCoord) rl.Vector2 {
	var mass, mx, my float32
	add := func(c GridCoord, p *Part) {
		mass += p.Weight
		mx += p.Weight * float32(c.X) * cellSize
		my += p.Weight * float32(c.Y) * cellSize
	}
	for c, p := range parts {
		add(c, p)
	}
	if extra != nil {
		add(coord, extra)
	}
	if mass == 0 {
		return rl.Vector2{}
	}
	return rl.NewVector2(mx/mass, my/mass)
}

// Radius returns a rough bounding radius (world px) around the cockpit origin:
// the farthest occupied cell plus half a cell. Used by the enemy AI for obstacle
// clearance, not for physics collision.
func (s *Ship) Radius() float32 {
	var maxR float64
	for c := range s.Parts {
		r := math.Hypot(float64(c.X), float64(c.Y))
		if r > maxR {
			maxR = r
		}
	}
	return float32(maxR+0.5) * cellSize
}

// Validate enforces the ship's structural rules: exactly one cockpit, every part
// connected to it, and every control thruster attached on one or two sides. Its
// Facing only sets the thrust axis (perpendicular to Facing), so it need not point
// at an attached side.
func (s *Ship) Validate() error {
	cockpits := 0
	var cockpit GridCoord
	for c, p := range s.Parts {
		if p.Type == PartCockpit {
			cockpits++
			cockpit = c
		}
	}
	if cockpits == 0 {
		return errors.New("ship has no cockpit")
	}
	if cockpits > 1 {
		return fmt.Errorf("ship has %d cockpits, expected exactly 1", cockpits)
	}

	seen := map[GridCoord]bool{cockpit: true}
	queue := []GridCoord{cockpit}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range cur.neighbors() {
			if _, ok := s.Parts[n]; ok && !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	if len(seen) != len(s.Parts) {
		return fmt.Errorf("%d part(s) not connected to the cockpit", len(s.Parts)-len(seen))
	}

	for c, p := range s.Parts {
		if p.Type != PartControlThruster {
			continue
		}
		attached := 0
		for _, n := range c.neighbors() {
			if _, ok := s.Parts[n]; ok {
				attached++
			}
		}
		if attached < 1 || attached > 2 {
			return fmt.Errorf("control thruster at {%d,%d} attaches on %d sides, expected 1 or 2", c.X, c.Y, attached)
		}
	}
	return nil
}

// connectedParts returns the coordinates reachable from cockpit by walking
// adjacent parts. Parts absent from the result are stranded and should be cut loose.
func (s *Ship) connectedParts(cockpit GridCoord) map[GridCoord]bool {
	seen := map[GridCoord]bool{cockpit: true}
	queue := []GridCoord{cockpit}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range cur.neighbors() {
			if _, ok := s.Parts[n]; ok && !seen[n] {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	return seen
}

func DefaultShip(pos rl.Vector2) *Ship {
	s := NewShip(pos)
	s.AddPart(GridCoord{X: 0, Y: 0}, NewPart(PartCockpit, FacingUp))
	s.AddPart(GridCoord{X: 0, Y: -1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 0, Y: -2}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: -1, Y: 0}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: 0}, NewPart(PartBlock, FacingUp))
	// Engines flank the rear; the cell directly behind the cockpit ({0,1}) is left
	// empty so the pilot can climb out to spacewalk.
	s.AddPart(GridCoord{X: -1, Y: 1}, NewPart(PartEngine, FacingDown))
	s.AddPart(GridCoord{X: 1, Y: 1}, NewPart(PartEngine, FacingDown))
	s.AddPart(GridCoord{X: -2, Y: 0}, NewPart(PartControlThruster, FacingRight))
	s.AddPart(GridCoord{X: 2, Y: 0}, NewPart(PartControlThruster, FacingLeft))
	// PDCs sit at the front flanks with a clear line of fire; the cells directly
	// ahead of them are empty rather than walled off by the nose blocks.
	s.AddPart(GridCoord{X: -1, Y: -1}, NewPart(PartPDC, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: -1}, NewPart(PartPDC, FacingUp))
	// A missile launcher caps the nose, firing straight ahead over the gap between
	// the forward blocks.
	s.AddPart(GridCoord{X: 0, Y: -3}, NewPart(PartMissileLauncher, FacingUp))
	s.AddPart(GridCoord{X: 0, Y: -4}, NewPart(PartShield, FacingUp))
	// A Rattlesnake missile launcher sits at the right of the nose; its missiles eject out to the
	// right (into the open space beside the hull) and then boost in toward the target.
	s.AddPart(GridCoord{X: 1, Y: -2}, NewPart(PartRattlesnakeMissile, FacingUp))
	return s
}

// EnemyShip is the stock hostile hull: a deliberately underpowered, lopsided
// wreck. Where the player's DefaultShip is a symmetric two-engine, two-thruster,
// two-PDC craft, this thing limps along on a single engine, turns on a single
// control thruster, and fires a single junk PDC. The cockpit juts out front-left
// with no armor around it — an exposed, easy target — while the ragged hull
// trails down and to the right toward its one engine, giving it a junky,
// asymmetric silhouette.
func EnemyShip(pos rl.Vector2) *Ship {
	s := NewShip(pos)
	// Exposed cockpit, poking out the front-left corner with open space ahead and
	// to its left — no nose armor.
	s.AddPart(GridCoord{X: 0, Y: 0}, NewPart(PartCockpit, FacingUp))
	// A crooked hull spine that steps back and to the right.
	s.AddPart(GridCoord{X: 0, Y: 1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: 1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 2, Y: 1}, NewPart(PartBlock, FacingUp))
	// The lone engine hangs off the rear-right, well off the centerline.
	s.AddPart(GridCoord{X: 2, Y: 2}, NewPart(PartEngine, FacingDown))
	// A single junk PDC bolted beside the cockpit, clear line of fire straight
	// ahead. It fires at a third the rate of a standard PDC (see PartSlowPDC).
	s.AddPart(GridCoord{X: 1, Y: 0}, NewPart(PartSlowPDC, FacingUp))
	// The one control thruster sticks out the far right end of the spine.
	s.AddPart(GridCoord{X: 3, Y: 1}, NewPart(PartControlThruster, FacingLeft))
	return s
}

func (s *Ship) worldPoint(lx, ly float32) rl.Vector2 {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))
	return rl.NewVector2(
		s.Position.X+lx*cos-ly*sin,
		s.Position.Y+lx*sin+ly*cos,
	)
}

// gridAtWorld returns the grid cell that world point wp lands in. It inverts
// worldPoint: shift by the ship origin and un-rotate by the ship's heading to
// recover local pixels, then round to the nearest cell (each part fills one
// cellSize box around its center). The cell may or may not be occupied.
func (s *Ship) gridAtWorld(wp rl.Vector2) GridCoord {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))
	dx := wp.X - s.Position.X
	dy := wp.Y - s.Position.Y
	lx := dx*cos + dy*sin
	ly := -dx*sin + dy*cos
	return GridCoord{
		X: int(math.Round(float64(lx / cellSize))),
		Y: int(math.Round(float64(ly / cellSize))),
	}
}

// partAtWorld returns the part occupying the ship cell that world point wp lands
// in, or nil if the point is off the ship.
func (s *Ship) partAtWorld(wp rl.Vector2) *Part {
	return s.Parts[s.gridAtWorld(wp)]
}

func (s *Ship) shieldCovering(wp rl.Vector2) (*Part, rl.Vector2) {
	for c, part := range s.Parts {
		if !part.shieldActive() {
			continue
		}
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		if dist(wp, center) <= shieldRadius {
			return part, center
		}
	}
	return nil, rl.Vector2{}
}

// distToCell returns the distance from world point p to the nearest point of grid
// cell c's box (not its center), so reaching "any part of the block" counts. It
// works in the ship's local frame, where the cell is an axis-aligned cellSize box
// centered on the cell; distance is preserved under the rotation back to world.
func (s *Ship) distToCell(p rl.Vector2, c GridCoord) float32 {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))
	dx := p.X - s.Position.X
	dy := p.Y - s.Position.Y
	lx := dx*cos + dy*sin
	ly := -dx*sin + dy*cos

	half := float32(cellSize) / 2
	cx := float32(c.X) * cellSize
	cy := float32(c.Y) * cellSize
	ex := clamp(lx, cx-half, cx+half) - lx
	ey := clamp(ly, cy-half, cy+half) - ly
	return float32(math.Sqrt(float64(ex*ex + ey*ey)))
}

// isExterior reports whether the part at cell c sits on the hull's outer surface:
// at least one of its four orthogonal neighbors is empty, so an astronaut could
// reach it from open space. Parts walled in on all four sides are interior and
// can't be pried off. (An empty cell trivially counts as exterior, but callers
// only ask about occupied cells.)
func (s *Ship) isExterior(c GridCoord) bool {
	for _, n := range c.neighbors() {
		if _, ok := s.Parts[n]; !ok {
			return true
		}
	}
	return false
}

// canAttachAt reports whether a scavenged part may be placed at grid cell c: the
// cell must be empty and orthogonally adjacent to at least one existing part, so
// every added part stays connected to the rest of the ship (and its cockpit).
func (s *Ship) canAttachAt(c GridCoord) bool {
	if _, occupied := s.Parts[c]; occupied {
		return false
	}
	for _, n := range c.neighbors() {
		if _, ok := s.Parts[n]; ok {
			return true
		}
	}
	return false
}

// worldVec rotates a local-frame vector into world space by the ship's direction,
// without applying its position (unlike worldPoint).
func (s *Ship) worldVec(lx, ly float32) rl.Vector2 {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))
	return rl.NewVector2(lx*cos-ly*sin, lx*sin+ly*cos)
}

type ExhaustSource struct {
	Pos rl.Vector2
	Dir rl.Vector2
}

func (s *Ship) EngineExhaustSources() []ExhaustSource {
	var out []ExhaustSource
	for c, p := range s.Parts {
		if p.Type != PartEngine {
			continue
		}
		off := p.Facing.offset()
		dx, dy := float32(off.X), float32(off.Y)
		cx := float32(c.X) * cellSize
		cy := float32(c.Y) * cellSize
		out = append(out, ExhaustSource{
			Pos: s.worldPoint(cx+dx*cellSize*0.5, cy+dy*cellSize*0.5),
			Dir: s.worldVec(dx, dy),
		})
	}
	return out
}

// ControlThrusterExhaustSources returns the firing nozzle of each control thruster
// for the given turn: -1 for a left turn (A), +1 for a right turn (D). The plume
// comes from the nozzle whose reaction spins the ship the requested way.
func (s *Ship) ControlThrusterExhaustSources(turn int) []ExhaustSource {
	var out []ExhaustSource
	for c, p := range s.Parts {
		if p.Type != PartControlThruster {
			continue
		}
		off := p.Facing.offset()
		tx, ty := float32(-off.Y), float32(off.X)
		cx := float32(c.X) * cellSize
		cy := float32(c.Y) * cellSize

		// Expelling along +thrust pushes the ship along -thrust; about the cockpit
		// origin that yields torque of sign(cx*ty - cy*tx) for the -thrust nozzle, so
		// firing +sign turns left. Flip for a right turn.
		sign := float32(1)
		if cx*ty-cy*tx < 0 {
			sign = -1
		}
		if turn > 0 {
			sign = -sign
		}
		dx, dy := sign*tx, sign*ty
		out = append(out, ExhaustSource{
			Pos: s.worldPoint(cx+dx*cellSize*0.42, cy+dy*cellSize*0.42),
			Dir: s.worldVec(dx, dy),
		})
	}
	return out
}

func (s *Ship) Draw() {
	for c, p := range s.Parts {
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		drawPart(center, s.Direction, p)

		if maxHealth := partSpecs[p.Type].health; p.Health < maxHealth {
			drawHealthBar(center, p.Health/maxHealth)
		}
	}
}

// pdcArcRadius is how far each PDC's firing-arc overlay wedge reaches from its
// mount, in world pixels — a few cells, enough to read clearly over the ship.
const pdcArcRadius = cellSize * 4

// DrawFiringArcs overlays every weapon's firing arc on the ship. aim is the
// world point the player is aiming at (the cursor). Each mount whose aim at that
// point falls within its arc — the mounts that would actually fire — is drawn
// lit; the rest are dim. This mirrors the bearing test in FireWeapons. PDCs and
// missile launchers get distinct colors so the two weapon groups read apart.
func (s *Ship) DrawFiringArcs(aim rl.Vector2) {
	for c, part := range s.Parts {
		if !part.Type.isWeapon() {
			continue
		}

		halfArc := float64(part.Type.halfArc())
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		mount := s.Direction + part.Facing.angle()

		// A mount lights up when it can bear on the cursor — the same arc check
		// FireWeapons uses to decide whether the mount takes the shot.
		active := false
		if dx, dy := aim.X-center.X, aim.Y-center.Y; dx != 0 || dy != 0 {
			active = math.Abs(float64(angleDiff(heading(dx, dy), mount))) <= halfArc
		}

		// A world heading h points along (sin h, -cos h); as a raylib sector
		// angle (degrees, 0 = +x, increasing clockwise) that is h - 90°.
		startDeg := (mount-float32(halfArc))*180/math.Pi - 90
		endDeg := (mount+float32(halfArc))*180/math.Pi - 90

		// Missile launchers glow red; railguns glow white; PDCs glow blue (amber
		// when bearing).
		var fill, edge rl.Color
		if part.Type == PartRailgun {
			fill = rl.NewColor(220, 235, 255, 19)
			edge = rl.NewColor(220, 235, 255, 78)
			if active {
				fill = rl.NewColor(240, 248, 255, 55)
				edge = rl.NewColor(255, 255, 255, 200)
			}
		} else if part.Type == PartMissileLauncher {
			fill = rl.NewColor(220, 90, 80, 19)
			edge = rl.NewColor(220, 90, 80, 78)
			if active {
				fill = rl.NewColor(255, 80, 60, 44)
				edge = rl.NewColor(255, 90, 70, 175)
			}
		} else if part.Type == PartRattlesnakeMissile {
			fill = rl.NewColor(180, 120, 60, 19)
			edge = rl.NewColor(180, 120, 60, 78)
			if active {
				fill = rl.NewColor(210, 145, 70, 44)
				edge = rl.NewColor(225, 160, 80, 175)
			}
		} else {
			fill = rl.NewColor(120, 170, 220, 19)
			edge = rl.NewColor(120, 170, 220, 78)
			if active {
				fill = rl.NewColor(255, 200, 70, 44)
				edge = rl.NewColor(255, 205, 80, 175)
			}
		}

		rl.DrawCircleSector(center, pdcArcRadius, startDeg, endDeg, 24, fill)
		rl.DrawCircleSectorLines(center, pdcArcRadius, startDeg, endDeg, 24, edge)
	}
}

func (s *Ship) DrawShields() {
	const shimmerSegments = 64
	now := float32(rl.GetTime())
	for c, part := range s.Parts {
		if part.Type != PartShield {
			continue
		}
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)

		if part.shieldActive() {
			frac := clamp01(part.ShieldHealth / shieldMaxHealth)
			rl.DrawCircleV(center, shieldRadius, rl.NewColor(120, 180, 255, uint8(20*frac)))
			for i := 0; i < shimmerSegments; i++ {
				a0 := float32(i) / shimmerSegments * 360
				a1 := float32(i+1) / shimmerSegments * 360
				mid := float64((a0+a1)/2)*math.Pi/180 + float64(s.Direction)
				shimmer := 0.5 + 0.5*float32(math.Sin(mid*4+float64(now)*3))
				a := uint8((45 + 150*shimmer) * frac)
				rl.DrawRing(center, shieldRadius-3, shieldRadius+3, a0, a1, 2, rl.NewColor(165, 210, 255, a))
			}
		}

		for _, im := range part.ShieldImpacts {
			t := clamp01(im.timer / shieldFlashDuration)
			const spanDeg = 55
			rl.DrawCircleV(center, shieldRadius, rl.NewColor(150, 215, 255, uint8(45*t)))
			rl.DrawRing(center, shieldRadius-11, shieldRadius+7, im.angle-spanDeg/2, im.angle+spanDeg/2, 24, rl.NewColor(190, 235, 255, uint8(255*t)))
		}
	}
}

// drawPart renders a single part's cell and, for parts whose facing is meaningful,
// its facing indicator. baseAngle is the world rotation of the frame it sits in
// (the ship's Direction, or a debris rotation for loose parts).
func drawPart(center rl.Vector2, baseAngle float32, p *Part) {
	drawPartColored(center, baseAngle, p, partSpecs[p.Type].color)
}

// drawPartColored is drawPart with an explicit fill color, so a scavenged part
// being placed can be tinted (e.g. red for an invalid spot) while keeping the
// same outline, facing indicators, and geometry as a normal part.
func drawPartColored(center rl.Vector2, baseAngle float32, p *Part, fill rl.Color) {
	rotDeg := baseAngle * 180 / math.Pi

	drawCell(center, cellSize, rotDeg, rl.DarkGray)
	drawCell(center, cellSize-3, rotDeg, fill)

	switch p.Type {
	case PartCockpit:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.DarkBlue)
	case PartEngine:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.Red)
	case PartControlThruster:
		drawControlThrusterNozzlesAt(center, baseAngle, p.Facing)
	case PartPDC, PartSlowPDC:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.Black)
	case PartMissileLauncher:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.Yellow)
	case PartRailgun:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.DarkGray)
	case PartRattlesnakeMissile:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.Yellow)
	}
}

// drawHealthBar stays screen-aligned rather than rotating with the ship so it
// always reads cleanly. The bar shrinks with health and also shifts from green
// to red so it stays legible when zoomed out.
func drawHealthBar(center rl.Vector2, frac float32) {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	const barWidth int32 = cellSize * 4 / 5
	const barHeight int32 = 4
	x := int32(center.X) - barWidth/2
	y := int32(center.Y-cellSize*0.5) - barHeight - 2
	rl.DrawRectangle(x, y, int32(float32(barWidth)*frac), barHeight, healthColor(frac))
}

// healthColor blends from red (frac 0) through yellow to green (frac 1).
func healthColor(frac float32) rl.Color {
	return rl.NewColor(
		uint8(255*clamp01(2*(1-frac))),
		uint8(255*clamp01(2*frac)),
		0,
		255,
	)
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// engineThrustLine is the line of action of a ship's combined engine thrust, in
// local (cell-pixel) coordinates: a point the line passes through, the unit push
// direction, and the perpendicular distance (offset) from the reference point to
// the line. ok is false when the ship has no net engine force (no line to draw).
type engineThrustLine struct {
	point  rl.Vector2
	dir    rl.Vector2
	offset float32
	ok     bool
}

// engineForceTorqueAbout sums the ship's engine thrust (engineThrust per engine)
// into a net force and the torque it exerts about ref, in local (cell-pixel)
// coordinates. extra, if non-nil, is folded in at coord. This is the shared core
// of both the physics straight-flight test (Physics.Update) and the HUD thrust-
// line overlay, so the sim and the on-screen readout can never disagree about
// where thrust pushes or whether it's balanced.
func engineForceTorqueAbout(parts map[GridCoord]*Part, extra *Part, coord GridCoord, ref rl.Vector2) (force rl.Vector2, torque float32) {
	add := func(c GridCoord, p *Part) {
		if p.Type != PartEngine {
			return
		}
		off := p.Facing.offset()
		lfx, lfy := float32(-off.X)*engineThrust, float32(-off.Y)*engineThrust
		force.X += lfx
		force.Y += lfy
		rx := float32(c.X)*cellSize - ref.X
		ry := float32(c.Y)*cellSize - ref.Y
		torque += rx*lfy - ry*lfx
	}
	for c, p := range parts {
		add(c, p)
	}
	if extra != nil {
		add(coord, extra)
	}
	return force, torque
}

// engineThrustAbout builds the combined engine thrust line for parts (optionally
// folding in extra at coord), taking torque about ref. Its offset — the
// perpendicular distance from ref to the line — is the same quantity
// Physics.Update compares against engineStraightTolerance to decide whether the
// engines drive the ship straight, so a caller passing the center of mass as ref
// gets an on-screen readout of that balance test.
func engineThrustAbout(parts map[GridCoord]*Part, extra *Part, coord GridCoord, ref rl.Vector2) engineThrustLine {
	force, torque := engineForceTorqueAbout(parts, extra, coord, ref)
	mag := float32(math.Hypot(float64(force.X), float64(force.Y)))
	if mag == 0 {
		return engineThrustLine{}
	}
	// Foot of the perpendicular from ref onto the line of action: ref + τ·(Fy,-Fx)/|F|².
	return engineThrustLine{
		point:  rl.NewVector2(ref.X+torque*force.Y/(mag*mag), ref.Y-torque*force.X/(mag*mag)),
		dir:    rl.NewVector2(force.X/mag, force.Y/mag),
		offset: float32(math.Abs(float64(torque))) / mag,
		ok:     true,
	}
}

// thrustLineHalfLength returns how far to extend the thrust line each way from
// point so it spans the whole ship (plus a cell of margin) rather than reading as
// a stub near the center of mass.
func thrustLineHalfLength(parts map[GridCoord]*Part, extra *Part, coord GridCoord, point rl.Vector2) float32 {
	max := float32(cellSize)
	consider := func(c GridCoord) {
		dx := float32(c.X)*cellSize - point.X
		dy := float32(c.Y)*cellSize - point.Y
		if d := float32(math.Hypot(float64(dx), float64(dy))); d > max {
			max = d
		}
	}
	for c := range parts {
		consider(c)
	}
	if extra != nil {
		consider(coord)
	}
	return max + cellSize
}

// drawThrustLine draws tl across the ship as an arrow pointing the way the engines
// push. The line and its arrowhead are rotated into world space through the ship's
// frame so it tracks the hull's heading.
func drawThrustLine(ship *Ship, tl engineThrustLine, half float32, col rl.Color) {
	a := ship.worldPoint(tl.point.X-tl.dir.X*half, tl.point.Y-tl.dir.Y*half)
	b := ship.worldPoint(tl.point.X+tl.dir.X*half, tl.point.Y+tl.dir.Y*half)
	rl.DrawLineEx(a, b, 2, col)

	// Arrowhead at the push (+dir) end. Two short strokes avoid triangle back-face
	// culling regardless of the ship's orientation.
	wd := ship.worldVec(tl.dir.X, tl.dir.Y)
	px, py := -wd.Y, wd.X
	const h = cellSize * 0.4
	base := rl.NewVector2(b.X-wd.X*h, b.Y-wd.Y*h)
	rl.DrawLineEx(b, rl.NewVector2(base.X+px*h*0.6, base.Y+py*h*0.6), 2, col)
	rl.DrawLineEx(b, rl.NewVector2(base.X-px*h*0.6, base.Y-py*h*0.6), 2, col)
}

// drawCenterOfMassMarker draws the classic balance-point glyph — a ring with two
// opposite quadrants filled — centered at the given world point. It stays screen-
// aligned (the symbol is rotationally symmetric, so orientation doesn't matter).
func drawCenterOfMassMarker(center rl.Vector2, color rl.Color) {
	const r = cellSize * 0.3
	rl.DrawCircleV(center, r, rl.NewColor(255, 255, 255, 200))
	rl.DrawCircleSector(center, r, 0, 90, 12, color)
	rl.DrawCircleSector(center, r, 180, 270, 12, color)
	rl.DrawCircleLines(int32(center.X), int32(center.Y), r, color)
}

// drawCenterOfMassGhost marks a faint, hollow crosshair where the center of mass
// currently sits, so a placement preview can show how far the balance point moves.
func drawCenterOfMassGhost(center rl.Vector2) {
	const r = cellSize * 0.22
	col := rl.NewColor(90, 90, 90, 170)
	rl.DrawCircleLines(int32(center.X), int32(center.Y), r, col)
	rl.DrawLineEx(rl.NewVector2(center.X-r, center.Y), rl.NewVector2(center.X+r, center.Y), 1, col)
	rl.DrawLineEx(rl.NewVector2(center.X, center.Y-r), rl.NewVector2(center.X, center.Y+r), 1, col)
}

func drawCell(center rl.Vector2, size, rotDeg float32, color rl.Color) {
	rec := rl.NewRectangle(center.X, center.Y, size, size)
	origin := rl.NewVector2(size/2, size/2)
	rl.DrawRectanglePro(rec, origin, rotDeg, color)
}

func drawFacingIndicatorAt(center rl.Vector2, angle float32, color rl.Color) {
	sin := float32(math.Sin(float64(angle)))
	cos := float32(math.Cos(float64(angle)))

	pts := [3]rl.Vector2{
		{X: 0, Y: -cellSize * 0.32},
		{X: -cellSize * 0.26, Y: cellSize * 0.2},
		{X: cellSize * 0.26, Y: cellSize * 0.2},
	}

	var w [3]rl.Vector2
	for i, p := range pts {
		rx := p.X*cos - p.Y*sin
		ry := p.X*sin + p.Y*cos
		w[i] = rl.NewVector2(center.X+rx, center.Y+ry)
	}
	rl.DrawTriangle(w[0], w[1], w[2], color)
}

func drawControlThrusterNozzlesAt(center rl.Vector2, baseAngle float32, facing Facing) {
	sin := float32(math.Sin(float64(baseAngle)))
	cos := float32(math.Cos(float64(baseAngle)))
	at := func(x, y float32) rl.Vector2 {
		return rl.NewVector2(center.X+x*cos-y*sin, center.Y+x*sin+y*cos)
	}

	off := facing.offset()
	tx, ty := float32(-off.Y), float32(off.X)
	ax, ay := float32(off.X), float32(off.Y)

	for _, sign := range []float32{1, -1} {
		dx, dy := sign*tx, sign*ty
		tip := at(dx*cellSize*0.42, dy*cellSize*0.42)
		b1 := at(dx*cellSize*0.15+ax*cellSize*0.16, dy*cellSize*0.15+ay*cellSize*0.16)
		b2 := at(dx*cellSize*0.15-ax*cellSize*0.16, dy*cellSize*0.15-ay*cellSize*0.16)
		// Swap winding on the second nozzle so neither triangle gets back-face culled.
		if sign > 0 {
			rl.DrawTriangle(tip, b1, b2, rl.Violet)
		} else {
			rl.DrawTriangle(tip, b2, b1, rl.Violet)
		}
	}
}
