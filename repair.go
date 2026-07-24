package main

import (
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	// repairRange is how far, in world pixels, the astronaut can reach to mend a
	// part: measured from the player to the nearest edge of the target cell, so a
	// part has to be within a few blocks to be worked on.
	repairRange = cellSize * 3
	// repairRate is how much health a held beam restores per second. At 25/s a
	// battered block (max 50) mends in about two seconds.
	repairRate = 25.0
)

// repairBeamColor is the healing green the repair beam and its sparks are drawn in.
var repairBeamColor = rl.NewColor(80, 230, 130, 255)

// RepairTool is the spacewalk repair beam: while outside the ship the player holds
// right click and aims at a damaged part; if it's within reach the part's health
// ticks back up and a beam connects the astronaut to it. Out of reach (or with
// nothing to fix) the beam still shows, dissipating at its tip to signal how far
// the tool reaches. It owns only the transient beam endpoints resolved each frame.
type RepairTool struct {
	// active is true on frames the right button is held; hasTarget is true when a
	// repairable part is actually being mended this frame.
	active    bool
	hasTarget bool
	// beamStart is the astronaut; beamEnd is the target cell's center when mending,
	// otherwise the cursor direction capped at repairRange.
	beamStart rl.Vector2
	beamEnd   rl.Vector2
}

// Update runs one frame of the repair interaction: while right click is held, aim
// at the part under the cursor and, when it's damaged and within reach, restore its
// health toward the type's maximum. Endpoints for the beam are stashed for Draw. It
// only does anything while spacewalking (the caller gates it on that).
func (rt *RepairTool) Update(ship *Ship, player *Player, camera rl.Camera2D, dt float32) {
	rt.active = false
	rt.hasTarget = false

	if !rl.IsMouseButtonDown(rl.MouseButtonRight) {
		return
	}
	rt.active = true
	rt.beamStart = player.Position

	wp := mouseWorld(camera)

	// Target the part under the cursor, reachable when any part of its cell (not
	// just the center) is within repairRange of the astronaut.
	if part := ship.partAtWorld(wp); part != nil {
		coord := ship.gridAtWorld(wp)
		center := ship.worldPoint(float32(coord.X)*cellSize, float32(coord.Y)*cellSize)
		if ship.distToCell(player.Position, coord) <= repairRange {
			maxHealth := partSpecs[part.Type].health
			if part.Health < maxHealth {
				part.Health = clamp(part.Health+repairRate*dt, 0, maxHealth)
				rt.hasTarget = true
				rt.beamEnd = center
				return
			}
		}
	}

	// Nothing to mend: point the beam at the cursor but cap it at repairRange so its
	// fading tip shows how far the tool can reach.
	dx := wp.X - player.Position.X
	dy := wp.Y - player.Position.Y
	if d := dist(player.Position, wp); d > repairRange && d > 0 {
		s := repairRange / d
		dx, dy = dx*s, dy*s
	}
	rt.beamEnd = rl.NewVector2(player.Position.X+dx, player.Position.Y+dy)
}

// Draw renders the repair beam from the astronaut to its resolved endpoint. The
// beam is built from segments that taper and fade toward the tip: when mending it
// stays bright and connects solidly to the part (with a flickering spark there),
// otherwise it dissipates to nothing so its reach reads as the tool's range. It
// draws nothing while the button is up.
func (rt *RepairTool) Draw() {
	if !rt.active {
		return
	}

	const segments = 8
	tipAlpha := float32(0)
	tipWidth := float32(0.6)
	if rt.hasTarget {
		tipAlpha = 150
		tipWidth = 2
	}
	for i := 0; i < segments; i++ {
		t0 := float32(i) / segments
		t1 := float32(i+1) / segments
		a := lerpV2(rt.beamStart, rt.beamEnd, t0)
		b := lerpV2(rt.beamStart, rt.beamEnd, t1)

		alpha := lerpF(220, tipAlpha, t0) * (0.75 + 0.25*rand.Float32())
		width := lerpF(3.5, tipWidth, t0)
		c := repairBeamColor
		c.A = uint8(clamp(alpha, 0, 255))
		rl.DrawLineEx(a, b, width, c)
	}

	// A small flickering cluster of sparks marks the part being knitted back
	// together.
	if rt.hasTarget {
		for i := 0; i < 4; i++ {
			off := rl.NewVector2(
				rt.beamEnd.X+(rand.Float32()*2-1)*4,
				rt.beamEnd.Y+(rand.Float32()*2-1)*4,
			)
			rl.DrawCircleV(off, 1.5+rand.Float32()*2.5, repairBeamColor)
		}
		rl.DrawCircleV(rt.beamEnd, 3, rl.NewColor(200, 255, 220, 255))
	}
}

func lerpF(a, b, t float32) float32 {
	return a + (b-a)*t
}

func lerpV2(a, b rl.Vector2, t float32) rl.Vector2 {
	return rl.NewVector2(a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t)
}
