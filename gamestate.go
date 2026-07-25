package main

// GameState is the top-level mode of the application: piloting the ship, sitting
// in the pause menu, editing ships in the designer, or outfitting the current
// ship in the shop.
type GameState int

const (
	StatePlaying GameState = iota
	StateMenu
	StateDesigner
	StateShop
)
