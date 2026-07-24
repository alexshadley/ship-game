package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// LoosePart is a single ship part that has broken free of its ship — either
// stranded when the parts between it and the cockpit were destroyed, or scattered
// when the cockpit itself was lost. Each one is its own free-floating body: it
// tumbles through the world and physically bounces off ships, asteroids, and
// other loose parts, but deals no damage to anything it strikes (and takes none).
//
// Like Asteroid, a LoosePart is a lightweight struct whose motion is driven by a
// Chipmunk body owned by Physics; Position/Rotation/Velocity are synced back onto
// it each step for rendering.
type LoosePart struct {
	Part *Part // the freed part; carries its type and facing for rendering

	Position rl.Vector2 // world position of the part's center
	Velocity rl.Vector2 // world-space velocity in pixels/sec
	Rotation float32     // heading in radians; 0 points "up" (-Y on screen)
}

// Draw renders the loose part as a single tumbling cell, using the same routine
// as attached ship parts so debris reads identically to the part it came from.
func (l *LoosePart) Draw() {
	drawPart(l.Position, l.Rotation, l.Part)
}
