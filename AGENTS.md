# AGENTS.md

Guidance for AI agents working in this repository.

## Project

A 2D space ship game written in Go.

- **Language:** Go (see `go.mod` for version).
- **Rendering & input:** [raylib-go](https://github.com/gen2brain/raylib-go) (`rl`).
- **Physics:** [Chipmunk 2D](https://github.com/jakecoffman/cp) (`cp`) — the ship is a single
  rigid body; see `physics.go`.
- **Run:** `go run .`  · **Build:** `go build`  · **Check:** `go build ./... && go vet ./...`

All source is flat in the repo root, `package main`: `main.go` (window + game loop),
`ship.go`, `part.go`, `physics.go`, `asteroid.go`, `projectile.go`.

## Working here

- **Tests:** Don't write tests unless asked.
- **Comments:** Don't write comments unless asked. Keep comments miserly — write one
  only when the code does something counterintuitive or unexpected (a sign flip, a
  library quirk, a constraint, something that would otherwise read like a bug). Never
  comment what the code plainly says.

## Conventions

### Coordinates are centers

**By convention, an object's position (`Position`, and grid coordinates) refers to the
center of that object, not a corner.** This holds throughout the codebase:

- A ship part at `GridCoord{X, Y}` is centered at `(X*cellSize, Y*cellSize)` in the
  ship's local pixel frame; the cockpit is at grid `{0,0}`.
- `Ship.Position`, `Asteroid.Position`, and `Projectile.Position` are all world-space
  centers.
- Drawing follows suit: cells, boxes, and sprites are drawn about their center (e.g.
  `drawCell` and `Projectile.Draw` pass an origin of half the size to raylib's
  `*Pro` draw calls).

When you add a new positioned entity or a draw routine, keep position at the center.

### Frames and directions

- **Ship grid:** `+X` is right, `+Y` is toward the rear. `Facing.offset()` gives the unit
  grid step for a facing (`FacingUp = {0,-1}` = toward the nose).
- **Local forward is `-Y`** (toward the cockpit/nose). Rotate a local vector into world
  space with the ship's `Direction` (radians; `0` points up / `-Y` on screen).
  `Ship.worldPoint` does this for points.
- **Rotation:** radians, `0` = up, increasing **clockwise** to match screen space
  (`+X` right, `+Y` down). Convert to degrees (`* 180 / math.Pi`) only at raylib draw calls.

### Adding a part type

A new `PartType` touches `part.go` in three places — the `const` block, `String()`, and the
`partSpecs` map (health/weight/color; a missing entry yields a zero-value spec) — plus a
render case in `Ship.Draw` (`ship.go`). Add validation in `Ship.Validate` if the part has
placement rules, and count it in `NewPhysics` if it affects mass/behavior.
