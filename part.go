package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type PartType int

const (
	PartCockpit PartType = iota
	PartBlock
	PartEngine
	// PartControlThruster attaches on exactly one side; its Facing points toward
	// that attached side, and it thrusts along the perpendicular axis.
	PartControlThruster
	PartCannon
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
	case PartCannon:
		return "Cannon"
	default:
		return "Unknown"
	}
}

type Facing int

const (
	FacingUp Facing = iota
	FacingRight
	FacingDown
	FacingLeft
)

func (f Facing) angle() float32 {
	switch f {
	case FacingRight:
		return math.Pi / 2
	case FacingDown:
		return math.Pi
	case FacingLeft:
		return 3 * math.Pi / 2
	default:
		return 0
	}
}

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

type GridCoord struct {
	X, Y int
}

func (c GridCoord) neighbors() [4]GridCoord {
	return [4]GridCoord{
		{c.X, c.Y - 1},
		{c.X + 1, c.Y},
		{c.X, c.Y + 1},
		{c.X - 1, c.Y},
	}
}

type Part struct {
	Type   PartType
	Facing Facing
	Health float32
	Weight float32
}

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
	PartCannon:          {health: 70, weight: 2.0, color: rl.DarkGreen},
}

func NewPart(t PartType, facing Facing) *Part {
	spec := partSpecs[t]
	return &Part{
		Type:   t,
		Facing: facing,
		Health: spec.health,
		Weight: spec.weight,
	}
}
