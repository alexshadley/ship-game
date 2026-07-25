package main

import "math/rand"

// ShopState is the extra state the Designer carries when it's opened as the shop
// (see NewShop). It holds a shared pointer to the player's money and a shared
// inventory of parts they own, plus the three offers currently for sale. Because
// money and inventory are shared references, buying and placing parts in the shop
// persist back to the running game.
type ShopState struct {
	money     *int
	inventory map[PartType]int
	offers    []ShopOffer
}

// ShopOffer is a single part on sale. Once bought it's marked sold and can't be
// bought again until the shop is reopened (which rerolls the offers).
type ShopOffer struct {
	Type  PartType
	Price int
	Sold  bool
}

const shopOfferCount = 3

// randomShopOffers rolls n random parts for sale, priced from partSpecs. The
// cockpit is never offered — every ship already has exactly one and it can't be
// removed in the shop. Duplicates are allowed, so the same part can show up twice.
func randomShopOffers(n int) []ShopOffer {
	var pool []PartType
	for _, t := range AllPartTypes() {
		if t == PartCockpit {
			continue
		}
		pool = append(pool, t)
	}
	offers := make([]ShopOffer, 0, n)
	for i := 0; i < n; i++ {
		t := pool[rand.Intn(len(pool))]
		offers = append(offers, ShopOffer{Type: t, Price: partSpecs[t].price})
	}
	return offers
}

// buy purchases offer i if the player can afford it and it isn't already sold,
// subtracting the price and adding the part to inventory. Returns false when the
// purchase can't be made.
func (s *ShopState) buy(i int) bool {
	if i < 0 || i >= len(s.offers) {
		return false
	}
	o := &s.offers[i]
	if o.Sold || *s.money < o.Price {
		return false
	}
	*s.money -= o.Price
	s.inventory[o.Type]++
	o.Sold = true
	return true
}
