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

// Validate enforces the ship's structural rules: exactly one cockpit, every part
// connected to it, and every control thruster attached on exactly one side facing
// that attachment.
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
	s.AddPart(GridCoord{X: -1, Y: -1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: -1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 0, Y: -2}, NewPart(PartBlock, FacingUp))
	// Engines flank the rear; the cell directly behind the cockpit ({0,1}) is left
	// empty so the pilot can climb out to spacewalk.
	s.AddPart(GridCoord{X: -1, Y: 1}, NewPart(PartEngine, FacingDown))
	s.AddPart(GridCoord{X: 1, Y: 1}, NewPart(PartEngine, FacingDown))
	s.AddPart(GridCoord{X: -2, Y: 0}, NewPart(PartControlThruster, FacingRight))
	s.AddPart(GridCoord{X: 2, Y: 0}, NewPart(PartControlThruster, FacingLeft))
	s.AddPart(GridCoord{X: -1, Y: 0}, NewPart(PartCannon, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: 0}, NewPart(PartCannon, FacingUp))
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
	case PartCannon:
		drawFacingIndicatorAt(center, baseAngle+p.Facing.angle(), rl.Black)
	}
}

// drawHealthBar stays screen-aligned rather than rotating with the ship so it
// always reads cleanly.
func drawHealthBar(center rl.Vector2, frac float32) {
	if frac < 0 {
		frac = 0
	}
	const barWidth int32 = cellSize * 4 / 5
	const barHeight int32 = 4
	x := int32(center.X) - barWidth/2
	y := int32(center.Y-cellSize*0.5) - barHeight - 2
	rl.DrawRectangle(x, y, barWidth, barHeight, rl.Maroon)
	rl.DrawRectangle(x, y, int32(float32(barWidth)*frac), barHeight, rl.Lime)
	rl.DrawRectangleLines(x, y, barWidth, barHeight, rl.Black)
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
