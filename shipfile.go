package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// shipsDir is where ship designs live as JSON files. The designer reads and
// writes here, and enemies are spawned from these files.
const shipsDir = "ships"

// partJSON and shipDesign are the on-disk form of a ship. Part types and facings
// are stored by their human-readable String() names so the files stay legible and
// hand-editable.
type partJSON struct {
	X         int      `json:"x"`
	Y         int      `json:"y"`
	Type      string   `json:"type"`
	Facing    string   `json:"facing"`
	Level     int      `json:"level,omitempty"`
	Modifiers []string `json:"modifiers,omitempty"`
}

type shipDesign struct {
	Parts []partJSON `json:"parts"`
}

func partTypeFromString(s string) (PartType, bool) {
	for t := PartCockpit; t < partTypeCount; t++ {
		if t.String() == s {
			return t, true
		}
	}
	return 0, false
}

func facingFromString(s string) (Facing, bool) {
	for f := FacingUp; f <= FacingLeft; f++ {
		if f.String() == s {
			return f, true
		}
	}
	return 0, false
}

// ShipToDesign flattens a ship's part grid into a serializable design, dropping
// the runtime state (position, velocity, health) that a saved layout doesn't need.
func ShipToDesign(s *Ship) shipDesign {
	parts := make([]partJSON, 0, len(s.Parts))
	for c, p := range s.Parts {
		pj := partJSON{
			X:      c.X,
			Y:      c.Y,
			Type:   p.Type.String(),
			Facing: p.Facing.String(),
		}
		if p.Type.isLeveled() {
			pj.Level = clampPartLevel(p.Level)
			for _, m := range p.Modifiers {
				if m.valid() {
					pj.Modifiers = append(pj.Modifiers, m.String())
				}
			}
		}
		parts = append(parts, pj)
	}
	// Stable order so saved files diff cleanly regardless of map iteration order.
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].Y != parts[j].Y {
			return parts[i].Y < parts[j].Y
		}
		return parts[i].X < parts[j].X
	})
	return shipDesign{Parts: parts}
}

// DesignToShip rebuilds a ship at pos from a design, skipping any part whose type
// or facing name isn't recognized.
func DesignToShip(d shipDesign, pos rl.Vector2) *Ship {
	s := NewShip(pos)
	for _, pj := range d.Parts {
		t, ok := partTypeFromString(pj.Type)
		if !ok {
			continue
		}
		f, ok := facingFromString(pj.Facing)
		if !ok {
			f = FacingUp
		}
		part := NewLeveledPart(t, f, pj.Level)
		for _, name := range pj.Modifiers {
			if m, ok := partModifierFromString(name); ok {
				part.Modifiers = append(part.Modifiers, m)
			}
		}
		if part.Type == PartShield {
			part.ShieldHealth = part.shieldMax()
		}
		s.AddPart(GridCoord{X: pj.X, Y: pj.Y}, part)
	}
	return s
}

// LoadShipFile reads a ship design from path and instantiates it at pos.
func LoadShipFile(path string, pos rl.Vector2) (*Ship, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d shipDesign
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return DesignToShip(d, pos), nil
}

// LoadPlayerShip loads the player's design from ships/player.json, falling back
// to the built-in DefaultShip if the file is missing or invalid.
func LoadPlayerShip(pos rl.Vector2) *Ship {
	ship, err := LoadShipFile(filepath.Join(shipsDir, "player.json"), pos)
	if err != nil || ship.Validate() != nil {
		return DefaultShip(pos)
	}
	return ship
}

// SaveShipFile writes ship to shipsDir under name (a ".json" suffix is added if
// missing), creating the directory if needed. It returns the path written.
func SaveShipFile(name string, ship *Ship) (string, error) {
	if err := os.MkdirAll(shipsDir, 0o755); err != nil {
		return "", err
	}
	if !strings.HasSuffix(name, ".json") {
		name += ".json"
	}
	path := filepath.Join(shipsDir, name)
	data, err := json.MarshalIndent(ShipToDesign(ship), "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ListShipFiles returns the base names of every ship design in shipsDir, sorted.
// A missing directory is treated as empty rather than an error.
func ListShipFiles() []string {
	entries, err := os.ReadDir(shipsDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}
