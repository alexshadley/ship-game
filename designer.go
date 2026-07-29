package main

import (
	"math"
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Designer is the in-app ship editor reached from the pause menu. It reads and
// writes JSON designs in the ships/ folder: the left panel lists existing ships
// to load, the center is a grid you paint parts onto, and the right panel picks
// the part type and facing. It lays out at the logical UI resolution and splits
// each frame into an input pass (Update) and a render pass (Draw). Opened in shop
// mode (shop != nil) it becomes the shop; that mode's passes live in
// shopupdate.go and shopdraw.go.
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

// Update runs one input frame of the designer (or shop): keyboard shortcuts,
// zoom, grid editing, and panel button clicks. It mutates state but draws
// nothing — Draw renders the result afterward off the same layout rects. It
// returns true when the user wants to leave (back to the pause menu, or, in the
// shop, to embark; the shop's embark flag distinguishes the two).
func (d *Designer) Update() bool {
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

	exit := false
	if d.shop != nil {
		d.updateShop(&exit)
	} else {
		d.updateLeftPanel()
		d.updateRightPanel(&exit)
		d.updateNameField()
	}
	return exit
}

// Draw renders the designer (or shop) into the UI texture. It performs no input
// or state mutation; Update handled that first, so the two passes stay cleanly
// separated. It must be called inside a BeginTextureMode block.
func (d *Designer) Draw() {
	rl.ClearBackground(rl.NewColor(16, 18, 24, 255))
	d.drawGrid()
	if d.shop != nil {
		d.drawShop()
	} else {
		d.drawLeftPanel()
		d.drawRightPanel()
		d.drawNameField()
	}
	d.drawHint()
	d.drawPartTooltip()
}

func (d *Designer) drawPartTooltip() {
	if d.editingName || !mouseIn(d.gridRegion()) {
		return
	}
	part, ok := d.ship.Parts[d.hoveredCell()]
	if !ok {
		return
	}
	drawTooltip(partTooltipLines(part), rl.GetMousePosition(), 18, windowWidth, windowHeight)
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

func (d *Designer) gridRegion() rl.Rectangle {
	return rl.NewRectangle(dsLeftPanelW, dsTopBar, dsRightPanel-dsLeftPanelW, windowHeight-dsTopBar-dsBottomBar)
}

func (d *Designer) hoveredCell() GridCoord {
	w := rl.GetScreenToWorld2D(uiMouse(), d.camera)
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

// Panel layout. The designer and shop are immediate-mode UIs split into an input
// pass (Update) and a render pass (Draw); these helpers are the single source of
// each widget's rectangle so both passes agree on where things are. Their
// arithmetic mirrors the running-y layout the panels draw with.

func dsNewShipRect() rl.Rectangle {
	return rl.NewRectangle(16, 60, dsLeftPanelW-32, 40)
}

func dsFileRect(i int) rl.Rectangle {
	return rl.NewRectangle(16, 112+float32(i)*44, dsLeftPanelW-32, 36)
}

// dsRightPanelBounds is the x origin and content width shared by every widget in
// the right panel.
func dsRightPanelBounds() (x, w float32) {
	return float32(dsRightPanel + 16), float32(dsRightW - 32)
}

func dsPartRect(i int) rl.Rectangle {
	x, w := dsRightPanelBounds()
	return rl.NewRectangle(x, 60+float32(i)*48, w, 40)
}

func dsRotRect(tailY float32) rl.Rectangle {
	x, w := dsRightPanelBounds()
	return rl.NewRectangle(x, tailY+32, w, 40)
}

func dsBackRect() rl.Rectangle {
	x, w := dsRightPanelBounds()
	return rl.NewRectangle(x, windowHeight-70, w, 40)
}

func dsSaveRect() rl.Rectangle {
	x, w := dsRightPanelBounds()
	return rl.NewRectangle(x, windowHeight-120, w, 40)
}

func dsEmbarkRect() rl.Rectangle {
	x, w := dsRightPanelBounds()
	return rl.NewRectangle(x, windowHeight-124, w, 44)
}

func dsNameFieldRect() rl.Rectangle {
	return rl.NewRectangle(dsLeftPanelW+40, 12, 460, 40)
}

// designerTailY gives the top of the right panel's shared tail (facing selector,
// validation, balance, action buttons), which sits just below the parts palette.
// The shop's variant is shopTailY, since its list length varies with what's owned.
func designerTailY() float32 { return 60 + float32(len(palettePartTypes))*48 + 12 }

// updateLeftPanel handles the designer left panel's clicks: selecting or creating
// ship files. (The shop replaces this panel with a storefront; see updateShop.)
func (d *Designer) updateLeftPanel() {
	if uiButtonClicked(dsNewShipRect()) {
		d.newShip()
	}
	for i, f := range d.files {
		if uiButtonClicked(dsFileRect(i)) {
			d.load(f)
		}
	}
}

// drawLeftPanel renders the designer's ship-file list. (The shop replaces this
// panel with a storefront; see drawShop.)
func (d *Designer) drawLeftPanel() {
	rl.DrawRectangle(0, 0, dsLeftPanelW, windowHeight, uiPanel)
	rl.DrawText("SHIPS", 20, 20, 28, uiText)
	uiButtonRect(dsNewShipRect(), "+ New Ship", 22, false)
	for i, f := range d.files {
		name := strings.TrimSuffix(f, ".json")
		uiButtonRect(dsFileRect(i), name, 20, name == d.name)
	}
}

// updateRightPanel handles the designer's parts palette clicks, then the shared
// tail.
func (d *Designer) updateRightPanel(exit *bool) {
	for i, t := range palettePartTypes {
		if uiButtonClicked(dsPartRect(i)) {
			d.selType = t
		}
	}
	d.updateRightPanelTail(exit, designerTailY())
}

// drawRightPanel renders the designer's fixed parts palette, then the shared tail.
func (d *Designer) drawRightPanel() {
	x, w := dsRightPanelBounds()
	rl.DrawRectangle(dsRightPanel, 0, dsRightW, windowHeight, uiPanel)
	rl.DrawText("PARTS", int32(x), 20, 28, uiText)
	for i, t := range palettePartTypes {
		r := dsPartRect(i)
		uiButtonRect(r, t.String(), 20, d.selType == t)
		// Part color swatch on the left edge of the button.
		rl.DrawRectangle(int32(x+8), int32(r.Y+12), 16, 16, partSpecs[t].color)
	}
	d.drawRightPanelTail(x, w, designerTailY())
}

// updateRightPanelTail handles the clicks in the shared lower half of the right
// panel (used by both designer and shop): the facing toggle and the bottom action
// buttons. y is the top of the facing section.
func (d *Designer) updateRightPanelTail(exit *bool, y float32) {
	if uiButtonClicked(dsRotRect(y)) {
		d.selFacing = (d.selFacing + 1) % 4
	}
	if uiButtonClicked(dsBackRect()) {
		*exit = true
	}
	// The shop edits the live ship in place; there's nothing to save to a file.
	// Its primary action is Embark, which launches the next round.
	if d.shop == nil {
		if uiButtonClicked(dsSaveRect()) {
			d.save()
		}
	} else if uiButtonClicked(dsEmbarkRect()) {
		// Everything must be fitted to the ship or sold first — you can't set out
		// carrying loose parts in inventory.
		if n := d.shop.inventoryCount(); n > 0 {
			d.setStatus("Fit or sell your inventory before embarking", rl.Red)
		} else {
			d.shop.embark = true
			*exit = true
		}
	}
}

// drawRightPanelTail renders the shared lower half of the right panel: the facing
// selector, live validation, the balance readout, and the bottom action buttons.
// y is the top of the facing section.
func (d *Designer) drawRightPanelTail(x, w, y float32) {
	rl.DrawText("FACING", int32(x), int32(y), 24, uiText)
	uiButtonRect(dsRotRect(y), "Facing: "+d.selFacing.String()+"  (R)", 20, false)

	// Live validation feedback.
	vy := y + 92
	if err := d.ship.Validate(); err != nil {
		drawWrappedText("Invalid: "+err.Error(), int32(x), int32(vy), int(w), 18, rl.NewColor(255, 120, 120, 255))
	} else {
		rl.DrawText("Valid design", int32(x), int32(vy), 20, rl.Lime)
	}

	// Balance readout: the same center-of-mass + thrust-line concept as the grid
	// overlay, with a plain-language status.
	by := vy + 44
	tl := engineThrustAbout(d.ship.Parts, nil, GridCoord{}, d.ship.CenterOfMass())
	col := balanceColor(tl)
	rl.DrawText("BALANCE", int32(x), int32(by), 24, uiText)
	by += 34
	drawCenterOfMassMarker(rl.NewVector2(x+11, by+9), col)
	rl.DrawText("Center of mass + thrust", int32(x+30), int32(by), 18, uiText)
	by += 30
	status := "No engines"
	if tl.ok {
		if tl.offset <= engineStraightTolerance {
			status = "Balanced - flies straight"
		} else {
			status = "Off-axis - thrust spins it"
		}
	}
	rl.DrawText(status, int32(x), int32(by), 20, col)

	// Actions pinned to the bottom.
	backLabel := "Back to Menu (Esc)"
	if d.shop != nil {
		backLabel = "Leave Shop (Esc)"
	}
	uiButtonRect(dsBackRect(), backLabel, 20, false)
	if d.shop == nil {
		uiButtonRect(dsSaveRect(), "Save", 22, false)
	} else {
		uiButtonRect(dsEmbarkRect(), "EMBARK  ▶  Next Round", 22, true)
	}
	if d.status != "" {
		drawWrappedText(d.status, int32(x), windowHeight-165, int(w), 16, d.statusColor)
	}
}

// updateNameField toggles the name text field on click (designer only; the shop
// shows a fixed title instead).
func (d *Designer) updateNameField() {
	r := dsNameFieldRect()
	if mouseIn(r) && rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		d.editingName = true
	} else if d.editingName && rl.IsMouseButtonPressed(rl.MouseLeftButton) && !mouseIn(r) {
		d.editingName = false
	}
}

func (d *Designer) drawNameField() {
	r := dsNameFieldRect()
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
