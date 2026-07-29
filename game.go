package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Game holds all of the running game's state. It was previously a tangle of
// locals in main(); collecting it here lets the frame be split into two ordered
// passes — Update mutates state, Draw renders it — with main() just sequencing
// the two each frame.
type Game struct {
	// Window and render targets. winW/winH are the windowed size, restored when
	// leaving fullscreen. target holds the game world at gameWidth×gameHeight;
	// uiTarget holds the menu/designer at the logical UI resolution. src/uiSrc are
	// the (vertically flipped) source rects used to blit each to the window.
	winW, winH int32
	target     rl.RenderTexture2D
	uiTarget   rl.RenderTexture2D
	src        rl.Rectangle
	uiSrc      rl.Rectangle

	ship   *Ship
	camera rl.Camera2D

	asteroids []*Asteroid
	physics   *Physics
	particles *ParticleSystem

	player       Player
	scavenger    Scavenger
	repair       RepairTool
	spacewalking bool

	// User-controlled piloting zoom, nudged by the scroll wheel. Scroll changes
	// snap in instantly; only entering/exiting the ship eases the zoom smoothly
	// between the piloting and spacewalk framing (tracked by zoomEasing).
	pilotZoom        float32
	prevSpacewalking bool
	zoomEasing       bool

	// gameOver freezes the simulation once the astronaut dies (or the cockpit is
	// lost while aboard); only the render pass keeps running so the banner stays up.
	gameOver bool

	// stageTimer counts down from stageDuration while piloting; reaching zero ends
	// the round and drops the player into the shop.
	stageTimer float32

	godMode     bool
	aiDebug     bool
	autoWeapons bool

	enemies         []*Ship
	enemyAIs        []*EnemyAI
	enemySpawnTimer float32

	projectiles []*Projectile

	state    GameState
	menu     Menu
	designer *Designer

	// money and inventory are shared (by pointer / by reference) with the shop, so
	// buying and fitting parts there carries back into the running game.
	money     int
	inventory map[PartType]int

	// designerDone is set by the designer/shop's immediate-mode Frame() during the
	// draw pass; the next Update picks it up to apply the resulting state change.
	designerDone bool

	// quit is raised by the pause menu's Quit action; main's loop breaks on it.
	quit bool
}
