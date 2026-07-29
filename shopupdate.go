package main

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The shop is the Designer opened in shop mode (d.shop != nil): it edits the live
// player ship, draws parts from a limited inventory rather than an endless
// palette, and buys/sells for money. Its input pass lives here and its render
// pass in shopdraw.go; the shared frame plumbing (Update/Draw dispatch, grid,
// right-panel tail) stays on Designer in designer.go.

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

// updateShop runs the shop's input pass: the storefront (buying), the inventory
// panel (selecting a part to place and selling), and the shared right-panel tail.
func (d *Designer) updateShop(exit *bool) {
	d.updateStore()
	d.updateInventory(exit)
}

// updateStore handles buy clicks in the left storefront panel.
func (d *Designer) updateStore() {
	for i := range d.shop.offers {
		if uiButtonClicked(shopOfferRect(i)) {
			o := &d.shop.offers[i]
			if d.shop.buy(i) {
				d.setStatus("Bought "+o.Type.String(), rl.Lime)
			} else {
				d.setStatus("Not enough money for "+o.Type.String(), rl.Red)
			}
		}
	}
}

// updateInventory handles the shop's inventory rows — selecting a part to place
// and selling parts back — then the shared tail.
func (d *Designer) updateInventory(exit *bool) {
	owned := d.ownedTypes()
	for j, t := range owned {
		if uiButtonClicked(dsInventorySelectRect(j)) {
			d.selType = t
		}
		if uiButtonClicked(dsInventorySellRect(j)) {
			if d.shop.sell(t) {
				d.setStatus(fmt.Sprintf("Sold %s for $%d", t.String(), sellPrice(t)), rl.Lime)
			}
		}
	}
	d.updateRightPanelTail(exit, shopTailY(len(owned)))
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

// Shop panel layout. Like the designer's ds*Rect helpers, these are the single
// source of each widget's rectangle so the input and render passes agree.

func shopOfferRect(i int) rl.Rectangle {
	return rl.NewRectangle(16, 100+float32(i)*56, dsLeftPanelW-32, 48)
}

func dsInventorySelectRect(j int) rl.Rectangle {
	x, w := dsRightPanelBounds()
	return rl.NewRectangle(x, 60+float32(j)*48, w-96, 40)
}

func dsInventorySellRect(j int) rl.Rectangle {
	x, w := dsRightPanelBounds()
	return rl.NewRectangle(x+w-88, 60+float32(j)*48, 88, 40)
}

// shopTailY is the shop's version of designerTailY: the top of the shared right-
// panel tail, just below the inventory list, whose length varies with what's owned.
func shopTailY(ownedCount int) float32 {
	y := 60 + float32(ownedCount)*48
	if ownedCount == 0 {
		y += 30 // room for the "(buy parts on the left)" line
	}
	return y + 12
}

// ownedTypes lists the part types the player currently holds in inventory, in
// palette order — the rows shown in the shop's right panel. The cockpit is fixed
// to the ship and never held, so it's excluded.
func (d *Designer) ownedTypes() []PartType {
	var out []PartType
	for _, t := range palettePartTypes {
		if t == PartCockpit || d.shop.inventory[t] <= 0 {
			continue
		}
		out = append(out, t)
	}
	return out
}
