package main

import (
	"errors"
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// cellSize is the on-screen size, in pixels, of a single 1x1 part.
const cellSize = 36

// Ship is a collection of connected parts arrayed on a grid, together with its
// state in the world. The grid is the ship's local frame (cockpit at {0,0});
// Position/Direction place and orient that frame in the world.
type Ship struct {
	Parts map[GridCoord]*Part

	Position        rl.Vector2 // world position of the ship origin (cockpit)
	Direction       float32    // heading in radians; 0 points "up" (-Y on screen)
	Velocity        rl.Vector2 // world-space velocity in pixels/sec
	AngularVelocity float32    // turning rate in radians/sec (+clockwise)

	// Destroyed is set when the cockpit is lost; the ship's remaining parts have
	// all been flung off as loose debris and it no longer participates in the sim.
	Destroyed bool
}

// NewShip returns an empty ship positioned at pos.
func NewShip(pos rl.Vector2) *Ship {
	return &Ship{
		Parts:    make(map[GridCoord]*Part),
		Position: pos,
	}
}

// AddPart places p at grid coordinate c, replacing any existing part there.
func (s *Ship) AddPart(c GridCoord, p *Part) {
	s.Parts[c] = p
}

// Cockpit returns the coordinate of the ship's cockpit, or false if it has none.
func (s *Ship) Cockpit() (GridCoord, bool) {
	for c, p := range s.Parts {
		if p.Type == PartCockpit {
			return c, true
		}
	}
	return GridCoord{}, false
}

// Mass returns the total weight of all parts on the ship.
func (s *Ship) Mass() float32 {
	var total float32
	for _, p := range s.Parts {
		total += p.Weight
	}
	return total
}

// Validate enforces the ship's structural rules: exactly one cockpit, and every
// part connected to that cockpit directly or through other parts.
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

	// Flood-fill from the cockpit across orthogonally adjacent parts.
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

	// A control thruster attaches to the ship on exactly one side, and its Facing
	// must point toward that attached part (it thrusts along the free axis).
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
		if attached != 1 {
			return fmt.Errorf("control thruster at {%d,%d} attaches on %d sides, expected exactly 1", c.X, c.Y, attached)
		}
		off := p.Facing.offset()
		if _, ok := s.Parts[GridCoord{c.X + off.X, c.Y + off.Y}]; !ok {
			return fmt.Errorf("control thruster at {%d,%d} faces %s, but no part is attached on that side", c.X, c.Y, p.Facing)
		}
	}
	return nil
}

// connectedParts returns the set of grid coordinates reachable from cockpit by
// walking orthogonally adjacent parts. Parts absent from the result are stranded
// (no longer attached to the cockpit) and should be cut loose.
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

// DefaultShip builds a small, valid arrowhead-shaped ship centered at pos. The
// cockpit sits at the rear with nothing directly behind it (grid {0,1} is left
// open) so the pilot can pop out the back on a spacewalk; a spine of protective
// blocks shields the nose in front of it, the cannons sit out at the front
// flanks with a clear line of fire, and the engines flank the open exit slot.
func DefaultShip(pos rl.Vector2) *Ship {
	s := NewShip(pos)
	s.AddPart(GridCoord{X: 0, Y: 0}, NewPart(PartCockpit, FacingUp))
	// Protective block spine in front of (ahead of, -Y) the cockpit, tapering to a
	// nose tip so the cockpit is shielded from head-on impacts. The block wall
	// stays on the centerline; the flanking front cells are left for the cannons.
	s.AddPart(GridCoord{X: 0, Y: -1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 0, Y: -2}, NewPart(PartBlock, FacingUp))
	// Blocks flanking the cockpit that tie the wing thrusters and engines into the
	// hull now that the cannons have moved forward.
	s.AddPart(GridCoord{X: -1, Y: 0}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: 0}, NewPart(PartBlock, FacingUp))
	// Rear engines flanking the open exit slot at {0,1}; the cell directly behind
	// the cockpit stays empty so the pilot can climb out.
	s.AddPart(GridCoord{X: -1, Y: 1}, NewPart(PartEngine, FacingDown))
	s.AddPart(GridCoord{X: 1, Y: 1}, NewPart(PartEngine, FacingDown))
	// Wing-tip control thrusters, each attached on its inboard side only.
	s.AddPart(GridCoord{X: -2, Y: 0}, NewPart(PartControlThruster, FacingRight))
	s.AddPart(GridCoord{X: 2, Y: 0}, NewPart(PartControlThruster, FacingLeft))
	// Forward-firing cannons out at the front flanks with a clear line of fire
	// ahead of the hull — nothing sits directly in front of them.
	s.AddPart(GridCoord{X: -1, Y: -1}, NewPart(PartCannon, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: -1}, NewPart(PartCannon, FacingUp))
	return s
}

// worldPoint maps a point expressed in the ship's local pixel frame (before
// rotation) to world coordinates, applying the ship's direction and position.
func (s *Ship) worldPoint(lx, ly float32) rl.Vector2 {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))
	return rl.NewVector2(
		s.Position.X+lx*cos-ly*sin,
		s.Position.Y+lx*sin+ly*cos,
	)
}

// partAtWorld returns the part occupying the ship cell that world point wp lands
// in, or nil if the point is off the ship. It inverts worldPoint: shift by the
// ship origin and un-rotate by the ship's heading to recover local pixels, then
// round to the nearest cell (each part fills one cellSize box around its center).
func (s *Ship) partAtWorld(wp rl.Vector2) *Part {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))
	dx := wp.X - s.Position.X
	dy := wp.Y - s.Position.Y
	lx := dx*cos + dy*sin
	ly := -dx*sin + dy*cos
	c := GridCoord{
		X: int(math.Round(float64(lx / cellSize))),
		Y: int(math.Round(float64(ly / cellSize))),
	}
	return s.Parts[c]
}

// worldVec rotates a vector expressed in the ship's local frame into world space
// by the ship's direction, without applying its position (unlike worldPoint).
func (s *Ship) worldVec(lx, ly float32) rl.Vector2 {
	sin := float32(math.Sin(float64(s.Direction)))
	cos := float32(math.Cos(float64(s.Direction)))
	return rl.NewVector2(lx*cos-ly*sin, lx*sin+ly*cos)
}

// ExhaustSource is a point on the ship where exhaust is emitted, together with
// the outward (world-space, unit) direction the plume travels.
type ExhaustSource struct {
	Pos rl.Vector2
	Dir rl.Vector2
}

// EngineExhaustSources returns one source per engine, at the rear face of the
// engine's cell and pointing out its back (opposite the forward thrust).
func (s *Ship) EngineExhaustSources() []ExhaustSource {
	var out []ExhaustSource
	for c, p := range s.Parts {
		if p.Type != PartEngine {
			continue
		}
		// An engine's facing points to its rear, so exhaust leaves along it.
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

// ControlThrusterExhaustSources returns the firing nozzle of each control
// thruster for the given turn: -1 for a left turn (A), +1 for a right turn (D).
// A thruster expels exhaust from the nozzle whose reaction spins the ship the
// requested way, so the visible plume matches the applied torque.
func (s *Ship) ControlThrusterExhaustSources(turn int) []ExhaustSource {
	var out []ExhaustSource
	for c, p := range s.Parts {
		if p.Type != PartControlThruster {
			continue
		}
		// Thrust axis is perpendicular to the attachment side (see nozzle drawing).
		off := p.Facing.offset()
		tx, ty := float32(-off.Y), float32(off.X)
		cx := float32(c.X) * cellSize
		cy := float32(c.Y) * cellSize

		// Expelling along +thrust pushes the ship along -thrust; about the cockpit
		// origin that yields torque of sign(cx*ty - cy*tx) for the -thrust nozzle,
		// so firing +sign here turns left. Flip for a right turn.
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

// Draw renders the ship's parts at its current position and orientation.
func (s *Ship) Draw() {
	for c, p := range s.Parts {
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		drawPart(center, s.Direction, p)

		// A damaged part shows a small health bar above its cell.
		if maxHealth := partSpecs[p.Type].health; p.Health < maxHealth {
			drawHealthBar(center, p.Health/maxHealth)
		}
	}
}

// drawPart renders a single part's cell (dark outline plus colored fill) and, for
// parts whose facing is meaningful, its in-cell facing indicator. center is the
// part's world position and baseAngle the world rotation of the frame it sits in
// (the ship's Direction for ship parts, a debris rotation for loose parts), so
// the same routine draws parts attached to a ship or drifting free.
func drawPart(center rl.Vector2, baseAngle float32, p *Part) {
	rotDeg := baseAngle * 180 / math.Pi

	// Dark outline behind, colored fill inset slightly in front. Both share the
	// same center so they rotate together.
	drawCell(center, cellSize, rotDeg, rl.DarkGray)
	drawCell(center, cellSize-3, rotDeg, partSpecs[p.Type].color)

	switch p.Type {
	case PartCockpit:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.DarkBlue)
	case PartEngine:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.Red)
	case PartControlThruster:
		drawControlThrusterNozzlesAt(center, baseAngle, p.Facing)
	case PartCannon:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.Black)
	}
}

// drawHealthBar draws a small screen-aligned health bar just above center, with
// its green fill proportional to frac (0..1) over a dark red background. It stays
// axis-aligned rather than rotating with the ship so it always reads cleanly.
func drawHealthBar(center rl.Vector2, frac float32) {
	if frac < 0 {
		frac = 0
	}
	const barWidth int32 = cellSize * 4 / 5 // ~0.8 of a cell
	const barHeight int32 = 4
	x := int32(center.X) - barWidth/2
	y := int32(center.Y-cellSize*0.5) - barHeight - 2
	rl.DrawRectangle(x, y, barWidth, barHeight, rl.Maroon)
	rl.DrawRectangle(x, y, int32(float32(barWidth)*frac), barHeight, rl.Lime)
	rl.DrawRectangleLines(x, y, barWidth, barHeight, rl.Black)
}

// drawCell draws a square of the given size centered at center, rotated by
// rotDeg degrees about that center.
func drawCell(center rl.Vector2, size, rotDeg float32, color rl.Color) {
	rec := rl.NewRectangle(center.X, center.Y, size, size)
	origin := rl.NewVector2(size/2, size/2)
	rl.DrawRectanglePro(rec, origin, rotDeg, color)
}

// drawFacingIndicatorAt draws a small arrow centered at center pointing along
// angle (world radians, 0 = up). The arrow stays within ±cellSize/2 of center so
// it never leaves the part's own 1x1 cell.
func drawFacingIndicatorAt(center rl.Vector2, angle float32, color rl.Color) {
	sin := float32(math.Sin(float64(angle)))
	cos := float32(math.Cos(float64(angle)))

	// Canonical "up"-pointing arrow relative to the cell center. All offsets
	// stay within ±cellSize/2 so the arrow never leaves the cell.
	pts := [3]rl.Vector2{
		{X: 0, Y: -cellSize * 0.32},              // tip
		{X: -cellSize * 0.26, Y: cellSize * 0.2}, // base left
		{X: cellSize * 0.26, Y: cellSize * 0.2},  // base right
	}

	var w [3]rl.Vector2
	for i, p := range pts {
		rx := p.X*cos - p.Y*sin
		ry := p.X*sin + p.Y*cos
		w[i] = rl.NewVector2(center.X+rx, center.Y+ry)
	}
	rl.DrawTriangle(w[0], w[1], w[2], color)
}

// drawControlThrusterNozzlesAt draws a control thruster's two nozzles, one firing
// to each side of its attachment axis, centered at center within a frame rotated
// by baseAngle (world radians). Like drawFacingIndicatorAt, every offset is kept
// within ±cellSize/2 so the thruster never draws outside its own cell.
func drawControlThrusterNozzlesAt(center rl.Vector2, baseAngle float32, facing Facing) {
	sin := float32(math.Sin(float64(baseAngle)))
	cos := float32(math.Cos(float64(baseAngle)))
	// Rotate a local-frame offset (relative to the cell center) into world space.
	at := func(x, y float32) rl.Vector2 {
		return rl.NewVector2(center.X+x*cos-y*sin, center.Y+x*sin+y*cos)
	}

	// Thrust axis (perpendicular to the attachment side) and attachment axis, as
	// unit steps in the local pixel frame. The two arrows point along ±thrust.
	off := facing.offset()
	tx, ty := float32(-off.Y), float32(off.X) // thrust axis
	ax, ay := float32(off.X), float32(off.Y)  // attachment axis (arrow base spread)

	for _, sign := range []float32{1, -1} {
		dx, dy := sign*tx, sign*ty
		tip := at(dx*cellSize*0.42, dy*cellSize*0.42)
		b1 := at(dx*cellSize*0.15+ax*cellSize*0.16, dy*cellSize*0.15+ay*cellSize*0.16)
		b2 := at(dx*cellSize*0.15-ax*cellSize*0.16, dy*cellSize*0.15-ay*cellSize*0.16)
		// Keep a consistent winding on both sides so neither gets back-face culled.
		if sign > 0 {
			rl.DrawTriangle(tip, b1, b2, rl.Violet)
		} else {
			rl.DrawTriangle(tip, b2, b1, rl.Violet)
		}
	}
}
