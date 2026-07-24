package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	screenWidth  = 800
	screenHeight = 600
	playerSize   = 50
	playerSpeed  = 300 // pixels per second
)

func main() {
	rl.InitWindow(screenWidth, screenHeight, "Ship Game")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	player := rl.NewVector2(screenWidth/2-playerSize/2, screenHeight/2-playerSize/2)

	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()
		move := playerSpeed * dt

		if rl.IsKeyDown(rl.KeyD) {
			player.X += move
		}
		if rl.IsKeyDown(rl.KeyA) {
			player.X -= move
		}
		if rl.IsKeyDown(rl.KeyS) {
			player.Y += move
		}
		if rl.IsKeyDown(rl.KeyW) {
			player.Y -= move
		}

		// Keep the square inside the window.
		player.X = rl.Clamp(player.X, 0, screenWidth-playerSize)
		player.Y = rl.Clamp(player.Y, 0, screenHeight-playerSize)

		rl.BeginDrawing()
		rl.ClearBackground(rl.RayWhite)
		rl.DrawRectangleV(player, rl.NewVector2(playerSize, playerSize), rl.Maroon)
		rl.EndDrawing()
	}
}
