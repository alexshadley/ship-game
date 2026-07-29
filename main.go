package main

import (
	"flag"
	"log"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	gameWidth  = 640
	gameHeight = 360

	// Logical UI resolution the menu and designer lay out against. The actual
	// window can be any size (see the -res/-fullscreen flags); the UI renders into
	// a texture at this resolution and is scaled to fit, so this stays fixed.
	windowWidth  = 1920
	windowHeight = 1080

	pilotingZoom      = 0.25
	spacewalkZoom     = 0.7
	zoomEaseSpeed     = 3.0
	cameraFollowSpeed = 20.0

	// While piloting, the scroll wheel and the -/= keys adjust the zoom between
	// these bounds. pilotZoomMin frames the widest swath of the battlefield (most
	// zoomed out); pilotZoomMax matches the starting pilotingZoom, so the default
	// view is the most zoomed-in you can get and the controls only pull back out.
	pilotZoomMin  = 0.1
	pilotZoomMax  = pilotingZoom
	pilotZoomStep = 0.01
	// Per-second zoom rate while a zoom key is held (trackpad users have no wheel).
	pilotZoomKeyRate = 0.25

	// A stage lasts this many seconds; survive to the end to clear it.
	stageDuration = 180.0
)

func main() {
	// Window sizing: -res WxH picks an explicit window size, -fullscreen starts
	// fullscreen at the monitor's native resolution. With neither, the window is
	// fitted to the current monitor so it never opens larger than the screen.
	resFlag := flag.String("res", "", "window resolution as WxH (e.g. 1280x720); default fits the monitor")
	fullscreenFlag := flag.Bool("fullscreen", false, "start in fullscreen at the monitor's native resolution")
	flag.Parse()

	// Resolve the windowed size: explicit -res, else a fit to the current monitor.
	// Probe the monitor with a throwaway window first, then create the real window
	// once at the final size. Resizing a window after InitWindow desyncs the GL
	// framebuffer from the drawable on HiDPI Macs and renders the scene into a
	// corner, so we deliberately avoid SetWindowSize.
	rl.InitWindow(windowWidth, windowHeight, "Ship Game")
	mon := rl.GetCurrentMonitor()
	monW, monH := rl.GetMonitorWidth(mon), rl.GetMonitorHeight(mon)
	winW, winH := fitToMonitor(int32(monW), int32(monH))
	if w, h, ok := parseResolution(*resFlag); ok {
		winW, winH = w, h
	} else if *resFlag != "" {
		log.Printf("ignoring invalid -res %q; expected WxH like 1280x720", *resFlag)
	}
	rl.CloseWindow()

	rl.InitWindow(winW, winH, "Ship Game")
	defer rl.CloseWindow()
	// Center the window on the monitor.
	rl.SetWindowPosition((monW-int(winW))/2, (monH-int(winH))/2)
	if *fullscreenFlag {
		toggleFullscreen(winW, winH)
	}

	rl.SetTargetFPS(60)
	// Escape opens the pause menu rather than closing the window (raylib's default).
	rl.SetExitKey(rl.KeyNull)

	g := &Game{
		winW: winW,
		winH: winH,
	}

	g.target = rl.LoadRenderTexture(gameWidth, gameHeight)
	defer rl.UnloadRenderTexture(g.target)

	// The menu and designer draw at the logical UI resolution into this texture,
	// which is then scaled to the window alongside the game world.
	g.uiTarget = rl.LoadRenderTexture(windowWidth, windowHeight)
	defer rl.UnloadRenderTexture(g.uiTarget)

	var err error
	g.ship, err = LoadPlayerShip(rl.NewVector2(0, 0))
	if err != nil {
		log.Fatalf("failed to load player ship: %v", err)
	}

	// Target is nudged up half a cell because the ship's body extends forward of
	// the cockpit, so this reads as roughly centered.
	g.camera = rl.Camera2D{
		Target:   rl.NewVector2(0, -cellSize*0.5),
		Offset:   rl.NewVector2(gameWidth/2, gameHeight/2),
		Rotation: 0,
		Zoom:     pilotingZoom,
	}

	// Height is negative: OpenGL render textures have their origin at bottom-left,
	// so the source must be flipped vertically. The destination (viewport) is
	// computed each frame from the current window size.
	g.src = rl.NewRectangle(0, 0, float32(g.target.Texture.Width), -float32(g.target.Texture.Height))
	g.uiSrc = rl.NewRectangle(0, 0, float32(g.uiTarget.Texture.Width), -float32(g.uiTarget.Texture.Height))

	g.asteroids = DefaultAsteroids()
	g.physics = NewPhysics(g.asteroids)
	g.particles = NewParticleSystem()

	// Piloting zoom starts fully zoomed in (see pilotingZoom).
	g.pilotZoom = pilotingZoom

	// Counts down from stageDuration while piloting. Surviving until it hits zero
	// ends the round and drops the player into the shop to refit before embarking
	// on the next one.
	g.stageTimer = stageDuration

	// The player's wallet and the parts they own. Both are shared with the shop
	// (by pointer / by reference), so buying and fitting parts there carries back
	// into the running game. Inventory holds only unplaced parts — bought or pulled
	// off the ship — so it starts empty (the starting ship is already assembled) and
	// does not persist between games. Money starts at zero; sell parts to earn some.
	g.inventory = map[PartType]int{}

	g.state = StatePlaying

	g.physics.AddShip(g.ship, PilotInput{
		spacewalking: &g.spacewalking,
		player: PlayerInput{
			camera:   &g.camera,
			ship:     g.ship,
			autoFire: &g.autoWeapons,
			enemies:  func() []*Ship { return g.enemies },
		},
	})

	// Enemies arrive from offscreen: one at the start, then another every
	// enemySpawnInterval seconds.
	g.spawnEnemy()
	g.enemySpawnTimer = enemySpawnInterval

	// Wire debug hooks: mark the player's ship and share the god-mode flag so the
	// damage handlers can spare it, then scatter salvageable parts to scavenge.
	g.physics.playerShip = g.ship
	g.physics.godMode = &g.godMode
	g.physics.SeedLooseParts(3)

	// Each frame: advance the game state, then render it. Keeping the two passes
	// separate and ordered (update before draw) is the whole point of this split —
	// all state mutation lives in Update, all drawing in Draw.
	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()
		g.Update(dt)
		if g.quit {
			return
		}
		g.Draw()
	}
}

func emitExhaust(ship *Ship, controls Controls, particles *ParticleSystem) {
	if controls.Thrust > 0 {
		for _, src := range ship.EngineExhaustSources() {
			for i := 0; i < engineParticlesPerFrame; i++ {
				particles.Emit(src.Pos, src.Dir, ship.Velocity, engineExhaustColor)
			}
		}
	}

	turn := 0
	if controls.Turn < 0 {
		turn = -1
	} else if controls.Turn > 0 {
		turn = 1
	}
	if turn != 0 {
		for _, src := range ship.ControlThrusterExhaustSources(turn) {
			for i := 0; i < thrusterParticlesPerFrame; i++ {
				particles.Emit(src.Pos, src.Dir, ship.Velocity, thrusterExhaustColor)
			}
		}
	}
}
