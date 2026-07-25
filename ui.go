package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// The menu and designer lay out at the logical UI resolution (windowWidth×
// windowHeight) and render through a texture that's scaled to fit the window, so
// mouse positions are mapped back into that logical space via uiMouse. These
// helpers hand-roll the few widgets they need.

var (
	uiPanel     = rl.NewColor(24, 28, 36, 255)
	uiButton    = rl.NewColor(44, 52, 66, 255)
	uiButtonHot = rl.NewColor(66, 80, 104, 255)
	uiAccent    = rl.NewColor(120, 180, 255, 255)
	uiText      = rl.RayWhite
	uiTextDim   = rl.NewColor(150, 160, 175, 255)
)

func mouseIn(r rl.Rectangle) bool {
	return rl.CheckCollisionPointRec(uiMouse(), r)
}

// uiButtonRect draws a labeled button and reports whether it was clicked this
// frame. When selected is true it's drawn with the accent outline to read as the
// active choice.
func uiButtonRect(r rl.Rectangle, label string, fontSize int32, selected bool) bool {
	hot := mouseIn(r)
	fill := uiButton
	if hot {
		fill = uiButtonHot
	}
	rl.DrawRectangleRec(r, fill)
	if selected {
		rl.DrawRectangleLinesEx(r, 2, uiAccent)
	}
	tw := rl.MeasureText(label, fontSize)
	rl.DrawText(label, int32(r.X+(r.Width-float32(tw))/2), int32(r.Y+(r.Height-float32(fontSize))/2), fontSize, uiText)
	return hot && rl.IsMouseButtonPressed(rl.MouseLeftButton)
}
