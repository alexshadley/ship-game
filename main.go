package main

import (
	"log"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	screenWidth  = 800
	screenHeight = 600
)

func main() {
	rl.InitWindow(screenWidth, screenHeight, "Ship Game")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	// Place the ship in the middle of the screen. Nudge up by one cell so the
	// body (which extends behind the cockpit) reads as roughly centered.
	origin := rl.NewVector2(screenWidth/2, screenHeight/2-cellSize)
	ship := DefaultShip(origin)
	if err := ship.Validate(); err != nil {
		log.Printf("default ship is invalid: %v", err)
	}

	asteroids := DefaultAsteroids()
	physics := NewPhysics(ship)

	for !rl.WindowShouldClose() {
		physics.Update(float64(rl.GetFrameTime()))

		rl.BeginDrawing()
		rl.ClearBackground(rl.RayWhite)
		for _, a := range asteroids {
			a.Draw()
		}
		ship.Draw()
		rl.EndDrawing()
	}
}
