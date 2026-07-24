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
}
