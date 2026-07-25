package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Designer is the in-app ship editor reached from the pause menu. It reads and
// writes JSON designs in the ships/ folder: the left panel lists existing ships
// to load, the center is a grid you paint parts onto, and the right panel picks
// the part type and facing. It draws directly to the window (not through the
// game's low-res render texture) and runs its own input, so it owns a full frame
// via Frame().
type Designer struct {
	camera    rl.Camera2D
	ship      *Ship
	name      string
	files     []string
	selType   PartType
	selFacing Facing

	editingName bool
	status      string
	statusColor rl.Color

	// armed gates grid editing until the mouse button has been released once, so
	// the click that opened the designer from the menu doesn't also place a part.
	armed bool

	// shop is non-nil when the designer is opened as the shop: it edits the live
	// player ship directly, parts come from a limited inventory rather than an
	// endless palette, and the left panel sells parts for money. See NewShop.
	shop *ShopState
}

const (
	dsLeftPanelW = 320
	dsRightPanel = 1600
	dsRightW     = windowWidth - dsRightPanel
	dsTopBar     = 64
	dsBottomBar  = 40
)

// palettePartTypes is the placeable set, in enum order, derived from AllPartTypes
// so every part type is automatically available in the designer (numbered keys
// 1..N). The cockpit is included so a fresh ship can be built from scratch;
// validation keeps the "exactly one" rule.
var palettePartTypes = AllPartTypes()

func NewDesigner() *Designer {
	d := &Designer{
		camera: rl.Camera2D{
			Target: rl.NewVector2(0, 0),
			Offset: rl.NewVector2(dsLeftPanelW+(dsRightPanel-dsLeftPanelW)/2, dsTopBar+(windowHeight-dsTopBar-dsBottomBar)/2),
			Zoom:   1.6,
		},
		selType:   PartBlock,
		selFacing: FacingUp,
	}
	d.refreshFiles()
	// Open the first existing design if there is one, otherwise start fresh.
	if len(d.files) > 0 {
		d.load(d.files[0])
	} else {
		d.newShip()
	}
	return d
}

// NewShop opens the designer in shop mode against the live player ship. Money and
// inventory are shared with the caller (the running game), so purchases and part
// placements persist after the shop closes. Unlike the plain designer it doesn't
// touch ship files: it edits ship in place and never saves.
func NewShop(ship *Ship, money *int, inventory map[PartType]int) *Designer {
	d := &Designer{
		camera: rl.Camera2D{
			Target: rl.NewVector2(0, 0),
			Offset: rl.NewVector2(dsLeftPanelW+(dsRightPanel-dsLeftPanelW)/2, dsTopBar+(windowHeight-dsTopBar-dsBottomBar)/2),
			Zoom:   1.6,
		},
		selType:   PartBlock,
		selFacing: FacingUp,
		ship:      ship,
		name:      "Your Ship",
		shop: &ShopState{
			money:     money,
			inventory: inventory,
			offers:    shopOffers(),
		},
	}
	d.setStatus("Buy parts, then place them on your ship", uiTextDim)
	return d
}

func (d *Designer) refreshFiles() { d.files = ListShipFiles() }

func (d *Designer) newShip() {
	s := NewShip(rl.NewVector2(0, 0))
	s.AddPart(GridCoord{0, 0}, NewPart(PartCockpit, FacingUp))
	d.ship = s
	d.name = "untitled"
	d.setStatus("New ship", uiTextDim)
}

func (d *Designer) load(file string) {
	ship, err := LoadShipFile(shipsDir+"/"+file, rl.NewVector2(0, 0))
	if err != nil {
		d.setStatus("Load failed: "+err.Error(), rl.Red)
		return
	}
	d.ship = ship
	d.name = strings.TrimSuffix(file, ".json")
	d.setStatus("Loaded "+file, uiTextDim)
}

func (d *Designer) save() {
	name := sanitizeShipName(d.name)
	if name == "" {
		d.setStatus("Name the ship before saving", rl.Red)
		return
	}
	path, err := SaveShipFile(name, d.ship)
	if err != nil {
		d.setStatus("Save failed: "+err.Error(), rl.Red)
		return
	}
	d.name = name
	d.refreshFiles()
	d.setStatus("Saved "+path, rl.Lime)
}

func (d *Designer) setStatus(msg string, c rl.Color) {
	d.status = msg
	d.statusColor = c
}

// Frame runs one frame of the designer (input + draw). It must be called inside
// BeginDrawing/EndDrawing. It returns true when the user wants to return to the
// pause menu.
func (d *Designer) Frame() bool {
	exit := false

	// Escape leaves the designer. While naming, it cancels the text field instead.
	if rl.IsKeyPressed(rl.KeyEscape) {
		if d.editingName {
			d.editingName = false
		} else {
			return true
		}
	}

	if d.editingName {
		d.updateNameInput()
	} else {
		d.updateShortcuts()
	}

	if !d.armed && rl.IsMouseButtonUp(rl.MouseLeftButton) {
		d.armed = true
	}

	d.updateZoom()
	d.updateGridEdit()

	rl.ClearBackground(rl.NewColor(16, 18, 24, 255))
	d.drawGrid()
	d.leftPanel()
	d.rightPanel(&exit)
	if d.shop == nil {
		d.drawNameField()
	} else {
		rl.DrawText("Outfitting: "+d.name, dsLeftPanelW+40, 22, 24, uiText)
	}
	d.drawHint()

	return exit
}

func (d *Designer) updateNameInput() {
	for {
		ch := rl.GetCharPressed()
		if ch == 0 {
			break
		}
		if ch >= 32 && ch < 127 && len(d.name) < 24 {
			d.name += string(rune(ch))
		}
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(d.name) > 0 {
		d.name = d.name[:len(d.name)-1]
	}
	if rl.IsKeyPressed(rl.KeyEnter) {
		d.editingName = false
	}
}

func (d *Designer) updateShortcuts() {
	// Number-row shortcuts 1..9 select the first nine parts; any beyond that are
	// still placeable via the palette buttons.
	for i, t := range palettePartTypes {
		if i >= 9 {
			break
		}
		if rl.IsKeyPressed(rl.KeyOne + int32(i)) {
			d.selType = t
		}
	}
	if rl.IsKeyPressed(rl.KeyR) {
		d.selFacing = (d.selFacing + 1) % 4
	}
}

func (d *Designer) updateZoom() {
	if wheel := rl.GetMouseWheelMove(); wheel != 0 && mouseIn(d.gridRegion()) {
		d.camera.Zoom = clamp(d.camera.Zoom+wheel*0.15, 0.5, 4)
	}
}

func (d *Designer) updateGridEdit() {
	if d.editingName || !mouseIn(d.gridRegion()) {
		return
	}
	if !d.armed {
		return
	}
	c := d.hoveredCell()
	if d.shop != nil {
		d.updateShopGridEdit(c)
		return
	}
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		d.ship.Parts[c] = NewPart(d.selType, d.selFacing)
	}
	if rl.IsMouseButtonPressed(rl.MouseRightButton) {
		delete(d.ship.Parts, c)
	}
}

// updateShopGridEdit is the shop's grid editing: placing a part spends one from
// inventory and removing it returns it there, so the palette counts stay honest.
// The cockpit is fixed — it can't be placed (never in inventory) or removed.
func (d *Designer) updateShopGridEdit(c GridCoord) {
	inv := d.shop.inventory
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		if inv[d.selType] <= 0 {
			d.setStatus("No "+d.selType.String()+" in inventory - buy one first", rl.Red)
			return
		}
		if existing, ok := d.ship.Parts[c]; ok {
			if existing.Type == PartCockpit {
				return // never build over the cockpit
			}
			inv[existing.Type]++ // the part being replaced goes back to inventory
		}
		inv[d.selType]--
		d.ship.Parts[c] = NewPart(d.selType, d.selFacing)
	}
	if rl.IsMouseButtonPressed(rl.MouseRightButton) {
		existing, ok := d.ship.Parts[c]
		if !ok || existing.Type == PartCockpit {
			return
		}
		inv[existing.Type]++
		delete(d.ship.Parts, c)
	}
}

func (d *Designer) gridRegion() rl.Rectangle {
	return rl.NewRectangle(dsLeftPanelW, dsTopBar, dsRightPanel-dsLeftPanelW, windowHeight-dsTopBar-dsBottomBar)
}

func (d *Designer) hoveredCell() GridCoord {
	w := rl.GetScreenToWorld2D(rl.GetMousePosition(), d.camera)
	return GridCoord{
		X: int(math.Round(float64(w.X / cellSize))),
		Y: int(math.Round(float64(w.Y / cellSize))),
	}
}

func (d *Designer) drawGrid() {
	region := d.gridRegion()
	rl.BeginScissorMode(int32(region.X), int32(region.Y), int32(region.Width), int32(region.Height))
	rl.BeginMode2D(d.camera)

	const half = 24
	gridLine := rl.NewColor(40, 46, 58, 255)
	for i := -half; i <= half; i++ {
		f := float32(i)*cellSize - cellSize/2
		rl.DrawLineV(rl.NewVector2(f, -half*cellSize), rl.NewVector2(f, half*cellSize), gridLine)
		rl.DrawLineV(rl.NewVector2(-half*cellSize, f), rl.NewVector2(half*cellSize, f), gridLine)
	}
	// Emphasize the origin (cockpit anchor) axes.
	axis := rl.NewColor(70, 80, 100, 255)
	rl.DrawLineV(rl.NewVector2(0, -half*cellSize), rl.NewVector2(0, half*cellSize), axis)
	rl.DrawLineV(rl.NewVector2(-half*cellSize, 0), rl.NewVector2(half*cellSize, 0), axis)

	for c, p := range d.ship.Parts {
		center := rl.NewVector2(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		drawPart(center, 0, p)
	}

	// Placement preview under the cursor.
	if mouseIn(region) && !d.editingName {
		c := d.hoveredCell()
		center := rl.NewVector2(float32(c.X)*cellSize, float32(c.Y)*cellSize)
		preview := partSpecs[d.selType].color
		preview.A = 130
		drawPartColored(center, 0, NewPart(d.selType, d.selFacing), preview)
	}

	d.drawBalanceOverlay()

	rl.EndMode2D()
	rl.EndScissorMode()
}

// drawBalanceOverlay reuses the game's placement-HUD balance readout: the center
// of mass glyph and the engine thrust line, colored green when thrust passes
// close enough to the center of mass to fly straight and red otherwise (gray with
// no engines). Torque is taken about the center of mass, matching Physics.Update.
func (d *Designer) drawBalanceOverlay() {
	com := d.ship.CenterOfMass()
	tl := engineThrustAbout(d.ship.Parts, nil, GridCoord{}, com)
	col := balanceColor(tl)
	if tl.ok {
		half := thrustLineHalfLength(d.ship.Parts, nil, GridCoord{}, tl.point)
		drawThrustLine(d.ship, tl, half, col)
	}
	drawCenterOfMassMarker(d.ship.worldPoint(com.X, com.Y), col)
}

// balanceColor picks the HUD color for a thrust line: gray when there are no
// engines, green when balanced within engineStraightTolerance, red otherwise.
func balanceColor(tl engineThrustLine) rl.Color {
	if !tl.ok {
		return comNeutralColor
	}
	if tl.offset <= engineStraightTolerance {
		return balancedColor
	}
	return unbalancedColor
}

// leftPanel draws the ship-file list and handles selecting/creating designs. In
// shop mode it shows the storefront instead.
func (d *Designer) leftPanel() {
	if d.shop != nil {
		d.shopPanel()
		return
	}
	rl.DrawRectangle(0, 0, dsLeftPanelW, windowHeight, uiPanel)
	rl.DrawText("SHIPS", 20, 20, 28, uiText)

	newBtn := rl.NewRectangle(16, 60, dsLeftPanelW-32, 40)
	if uiButtonRect(newBtn, "+ New Ship", 22, false) {
		d.newShip()
	}

	y := float32(112)
	for _, f := range d.files {
		r := rl.NewRectangle(16, y, dsLeftPanelW-32, 36)
		selected := strings.TrimSuffix(f, ".json") == d.name
		if uiButtonRect(r, strings.TrimSuffix(f, ".json"), 20, selected) {
			d.load(f)
		}
		y += 44
	}
}

// shopPanel draws the storefront in the left panel: the money balance up top, the
// fixed catalog of parts on sale with buy buttons, and the current inventory below.
func (d *Designer) shopPanel() {
	rl.DrawRectangle(0, 0, dsLeftPanelW, windowHeight, uiPanel)
	rl.DrawText("SHOP", 20, 20, 28, uiText)
	rl.DrawText(fmt.Sprintf("Money: $%d", *d.shop.money), 20, 56, 24, uiAccent)

	y := float32(100)
	for i := range d.shop.offers {
		o := &d.shop.offers[i]
		r := rl.NewRectangle(16, y, dsLeftPanelW-32, 48)
		label := fmt.Sprintf("%s  -  $%d", o.Type.String(), o.Price)
		if uiButtonRect(r, label, 20, false) {
			if d.shop.buy(i) {
				d.setStatus("Bought "+o.Type.String(), rl.Lime)
			} else {
				d.setStatus("Not enough money for "+o.Type.String(), rl.Red)
			}
		}
		rl.DrawRectangle(24, int32(y+16), 16, 16, partSpecs[o.Type].color)
		y += 56
	}

	y += 12
	rl.DrawText("INVENTORY", 20, int32(y), 24, uiText)
	y += 34
	empty := true
	for _, t := range palettePartTypes {
		if t == PartCockpit || d.shop.inventory[t] <= 0 {
			continue
		}
		empty = false
		rl.DrawRectangle(24, int32(y+3), 14, 14, partSpecs[t].color)
		rl.DrawText(fmt.Sprintf("%s  x%d", t.String(), d.shop.inventory[t]), 46, int32(y), 20, uiText)
		y += 28
	}
	if empty {
		rl.DrawText("(empty - buy some parts)", 24, int32(y), 18, uiTextDim)
	}
}

func (d *Designer) rightPanel(exit *bool) {
	rl.DrawRectangle(dsRightPanel, 0, dsRightW, windowHeight, uiPanel)
	x := float32(dsRightPanel + 16)
	w := float32(dsRightW - 32)

	rl.DrawText("PARTS", int32(x), 20, 28, uiText)
	y := float32(60)
	for _, t := range palettePartTypes {
		r := rl.NewRectangle(x, y, w, 40)
		label := t.String()
		if d.shop != nil {
			// In the shop the palette is bounded by what the player owns; the count
			// rides on the button and the cockpit (which can't be placed) is skipped.
			if t == PartCockpit {
				continue
			}
			label = fmt.Sprintf("%s  x%d", t.String(), d.shop.inventory[t])
		}
		if uiButtonRect(r, label, 20, d.selType == t) {
			d.selType = t
		}
		// Part color swatch on the left edge of the button.
		rl.DrawRectangle(int32(x+8), int32(y+12), 16, 16, partSpecs[t].color)
		y += 48
	}

	y += 12
	rl.DrawText("FACING", int32(x), int32(y), 24, uiText)
	y += 32
	rotBtn := rl.NewRectangle(x, y, w, 40)
	if uiButtonRect(rotBtn, "Facing: "+d.selFacing.String()+"  (R)", 20, false) {
		d.selFacing = (d.selFacing + 1) % 4
	}
	y += 60

	// Live validation feedback.
	if err := d.ship.Validate(); err != nil {
		drawWrappedText("Invalid: "+err.Error(), int32(x), int32(y), int(w), 18, rl.NewColor(255, 120, 120, 255))
	} else {
		rl.DrawText("Valid design", int32(x), int32(y), 20, rl.Lime)
	}
	y += 44

	// Balance readout: the same center-of-mass + thrust-line concept as the grid
	// overlay, with a plain-language status.
	tl := engineThrustAbout(d.ship.Parts, nil, GridCoord{}, d.ship.CenterOfMass())
	col := balanceColor(tl)
	rl.DrawText("BALANCE", int32(x), int32(y), 24, uiText)
	y += 34
	drawCenterOfMassMarker(rl.NewVector2(x+11, y+9), col)
	rl.DrawText("Center of mass + thrust", int32(x+30), int32(y), 18, uiText)
	y += 30
	status := "No engines"
	if tl.ok {
		if tl.offset <= engineStraightTolerance {
			status = "Balanced - flies straight"
		} else {
			status = "Off-axis - thrust spins it"
		}
	}
	rl.DrawText(status, int32(x), int32(y), 20, col)

	// Actions pinned to the bottom.
	backLabel := "Back to Menu (Esc)"
	if d.shop != nil {
		backLabel = "Leave Shop (Esc)"
	}
	backBtn := rl.NewRectangle(x, windowHeight-70, w, 40)
	if uiButtonRect(backBtn, backLabel, 20, false) {
		*exit = true
	}
	// The shop edits the live ship in place; there's nothing to save to a file.
	// Its primary action is Embark, which launches the next round.
	if d.shop == nil {
		saveBtn := rl.NewRectangle(x, windowHeight-120, w, 40)
		if uiButtonRect(saveBtn, "Save", 22, false) {
			d.save()
		}
	} else {
		embarkBtn := rl.NewRectangle(x, windowHeight-124, w, 44)
		if uiButtonRect(embarkBtn, "EMBARK  ▶  Next Round", 22, true) {
			d.shop.embark = true
			*exit = true
		}
	}
	if d.status != "" {
		drawWrappedText(d.status, int32(x), windowHeight-165, int(w), 16, d.statusColor)
	}
}

func (d *Designer) drawNameField() {
	r := rl.NewRectangle(dsLeftPanelW+40, 12, 460, 40)
	fill := uiButton
	if d.editingName {
		fill = uiButtonHot
	}
	rl.DrawRectangleRec(r, fill)
	if d.editingName {
		rl.DrawRectangleLinesEx(r, 2, uiAccent)
	}
	label := d.name
	if d.editingName {
		label += "_"
	}
	rl.DrawText("Name:", int32(r.X-70), int32(r.Y+10), 20, uiTextDim)
	rl.DrawText(label, int32(r.X+10), int32(r.Y+10), 22, uiText)

	if mouseIn(r) && rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		d.editingName = true
	} else if d.editingName && rl.IsMouseButtonPressed(rl.MouseLeftButton) && !mouseIn(r) {
		d.editingName = false
	}
}

func (d *Designer) drawHint() {
	rl.DrawRectangle(0, windowHeight-dsBottomBar, windowWidth, dsBottomBar, rl.NewColor(12, 14, 18, 255))
	partKeys := "1"
	if n := min(len(palettePartTypes), 9); n > 1 {
		partKeys = "1-" + strconv.Itoa(n)
	}
	hint := "Left-click: place  ·  Right-click: remove  ·  " + partKeys + ": part  ·  R: rotate  ·  Wheel: zoom"
	if d.shop != nil {
		hint = "Left-click: place (uses inventory)  ·  Right-click: remove (to inventory)  ·  R: rotate  ·  Wheel: zoom"
	}
	rl.DrawText(hint, dsLeftPanelW+20, windowHeight-28, 18, uiTextDim)
}

// drawWrappedText renders text wrapped to maxWidth pixels, one greedy line at a
// time. Good enough for short status/validation strings in a fixed-width panel.
func drawWrappedText(text string, x, y int32, maxWidth, fontSize int, color rl.Color) {
	words := strings.Fields(text)
	line := ""
	ly := y
	for _, word := range words {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if rl.MeasureText(try, int32(fontSize)) > int32(maxWidth) && line != "" {
			rl.DrawText(line, x, ly, int32(fontSize), color)
			ly += int32(fontSize) + 4
			line = word
		} else {
			line = try
		}
	}
	if line != "" {
		rl.DrawText(line, x, ly, int32(fontSize), color)
	}
}

// sanitizeShipName strips a filename down to a safe base name: letters, digits,
// dash, and underscore, with spaces folded to underscores.
func sanitizeShipName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".json")
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	return b.String()
}
