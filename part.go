package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type PartType int

const (
	PartCockpit PartType = iota
	PartBlock
	// PartArmor is a heavy plated block: triple a normal block's health at twice
	// its weight (see partSpecs).
	PartArmor
	PartEngine
	// PartControlThruster attaches on exactly one side; its Facing points toward
	// that attached side, and it thrusts along the perpendicular axis.
	PartControlThruster
	// PartPDC is a point-defense cannon: it slews its aim toward the ship's fire
	// target anywhere within pdcHalfArc of its mount facing and spits short-ranged
	// rounds (see FireWeapons).
	PartPDC
	// PartSlowPDC is a junk PDC: it fires the same round as PartPDC but at a
	// third the cadence (see slowPDCFireInterval). Stock enemies carry one.
	PartSlowPDC
	// PartMissileLauncher is a heavy weapon: it fires rarely and within a narrow
	// arc, launching a slow round that accelerates to a cruise speed, can be shot
	// down in flight (it has health), and detonates for area damage that shoves
	// ships away from the blast (see FireWeapons and Physics.missileBlast).
	PartMissileLauncher

	// partTypeCount is the sentinel one past the last real part. Callers that need
	// "every part" (the designer palette, the file-format parser) iterate up to it,
	// so a new part type added above shows up everywhere automatically.
	partTypeCount
)

// AllPartTypes returns every part type in enum order. Anything that lists the full
// set of parts should derive from this rather than hand-maintaining a slice, so a
// newly added part type is picked up automatically.
func AllPartTypes() []PartType {
	types := make([]PartType, 0, partTypeCount)
	for t := PartCockpit; t < partTypeCount; t++ {
		types = append(types, t)
	}
	return types
}

func (t PartType) String() string {
	switch t {
	case PartCockpit:
		return "Cockpit"
	case PartBlock:
		return "Block"
	case PartArmor:
		return "Armor"
	case PartEngine:
		return "Engine"
	case PartControlThruster:
		return "Control Thruster"
	case PartPDC:
		return "PDC"
	case PartSlowPDC:
		return "Slow PDC"
	case PartMissileLauncher:
		return "Missile Launcher"
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
	// FireCooldown is the per-PDC countdown to its next shot, so each PDC
	// fires on its own cadence regardless of the ship's other PDCs.
	FireCooldown float32
}

type partSpec struct {
	health float32
	weight float32
	color  rl.Color
	// price is what the part costs in the shop, and what it's worth as shop
	// inventory. The cockpit is free (and never sold).
	price int
}

// partWeight is the mass of a standard part. Most parts weigh the same, so a
// ship's center of mass is close to the centroid of its occupied cells; heavy
// parts like armor deviate from that and are handled by the true mass-weighted
// center of mass (see the physics body's shape masses).
const partWeight float32 = 2.0

var partSpecs = map[PartType]partSpec{
	PartCockpit: {health: 75, weight: partWeight, color: rl.SkyBlue, price: 0},
	PartBlock:   {health: 150, weight: partWeight, color: rl.Gray, price: 20},
	// Armor is a heavy plate: triple a block's health at twice a normal part's weight.
	PartArmor:  {health: 450, weight: 2 * partWeight, color: rl.DarkBlue, price: 60},
	PartEngine: {health: 150, weight: partWeight, color: rl.Orange, price: 40},

	PartControlThruster: {health: 150, weight: partWeight, color: rl.Purple, price: 40},
	PartPDC:             {health: 75, weight: partWeight, color: rl.DarkGreen, price: 80},
	PartSlowPDC:         {health: 75, weight: partWeight, color: rl.DarkBrown, price: 30},
	PartMissileLauncher: {health: 75, weight: partWeight, color: rl.Maroon, price: 150},
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
