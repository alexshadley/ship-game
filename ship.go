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

	Position  rl.Vector2 // world position of the ship origin (cockpit)
	Direction float32    // heading in radians; 0 points "up" (-Y on screen)
	Velocity  rl.Vector2 // world-space velocity in pixels/sec
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
	return nil
}

// DefaultShip builds a small, valid arrowhead-shaped ship centered at pos:
// a cockpit up front, a block body with wings, and two rear engines.
func DefaultShip(pos rl.Vector2) *Ship {
	s := NewShip(pos)
	s.AddPart(GridCoord{X: 0, Y: 0}, NewPart(PartCockpit, FacingUp))
	s.AddPart(GridCoord{X: 0, Y: 1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: -1, Y: 1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 1, Y: 1}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: 0, Y: 2}, NewPart(PartBlock, FacingUp))
	s.AddPart(GridCoord{X: -1, Y: 2}, NewPart(PartEngine, FacingDown))
	s.AddPart(GridCoord{X: 1, Y: 2}, NewPart(PartEngine, FacingDown))
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

// Draw renders the ship's parts at its current position and orientation.
func (s *Ship) Draw() {
	rotDeg := s.Direction * 180 / math.Pi

	for c, p := range s.Parts {
		center := s.worldPoint(float32(c.X)*cellSize, float32(c.Y)*cellSize)

		// Dark outline behind, colored fill inset slightly in front. Both share
		// the same center so they rotate together.
		drawCell(center, cellSize, rotDeg, rl.DarkGray)
		drawCell(center, cellSize-3, rotDeg, partSpecs[p.Type].color)

		// Parts whose facing is meaningful get an in-cell arrow showing it.
		switch p.Type {
		case PartCockpit:
			s.drawFacingIndicator(c, p.Facing, rl.DarkBlue)
		case PartEngine:
			s.drawFacingIndicator(c, p.Facing, rl.Red)
		}
	}
}

// drawCell draws a square of the given size centered at center, rotated by
// rotDeg degrees about that center.
func drawCell(center rl.Vector2, size, rotDeg float32, color rl.Color) {
	rec := rl.NewRectangle(center.X, center.Y, size, size)
	origin := rl.NewVector2(size/2, size/2)
	rl.DrawRectanglePro(rec, origin, rotDeg, color)
}

// drawFacingIndicator draws a small arrow inside the part's cell pointing in its
// facing direction. The arrow is kept within the cell bounds so a part never
// draws outside its own 1x1 area.
func (s *Ship) drawFacingIndicator(c GridCoord, facing Facing, color rl.Color) {
	cx := float32(c.X) * cellSize
	cy := float32(c.Y) * cellSize

	a := facing.angle()
	sin := float32(math.Sin(float64(a)))
	cos := float32(math.Cos(float64(a)))

	// Canonical "up"-pointing arrow relative to the cell center. All offsets
	// stay within ±cellSize/2 so the arrow never leaves the cell.
	pts := [3]rl.Vector2{
		{X: 0, Y: -cellSize * 0.32},              // tip
		{X: -cellSize * 0.26, Y: cellSize * 0.2}, // base left
		{X: cellSize * 0.26, Y: cellSize * 0.2},  // base right
	}

	var w [3]rl.Vector2
	for i, p := range pts {
		// Rotate the point about the cell center by the facing angle, then place
		// it in the world (which also applies the ship's own rotation).
		rx := p.X*cos - p.Y*sin
		ry := p.X*sin + p.Y*cos
		w[i] = s.worldPoint(cx+rx, cy+ry)
	}
	rl.DrawTriangle(w[0], w[1], w[2], color)
}
