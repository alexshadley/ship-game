package main

// ShopState is the extra state the Designer carries when it's opened as the shop
// (see NewShop). It holds a shared pointer to the player's money and a shared
// inventory of parts they own, plus the fixed list of parts for sale. Because
// money and inventory are shared references, buying and placing parts in the shop
// persist back to the running game.
type ShopState struct {
	money     *int
	inventory map[PartType]int
	offers    []ShopOffer

	// embark is set by the Embark button; it tells the caller to start the next
	// round rather than fall back to the pause menu when the shop closes.
	embark bool
}

// ShopOffer is a single part on sale. The shop stocks a fixed catalog and every
// offer can be bought any number of times, so there's no sold-out state.
type ShopOffer struct {
	Type  PartType
	Price int
}

// shopOffers is the fixed storefront: cheap unlimited blocks, PDCs and armor in
// the middle, and missiles as the priciest option. Nothing else is sold here.
func shopOffers() []ShopOffer {
	return []ShopOffer{
		{Type: PartBlock, Price: 3},
		{Type: PartPDC, Price: 10},
		{Type: PartArmor, Price: 10},
		{Type: PartMissileLauncher, Price: 20},
	}
}

// buy purchases offer i if the player can afford it, subtracting the price and
// adding the part to inventory. Offers never sell out, so the only failure is not
// enough money. Returns false when the purchase can't be made.
func (s *ShopState) buy(i int) bool {
	if i < 0 || i >= len(s.offers) {
		return false
	}
	o := &s.offers[i]
	if *s.money < o.Price {
		return false
	}
	*s.money -= o.Price
	s.inventory[o.Type]++
	return true
}
