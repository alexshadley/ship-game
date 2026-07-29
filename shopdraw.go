package main

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// drawShop renders the shop: the storefront (left), the owned-parts inventory
// (right), and the title. The grid, hint bar, and part tooltip are shared with
// the designer and drawn by Designer.Draw around this. Input lives in
// shopupdate.go; both passes read the same layout rects.
func (d *Designer) drawShop() {
	d.drawStore()
	d.drawInventory()
	rl.DrawText("Outfitting: "+d.name, dsLeftPanelW+40, 22, 24, uiText)
}

// drawStore draws the storefront in the left panel: the money balance up top and
// the fixed catalog of parts on sale with buy buttons. Owned parts and their sell
// buttons live in the right panel (see drawInventory).
func (d *Designer) drawStore() {
	rl.DrawRectangle(0, 0, dsLeftPanelW, windowHeight, uiPanel)
	rl.DrawText("SHOP", 20, 20, 28, uiText)
	rl.DrawText(fmt.Sprintf("Money: $%d", *d.shop.money), 20, 56, 24, uiAccent)
	for i := range d.shop.offers {
		o := &d.shop.offers[i]
		r := shopOfferRect(i)
		uiButtonRect(r, fmt.Sprintf("%s  -  $%d", o.Type.String(), o.Price), 20, false)
		rl.DrawRectangle(24, int32(r.Y+16), 16, 16, partSpecs[o.Type].color)
	}
}

// drawInventory renders the shop's right panel. Unlike the designer's fixed parts
// palette (which always lists every part type), it shows only the part types the
// player currently owns: each row is a selectable palette entry for placing the
// part on the ship, and carries its own Sell button. Buying more parts is done in
// the left storefront panel.
func (d *Designer) drawInventory() {
	x, w := dsRightPanelBounds()
	rl.DrawRectangle(dsRightPanel, 0, dsRightW, windowHeight, uiPanel)
	rl.DrawText("INVENTORY", int32(x), 20, 28, uiText)
	owned := d.ownedTypes()
	for j, t := range owned {
		selR := dsInventorySelectRect(j)
		uiButtonRect(selR, fmt.Sprintf("%s  x%d", t.String(), d.shop.inventory[t]), 20, d.selType == t)
		// Part color swatch on the left edge of the select button.
		rl.DrawRectangle(int32(x+8), int32(selR.Y+12), 16, 16, partSpecs[t].color)
		// Sell one back for half its catalog value, crediting money.
		uiButtonRect(dsInventorySellRect(j), fmt.Sprintf("Sell $%d", sellPrice(t)), 18, false)
	}
	if len(owned) == 0 {
		rl.DrawText("(buy parts on the left)", int32(x), 60, 18, uiTextDim)
	}
	d.drawRightPanelTail(x, w, shopTailY(len(owned)))
}
