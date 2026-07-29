package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

type MenuAction int

const (
	MenuNone MenuAction = iota
	MenuResume
	MenuOpenDesigner
	MenuToggleGodMode
	MenuToggleAIDebug
	MenuQuit
)

// Menu is the pause overlay opened with Escape. It draws on top of the frozen
// game frame. GodMode and AIDebug mirror the main loop's debug flags so the toggle
// rows can render their ON/OFF state; the main loop sets them each frame before
// Update/Draw and flips the flag itself when the matching toggle action comes back.
type Menu struct {
	GodMode bool
	AIDebug bool
}

type menuItem struct {
	label  string
	action MenuAction
}

// items builds the current menu rows, folding live debug state into the toggle
// labels. Rebuilt each frame so the ON/OFF text tracks the flags.
func (m *Menu) items() []menuItem {
	return []menuItem{
		{"Resume", MenuResume},
		{"Ship Designer", MenuOpenDesigner},
		{"God Mode: " + onOff(m.GodMode), MenuToggleGodMode},
		{"AI Debug: " + onOff(m.AIDebug), MenuToggleAIDebug},
		{"Quit", MenuQuit},
	}
}

func onOff(on bool) string {
	if on {
		return "ON"
	}
	return "OFF"
}

// Update handles input and returns the chosen action (MenuNone if still open).
// Toggle actions leave the menu open; the main loop applies them and the label
// updates on the next frame.
func (m *Menu) Update() MenuAction {
	items := m.items()
	for i := range items {
		if uiButtonClicked(menuButtonRect(i)) {
			return items[i].action
		}
	}
	return MenuNone
}

func (m *Menu) Draw() {
	rl.DrawRectangle(0, 0, windowWidth, windowHeight, rl.NewColor(0, 0, 0, 150))

	const titleSize = 64
	title := "SHIP GAME"
	tw := rl.MeasureText(title, titleSize)
	rl.DrawText(title, (windowWidth-tw)/2, windowHeight/2-320, titleSize, uiText)

	for i, it := range m.items() {
		r := menuButtonRect(i)
		uiButtonRect(r, it.label, 32, false)
	}
}

func menuButtonRect(i int) rl.Rectangle {
	const (
		w      = 420
		h      = 72
		gap    = 24
		startY = float32(windowHeight)/2 - 160
	)
	return rl.NewRectangle(float32(windowWidth-w)/2, startY+float32(i)*(h+gap), w, h)
}
