package main

// GameState is the top-level mode of the application: piloting the ship, sitting
// in the pause menu, or editing ships in the designer.
type GameState int

const (
	StatePlaying GameState = iota
	StateMenu
	StateDesigner
)
