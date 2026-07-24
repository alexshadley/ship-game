package main

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// breakPart zeroes the health of the part at c, as a collision would.
func breakPart(t *testing.T, s *Ship, c GridCoord) {
	t.Helper()
	p, ok := s.Parts[c]
	if !ok {
		t.Fatalf("no part at %v to break", c)
	}
	p.Health = 0
}

// addTestShip registers ship with a physics space and returns its shipBody so
// tests can inspect per-ship state (mass counts) after breakage.
func addTestShip(ship *Ship) (*Physics, *shipBody) {
	phys := NewPhysics(nil)
	phys.AddShip(ship, PlayerInput{})
	return phys, phys.ships[len(phys.ships)-1]
}

// TestBreakStrandsPart: destroying the block at {1,1} disconnects the wing-tip
// control thruster at {2,1} (its only path to the cockpit ran through {1,1}), so
// the block vanishes and the thruster becomes a loose part.
func TestBreakStrandsPart(t *testing.T) {
	ship := DefaultShip(rl.NewVector2(0, 0))
	phys, sb := addTestShip(ship)

	breakPart(t, ship, GridCoord{1, 1})
	phys.handleBreakage(sb)

	if ship.Destroyed {
		t.Fatal("ship should survive losing a block")
	}
	if _, ok := ship.Parts[GridCoord{1, 1}]; ok {
		t.Error("broken block should be removed from the grid")
	}
	if _, ok := ship.Parts[GridCoord{2, 1}]; ok {
		t.Error("stranded thruster should be removed from the grid")
	}
	loose := phys.LooseParts()
	if len(loose) != 1 {
		t.Fatalf("expected 1 loose part, got %d", len(loose))
	}
	if loose[0].Part.Type != PartControlThruster {
		t.Errorf("expected the stranded thruster to be loose, got %v", loose[0].Part.Type)
	}
	// Every remaining part must still connect to the cockpit.
	if err := ship.Validate(); err != nil {
		t.Errorf("ship left in an invalid state: %v", err)
	}
	// Thruster count must drop so the ship's turn strength reflects the loss.
	if sb.thrusters != 1 {
		t.Errorf("expected 1 thruster remaining, got %d", sb.thrusters)
	}
}

// TestBreakCockpitDestroysShip: losing the cockpit scatters the whole ship.
func TestBreakCockpitDestroysShip(t *testing.T) {
	ship := DefaultShip(rl.NewVector2(0, 0))
	total := len(ship.Parts)
	phys, sb := addTestShip(ship)

	breakPart(t, ship, GridCoord{0, 0})
	phys.handleBreakage(sb)

	if !ship.Destroyed {
		t.Fatal("ship should be destroyed when the cockpit is lost")
	}
	if len(ship.Parts) != 0 {
		t.Errorf("destroyed ship should have no parts, has %d", len(ship.Parts))
	}
	// Every original part except the cockpit becomes loose debris; the cockpit
	// vanishes when the ship scatters.
	if got, want := len(phys.LooseParts()), total-1; got != want {
		t.Errorf("expected %d loose parts, got %d", want, got)
	}
	for _, lp := range phys.LooseParts() {
		if lp.Part.Type == PartCockpit {
			t.Error("cockpit should not scatter as a loose part")
		}
	}
	if sb.engines != 0 || sb.thrusters != 0 {
		t.Errorf("destroyed ship should have no engines/thrusters, got %d/%d", sb.engines, sb.thrusters)
	}
}

// TestBreakNoStranding: destroying a peripheral part strands nothing else.
func TestBreakNoStranding(t *testing.T) {
	ship := DefaultShip(rl.NewVector2(0, 0))
	phys, sb := addTestShip(ship)

	// The left cannon at {-1,0} is a leaf: nothing hangs off it.
	breakPart(t, ship, GridCoord{-1, 0})
	phys.handleBreakage(sb)

	if ship.Destroyed {
		t.Fatal("ship should survive losing a cannon")
	}
	if len(phys.LooseParts()) != 0 {
		t.Errorf("expected no loose parts, got %d", len(phys.LooseParts()))
	}
	if _, ok := ship.Parts[GridCoord{-1, 0}]; ok {
		t.Error("broken cannon should be removed")
	}
	if err := ship.Validate(); err != nil {
		t.Errorf("ship left in an invalid state: %v", err)
	}
}
