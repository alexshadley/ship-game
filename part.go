package main

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type PartType int

const (
	PartCockpit PartType = iota
	PartBlock
	// PartArmor is a heavy plated block: triple a normal block's health at twice
	// its weight (see partSpecs).
	PartArmor
	// PartGold is a block-grade hull cell: a normal block's health at twice its
	// weight, but valuable — it sells for $10 in the shop (see partPrice).
	PartGold
	PartEngine
	// PartControlThruster attaches on one or two sides; its Facing sets the thrust
	// axis (it thrusts along the perpendicular) and need not point at an attached side.
	PartControlThruster
	// PartPDC is a point-defense cannon: it slews its aim toward the ship's fire
	// target anywhere within pdcHalfArc of its mount facing and spits short-ranged
	// rounds (see FireWeapons).
	PartPDC
	// PartSlowPDC is a junk PDC: it fires the same round as PartPDC but at a
	// third the cadence (see slowPDCFireInterval). Stock enemies carry one.
	PartSlowPDC
	// PartMissileLauncher is a heavy weapon: it fires rarely and within a narrow
	// arc, launching a slow round that accelerates to a cruise speed, can be shot
	// down in flight (it has health), and detonates for area damage that shoves
	// ships away from the blast (see FireWeapons and Physics.missileBlast).
	PartMissileLauncher
	PartShield
	// PartRailgun is a heavy hitscan weapon: it fires only when the ship's energy is
	// fully charged, dumping ALL of it into one instant strike along a straight line
	// (no projectile). Damage and knockback scale with the energy spent, so a ship
	// with bigger reserves hits harder but waits longer between shots (see
	// FireWeapons and Physics.fireRailgun).
	PartRailgun
	// PartRattlesnakeMissile fires the same destructible, area-blast missile as a
	// PartMissileLauncher, but the round ejects out the mount's right side and
	// floats there under a small thruster before its booster lights and drives it
	// toward the fire target — a curved, flanking shot (see FireWeapons and the
	// drift-missile branch of Projectile.Update).
	PartRattlesnakeMissile
	// PartAutoTurret is an automatic weapon: rather than aiming at a manual fire
	// target it slews a full circle and tracks the nearest enemy on its own,
	// spitting PDC-style rounds while the player's auto-turret toggle is armed (see
	// FireWeapons and Controls.AutoFire). Enemy ships fire theirs at the player.
	PartAutoTurret
	// PartBattery extends a ship's energy reserves: each one raises the maximum
	// energy the ship can store (see Ship.EnergyMax). It has no facing or firing
	// behavior of its own.
	PartBattery
	// PartCharger speeds a ship's energy recharge: each one adds to the rate at
	// which reserves refill over time (see Ship.EnergyRegen).
	PartCharger

	// partTypeCount is the sentinel one past the last real part. Callers that need
	// "every part" (the designer palette, the file-format parser) iterate up to it,
	// so a new part type added above shows up everywhere automatically.
	partTypeCount
)

// AllPartTypes returns every part type in enum order. Anything that lists the full
// set of parts should derive from this rather than hand-maintaining a slice, so a
// newly added part type is picked up automatically.
func AllPartTypes() []PartType {
	types := make([]PartType, 0, partTypeCount)
	for t := PartCockpit; t < partTypeCount; t++ {
		types = append(types, t)
	}
	return types
}

func (t PartType) String() string {
	switch t {
	case PartCockpit:
		return "Cockpit"
	case PartBlock:
		return "Block"
	case PartArmor:
		return "Armor"
	case PartGold:
		return "Gold"
	case PartEngine:
		return "Engine"
	case PartControlThruster:
		return "Control Thruster"
	case PartPDC:
		return "PDC"
	case PartSlowPDC:
		return "Slow PDC"
	case PartMissileLauncher:
		return "Missile Launcher"
	case PartShield:
		return "Shield"
	case PartRailgun:
		return "Railgun"
	case PartRattlesnakeMissile:
		return "Rattlesnake Missile"
	case PartAutoTurret:
		return "Auto-Turret"
	case PartBattery:
		return "Battery"
	case PartCharger:
		return "Charger"
	default:
		return "Unknown"
	}
}

type Facing int

const (
	FacingUp Facing = iota
	FacingRight
	FacingDown
	FacingLeft
)

func (f Facing) angle() float32 {
	switch f {
	case FacingRight:
		return math.Pi / 2
	case FacingDown:
		return math.Pi
	case FacingLeft:
		return 3 * math.Pi / 2
	default:
		return 0
	}
}

func (f Facing) String() string {
	switch f {
	case FacingUp:
		return "Up"
	case FacingRight:
		return "Right"
	case FacingDown:
		return "Down"
	case FacingLeft:
		return "Left"
	default:
		return "Unknown"
	}
}

func (f Facing) offset() GridCoord {
	switch f {
	case FacingUp:
		return GridCoord{0, -1}
	case FacingRight:
		return GridCoord{1, 0}
	case FacingDown:
		return GridCoord{0, 1}
	case FacingLeft:
		return GridCoord{-1, 0}
	default:
		return GridCoord{}
	}
}

type GridCoord struct {
	X, Y int
}

func (c GridCoord) neighbors() [4]GridCoord {
	return [4]GridCoord{
		{c.X, c.Y - 1},
		{c.X + 1, c.Y},
		{c.X, c.Y + 1},
		{c.X - 1, c.Y},
	}
}

const maxPartLevel = 5

func (t PartType) isLeveled() bool {
	return t.isWeapon() || t == PartShield
}

func clampPartLevel(level int) int {
	if level < 1 {
		return 1
	}
	if level > maxPartLevel {
		return maxPartLevel
	}
	return level
}

type statMults struct {
	damage   float32
	fireRate float32
	capacity float32
	regen    float32
}

var levelMults = [maxPartLevel + 1]statMults{
	1: {damage: 1.00, fireRate: 1.00, capacity: 1.00, regen: 1.00},
	2: {damage: 1.30, fireRate: 1.15, capacity: 1.40, regen: 1.25},
	3: {damage: 1.70, fireRate: 1.30, capacity: 1.90, regen: 1.55},
	4: {damage: 2.20, fireRate: 1.50, capacity: 2.50, regen: 1.90},
	5: {damage: 2.90, fireRate: 1.75, capacity: 3.25, regen: 2.35},
}

type PartModifier int

const (
	ModifierTurbo PartModifier = iota
	ModifierSluggish
	ModifierPowerful

	partModifierCount
)

type modifierSpec struct {
	name  string
	desc  string
	mults statMults
}

var modifierSpecs = [partModifierCount]modifierSpec{
	ModifierTurbo:    {"Turbo", "+35% fire rate / regen", statMults{damage: 1, fireRate: 1.35, capacity: 1, regen: 1.35}},
	ModifierSluggish: {"Sluggish", "-30% fire rate / regen", statMults{damage: 1, fireRate: 0.7, capacity: 1, regen: 0.7}},
	ModifierPowerful: {"Powerful", "+30% damage / capacity", statMults{damage: 1.3, fireRate: 1, capacity: 1.3, regen: 1}},
}

func (m PartModifier) valid() bool {
	return m >= 0 && m < partModifierCount
}

func (m PartModifier) String() string {
	if !m.valid() {
		return "Unknown"
	}
	return modifierSpecs[m].name
}

func partModifierFromString(s string) (PartModifier, bool) {
	for m := PartModifier(0); m < partModifierCount; m++ {
		if m.String() == s {
			return m, true
		}
	}
	return 0, false
}

type Part struct {
	Type      PartType
	Facing    Facing
	Health    float32
	Weight    float32
	Level     int
	Modifiers []PartModifier
	// FireCooldown is the per-PDC countdown to its next shot, so each PDC
	// fires on its own cadence regardless of the ship's other PDCs.
	FireCooldown float32
	// RailgunCharge is how long (seconds) a railgun mount has been warming up
	// toward its next shot while its trigger is held; it resets to zero if the
	// trigger lapses. The shot fires once it reaches railgunWarmup. Only used by
	// PartRailgun.
	RailgunCharge float32
	// RailgunAim is the fire heading captured when the warm-up began, stored
	// relative to the ship's own direction so it rotates with the hull. The aim is
	// locked for the whole warm-up: the mount commits to this bearing and ignores
	// any new target until it fires, but a spin of the ship still swings the shot.
	// Only meaningful while RailgunCharge > 0.
	RailgunAim float32
	// RailgunEnergy is the ship's energy reserve captured when the warm-up began. The
	// reserve is bled down linearly across the warm-up and this sets both the drain
	// rate and how far the shot's damage/knockback scale (see FireWeapons). Only
	// meaningful while RailgunCharge > 0.
	RailgunEnergy float32

	// ShieldImpacts are the transient hit flashes on a shield bubble. Shields no
	// longer have their own health pool — they block hits by draining the ship's
	// energy (see Ship.Energy and shieldEfficiency) — so only the visual impacts
	// live on the part.
	ShieldImpacts []shieldImpact
}

type shieldImpact struct {
	angle float32
	timer float32
}

func (p *Part) combatMults() statMults {
	m := levelMults[clampPartLevel(p.Level)]
	for _, mod := range p.Modifiers {
		if !mod.valid() {
			continue
		}
		s := modifierSpecs[mod].mults
		m.damage *= s.damage
		m.fireRate *= s.fireRate
		m.capacity *= s.capacity
		m.regen *= s.regen
	}
	return m
}

func (p *Part) weaponDamage() float32 {
	return p.Type.baseDamage() * p.combatMults().damage
}

func (p *Part) weaponFireInterval() float32 {
	return p.Type.fireInterval() / p.combatMults().fireRate
}

// shieldEfficiency is how many points of incoming damage this shield neutralizes
// per unit of ship energy spent. A base shield is 1:1; leveling it up (capacity
// mult) makes it soak more damage for the same energy. See Ship.shieldBlock.
func (p *Part) shieldEfficiency() float32 {
	return p.combatMults().capacity
}

func (p *Part) addShieldImpact(angleDeg float32) {
	p.ShieldImpacts = append(p.ShieldImpacts, shieldImpact{angle: angleDeg, timer: shieldFlashDuration})
}

// updateShield ages this shield's transient hit flashes. Shield charge itself now
// lives in the ship's energy pool (see Ship.updateEnergy), so nothing else is
// tracked per part.
func (p *Part) updateShield(dt float32) {
	if p.Type != PartShield || len(p.ShieldImpacts) == 0 {
		return
	}
	kept := p.ShieldImpacts[:0]
	for _, im := range p.ShieldImpacts {
		im.timer -= dt
		if im.timer > 0 {
			kept = append(kept, im)
		}
	}
	p.ShieldImpacts = kept
}

type partSpec struct {
	health float32
	weight float32
	color  rl.Color
}

// partWeight is the mass of a standard part. Most parts weigh the same, so a
// ship's center of mass is close to the centroid of its occupied cells; heavy
// parts like armor deviate from that and are handled by the true mass-weighted
// center of mass (see the physics body's shape masses).
const partWeight float32 = 2.0

var partSpecs = map[PartType]partSpec{
	PartCockpit: {health: 75, weight: partWeight, color: rl.SkyBlue},
	PartBlock:   {health: 150, weight: partWeight, color: rl.Gray},
	// Armor is a heavy plate: triple a block's health at twice a normal part's weight.
	PartArmor: {health: 450, weight: 2 * partWeight, color: rl.DarkBlue},
	// Gold is a block-grade cell at twice a normal part's weight; it's valuable cargo.
	PartGold:   {health: 150, weight: 2 * partWeight, color: rl.Gold},
	PartEngine: {health: 150, weight: partWeight, color: rl.Orange},

	PartControlThruster:    {health: 150, weight: partWeight, color: rl.Purple},
	PartPDC:                {health: 75, weight: partWeight, color: rl.DarkGreen},
	PartSlowPDC:            {health: 75, weight: partWeight, color: rl.DarkBrown},
	PartMissileLauncher:    {health: 75, weight: partWeight, color: rl.Maroon},
	PartShield:             {health: 75, weight: partWeight, color: rl.Blue},
	PartRailgun:            {health: 75, weight: partWeight, color: rl.NewColor(210, 225, 240, 255)},
	PartRattlesnakeMissile: {health: 75, weight: partWeight, color: rl.NewColor(150, 100, 55, 255)},
	PartAutoTurret:         {health: 75, weight: partWeight, color: rl.NewColor(80, 200, 160, 255)},
	PartBattery:            {health: 100, weight: partWeight, color: rl.NewColor(60, 200, 120, 255)},
	PartCharger:            {health: 75, weight: partWeight, color: rl.NewColor(240, 220, 90, 255)},
}

const (
	shieldRadius                = 2 * cellSize
	shieldFlashDuration float32 = 0.35
)

// Energy tuning. Every ship stores energy (see Ship.Energy) that recharges over
// time; firing weapons, thrusting, and shields soaking hits all draw it down. The
// cockpit provides the base reserve and recharge rate; batteries add capacity and
// chargers add recharge speed.
const (
	cockpitEnergyReserve float32 = 100
	cockpitEnergyRegen   float32 = 25
	batteryEnergyReserve float32 = 90
	chargerEnergyRegen   float32 = 18
	// energyBrownoutDuration is how long recharge stalls after the reserve is drained
	// to empty — a beat of downtime for bottoming out (see Ship.spendEnergy).
	energyBrownoutDuration float32 = 1.0
)

// energyCost is the energy a single shot from this weapon costs to fire; a mount
// with less than this in the ship's pool holds fire. The railgun is not listed
// here — it spends the ship's entire reserve at once (see FireWeapons).
//
// The gun rounds are priced so a single PDC — the fastest-firing of them — runs a
// mild deficit on the cockpit's recharge alone but is comfortably covered by one
// charger; doubling up on PDCs is what really strains the reserve. The slower
// auto-turret and slow PDC fire the same round, so they draw less per second and
// are cheaper to run. Missiles are a burst weapon and priced per shot.
func (t PartType) energyCost() float32 {
	switch t {
	case PartPDC, PartSlowPDC, PartAutoTurret:
		return 3.0
	case PartMissileLauncher, PartRattlesnakeMissile:
		return 18
	default:
		return 0
	}
}

func NewPart(t PartType, facing Facing) *Part {
	return NewLeveledPart(t, facing, 1)
}

func NewLeveledPart(t PartType, facing Facing, level int) *Part {
	spec := partSpecs[t]
	p := &Part{
		Type:   t,
		Facing: facing,
		Health: spec.health,
		Weight: spec.weight,
		Level:  clampPartLevel(level),
	}
	return p
}
