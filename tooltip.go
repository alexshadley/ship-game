package main

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type tooltipLine struct {
	text  string
	color rl.Color
}

var (
	tooltipPanel    = rl.NewColor(16, 18, 24, 240)
	tooltipBorder   = rl.NewColor(90, 110, 140, 255)
	tooltipModifier = rl.NewColor(255, 210, 100, 255)
)

func partTooltipLines(p *Part) []tooltipLine {
	title := p.Type.String()
	if p.Type.isLeveled() {
		title = fmt.Sprintf("%s  Lv %d", title, clampPartLevel(p.Level))
	}
	lines := []tooltipLine{{title, uiAccent}}

	if p.Type.isLeveled() {
		for _, m := range p.Modifiers {
			if !m.valid() {
				continue
			}
			spec := modifierSpecs[m]
			lines = append(lines, tooltipLine{spec.name + "  " + spec.desc, tooltipModifier})
		}
	}

	lines = append(lines, tooltipLine{fmt.Sprintf("Hull %.0f / %.0f", p.Health, partSpecs[p.Type].health), uiTextDim})
	switch {
	case p.Type.isWeapon():
		lines = append(lines, tooltipLine{fmt.Sprintf("Damage %.0f", p.weaponDamage()), uiText})
		lines = append(lines, tooltipLine{fmt.Sprintf("Fire rate %.1f/s", 1/p.weaponFireInterval()), uiText})
	case p.Type == PartShield:
		lines = append(lines, tooltipLine{fmt.Sprintf("Capacity %.0f", p.shieldMax()), uiText})
		lines = append(lines, tooltipLine{fmt.Sprintf("Regen %.1f/s", p.shieldRegen()), uiText})
	}
	return lines
}

func drawTooltip(lines []tooltipLine, anchor rl.Vector2, fontSize, boundsW, boundsH int32) {
	if len(lines) == 0 {
		return
	}
	pad := fontSize / 2
	lineH := fontSize + fontSize/4

	var textW int32
	for _, l := range lines {
		if w := rl.MeasureText(l.text, fontSize); w > textW {
			textW = w
		}
	}
	w := textW + pad*2
	h := int32(len(lines))*lineH - fontSize/4 + pad*2

	x := int32(anchor.X) + fontSize
	y := int32(anchor.Y) + fontSize
	if x+w > boundsW {
		x = int32(anchor.X) - fontSize - w
	}
	if y+h > boundsH {
		y = boundsH - h
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	rl.DrawRectangle(x, y, w, h, tooltipPanel)
	rl.DrawRectangleLines(x, y, w, h, tooltipBorder)
	ty := y + pad
	for _, l := range lines {
		rl.DrawText(l.text, x+pad, ty, fontSize, l.color)
		ty += lineH
	}
}

func hoveredWorldPart(physics *Physics, ship *Ship, enemies []*Ship, camera rl.Camera2D) *Part {
	wp := mouseWorld(camera)
	if p := ship.partAtWorld(wp); p != nil {
		return p
	}
	for _, e := range enemies {
		if p := e.partAtWorld(wp); p != nil {
			return p
		}
	}
	for _, l := range physics.LooseParts() {
		if looseBlastDist(l, wp) == 0 {
			return l.Part
		}
	}
	return nil
}
