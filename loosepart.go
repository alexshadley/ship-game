package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

type LoosePart struct {
	Part *Part

	Position rl.Vector2
	Velocity rl.Vector2
	Rotation float32
}

func (l *LoosePart) Draw() {
	drawPart(l.Position, l.Rotation, l.Part)

	// Debris is destructible, so show a shrinking health bar once it's been chipped
	// — the same readout ship parts get (see Ship.Draw).
	if maxHealth := partSpecs[l.Part.Type].health; l.Part.Health < maxHealth {
		drawHealthBar(l.Position, l.Part.Health/maxHealth)
	}
}
