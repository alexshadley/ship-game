package main

import (
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func breakPart(t *testing.T, s *Ship, c GridCoord) {
	t.Helper()
	p, ok := s.Parts[c]
	if !ok {
		t.Fatalf("no part at %v to break", c)
	}
	p.Health = 0
}

func addTestShip(ship *Ship) (*Physics, *shipBody) {
	phys := NewPhysics(nil)
	phys.AddShip(ship, PlayerInput{})
	return phys, phys.ships[len(phys.ships)-1]
}

// TestBreakStrandsPart: the wing-tip thruster at {2,1} reaches the cockpit only
// through the block at {1,1}, so destroying that block strands the thruster.
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
	if err := ship.Validate(); err != nil {
		t.Errorf("ship left in an invalid state: %v", err)
	}
	if sb.thrusters != 1 {
		t.Errorf("expected 1 thruster remaining, got %d", sb.thrusters)
	}
}

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
	// Every part except the cockpit becomes debris; the cockpit vanishes.
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
