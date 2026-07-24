package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

type MenuAction int

const (
	MenuNone MenuAction = iota
	MenuResume
	MenuOpenDesigner
	MenuQuit
)

// Menu is the pause overlay opened with Escape. It draws on top of the frozen
// game frame.
type Menu struct{}

type menuItem struct {
	label  string
	action MenuAction
}

var menuItems = []menuItem{
	{"Resume", MenuResume},
	{"Ship Designer", MenuOpenDesigner},
	{"Quit", MenuQuit},
}

// Update handles input and returns the chosen action (MenuNone if still open).
func (m *Menu) Update() MenuAction {
	for i := range menuItems {
		if mouseIn(menuButtonRect(i)) && rl.IsMouseButtonPressed(rl.MouseLeftButton) {
			return menuItems[i].action
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

	for i, it := range menuItems {
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
