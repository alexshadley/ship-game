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
}

// ShopOffer is a single part on sale. The shop stocks a fixed catalog and every
// offer can be bought any number of times, so there's no sold-out state.
type ShopOffer struct {
	Type  PartType
	Price int
}

// partPrice is the catalog value of a placeable part in dollars, the single
// source of truth for both buying and selling. The storefront (shopOffers) sells
// a subset of these, but any part held in inventory can be sold back at half this
// value — including parts pulled off the starting ship that aren't for sale.
func partPrice(t PartType) int {
	switch t {
	case PartBlock:
		return 3
	case PartArmor:
		return 10
	case PartGold:
		return 40
	case PartEngine:
		return 8
	case PartControlThruster:
		return 8
	case PartPDC:
		return 10
	case PartSlowPDC:
		return 4
	case PartMissileLauncher:
		return 20
	case PartRattlesnakeMissile:
		return 22
	case PartShield:
		return 15
	case PartRailgun:
		return 25
	case PartAutoTurret:
		return 18
	default:
		return 0
	}
}

// sellPrice is what the shop pays to take a part back: half its catalog value,
// rounded down.
func sellPrice(t PartType) int { return partPrice(t) / 2 }

// shopOffers is the fixed storefront: cheap unlimited blocks, PDCs and armor in
// the middle, and missiles as the priciest option. Nothing else is sold here.
func shopOffers() []ShopOffer {
	types := []PartType{PartBlock, PartPDC, PartArmor, PartShield, PartMissileLauncher, PartRattlesnakeMissile, PartAutoTurret}
	offers := make([]ShopOffer, len(types))
	for i, t := range types {
		offers[i] = ShopOffer{Type: t, Price: partPrice(t)}
	}
	return offers
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

// sell returns one part of type t from inventory to the shop for half its catalog
// value, crediting the player's money. Returns false when they own none.
func (s *ShopState) sell(t PartType) bool {
	if s.inventory[t] <= 0 {
		return false
	}
	s.inventory[t]--
	*s.money += sellPrice(t)
	return true
}

// inventoryCount is the total number of unplaced parts the player is holding.
// Embarking is blocked while it's above zero: everything must be fitted or sold.
func (s *ShopState) inventoryCount() int {
	n := 0
	for _, c := range s.inventory {
		if c > 0 {
			n += c
		}
	}
	return n
}
