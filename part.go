package main

import rl "github.com/gen2brain/raylib-go/raylib"

// PartType identifies a category of ship part.
type PartType int

const (
	// PartCockpit is the command module. A ship has exactly one, at grid {0,0}.
	PartCockpit PartType = iota
	// PartBlock is a standard structural component.
	PartBlock
	// PartEngine produces forward thrust.
	PartEngine
	// PartControlThruster is a maneuvering thruster that fires to either side.
	// It attaches to the ship on exactly one side; its Facing points toward that
	// attached side, and it thrusts along the perpendicular axis.
	PartControlThruster
)

func (t PartType) String() string {
	switch t {
	case PartCockpit:
		return "Cockpit"
	case PartBlock:
		return "Block"
	case PartEngine:
		return "Engine"
	case PartControlThruster:
		return "Control Thruster"
	default:
		return "Unknown"
	}
}

// Facing is the direction a part points. It is irrelevant for some part types
// (e.g. blocks), which conventionally use FacingUp.
type Facing int

const (
	FacingUp Facing = iota
	FacingRight
	FacingDown
	FacingLeft
)

func (f Facing) String() string {
	switch f {
	case FacingUp:
		return "Up"
	case FacingRight:
		return "Right"
	case FacingDown:
		return "Down"
	case FacingLeft:
		return "Left"
	default:
		return "Unknown"
	}
}

// offset returns the unit grid step in the direction of f.
func (f Facing) offset() GridCoord {
	switch f {
	case FacingUp:
		return GridCoord{0, -1}
	case FacingRight:
		return GridCoord{1, 0}
	case FacingDown:
		return GridCoord{0, 1}
	case FacingLeft:
		return GridCoord{-1, 0}
	default:
		return GridCoord{}
	}
}

// GridCoord is an integer position on a ship's part grid. The cockpit sits at
// {0,0}; +X is right and +Y is toward the rear of the ship.
type GridCoord struct {
	X, Y int
}

// neighbors returns the four orthogonally adjacent coordinates.
func (c GridCoord) neighbors() [4]GridCoord {
	return [4]GridCoord{
		{c.X, c.Y - 1},
		{c.X + 1, c.Y},
		{c.X, c.Y + 1},
		{c.X - 1, c.Y},
	}
}

// Part is a single 1x1 cube component of a ship.
type Part struct {
	Type   PartType
	Facing Facing
	Health float32
	Weight float32
}

// partSpec holds the default attributes and rendering color for a part type.
type partSpec struct {
	health float32
	weight float32
	color  rl.Color
}

var partSpecs = map[PartType]partSpec{
	PartCockpit: {health: 150, weight: 3.0, color: rl.SkyBlue},
	PartBlock:   {health: 100, weight: 2.0, color: rl.Gray},
	PartEngine:  {health: 80, weight: 2.5, color: rl.Orange},

	PartControlThruster: {health: 60, weight: 1.5, color: rl.Purple},
}

// NewPart builds a part of the given type and facing with default health/weight.
func NewPart(t PartType, facing Facing) *Part {
	spec := partSpecs[t]
	return &Part{
		Type:   t,
		Facing: facing,
		Health: spec.health,
		Weight: spec.weight,
	}
}
