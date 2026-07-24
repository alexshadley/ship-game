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

// TestBreakStrandsPart: the block at {1,0} is the only path to the cockpit for
// both the right engine at {1,1} and the right wing-tip thruster at {2,0}, so
// destroying it strands both.
func TestBreakStrandsPart(t *testing.T) {
	ship := DefaultShip(rl.NewVector2(0, 0))
	phys, sb := addTestShip(ship)

	breakPart(t, ship, GridCoord{1, 0})
	phys.handleBreakage(sb)

	if ship.Destroyed {
		t.Fatal("ship should survive losing a block")
	}
	if _, ok := ship.Parts[GridCoord{1, 0}]; ok {
		t.Error("broken block should be removed from the grid")
	}
	if _, ok := ship.Parts[GridCoord{1, 1}]; ok {
		t.Error("stranded engine should be removed from the grid")
	}
	if _, ok := ship.Parts[GridCoord{2, 0}]; ok {
		t.Error("stranded thruster should be removed from the grid")
	}
	loose := phys.LooseParts()
	if len(loose) != 2 {
		t.Fatalf("expected 2 loose parts, got %d", len(loose))
	}
	looseTypes := map[PartType]bool{}
	for _, lp := range loose {
		looseTypes[lp.Part.Type] = true
	}
	if !looseTypes[PartEngine] || !looseTypes[PartControlThruster] {
		t.Errorf("expected the stranded engine and thruster to be loose, got %v", looseTypes)
	}
	if err := ship.Validate(); err != nil {
		t.Errorf("ship left in an invalid state: %v", err)
	}
	if sb.engines != 1 {
		t.Errorf("expected 1 engine remaining, got %d", sb.engines)
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

	// The left cannon at {-1,-1} is a leaf: it hangs off the block spine and the
	// left flank block, and nothing hangs off it.
	breakPart(t, ship, GridCoord{-1, -1})
	phys.handleBreakage(sb)

	if ship.Destroyed {
		t.Fatal("ship should survive losing a cannon")
	}
	if len(phys.LooseParts()) != 0 {
		t.Errorf("expected no loose parts, got %d", len(phys.LooseParts()))
	}
	if _, ok := ship.Parts[GridCoord{-1, -1}]; ok {
		t.Error("broken cannon should be removed")
	}
	if err := ship.Validate(); err != nil {
		t.Errorf("ship left in an invalid state: %v", err)
	}
}
