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
	// PartRailgun is a heavy hitscan weapon: it fires within a very narrow arc on a
	// slow reload, striking instantly along a straight line (no projectile) for
	// heavy damage. Firing kicks the shooter back a little and shoves whatever it
	// hits a lot more (see FireWeapons and Physics.fireRailgun).
	PartRailgun
	// PartRattlesnakeMissile fires the same destructible, area-blast missile as a
	// PartMissileLauncher, but the round ejects out the mount's right side and
	// floats there under a small thruster before its booster lights and drives it
	// toward the fire target — a curved, flanking shot (see FireWeapons and the
	// drift-missile branch of Projectile.Update).
	PartRattlesnakeMissile

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

	ShieldHealth     float32
	ShieldDownTimer  float32
	ShieldRegenDelay float32
	ShieldImpacts    []shieldImpact
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

func (p *Part) shieldMax() float32 {
	return shieldMaxHealth * p.combatMults().capacity
}

func (p *Part) shieldRegen() float32 {
	return shieldRegenRate * p.combatMults().regen
}

func (p *Part) shieldActive() bool {
	return p.Type == PartShield && p.ShieldDownTimer <= 0 && p.ShieldHealth > 0
}

func (p *Part) damageShield(amount float32) {
	p.ShieldHealth -= amount
	p.ShieldRegenDelay = shieldRegenDelay
	if p.ShieldHealth <= 0 {
		p.ShieldHealth = 0
		p.ShieldDownTimer = shieldDownDuration
	}
}

func (p *Part) addShieldImpact(angleDeg float32) {
	p.ShieldImpacts = append(p.ShieldImpacts, shieldImpact{angle: angleDeg, timer: shieldFlashDuration})
}

func (p *Part) updateShield(dt float32) {
	if p.Type != PartShield {
		return
	}
	if len(p.ShieldImpacts) > 0 {
		kept := p.ShieldImpacts[:0]
		for _, im := range p.ShieldImpacts {
			im.timer -= dt
			if im.timer > 0 {
				kept = append(kept, im)
			}
		}
		p.ShieldImpacts = kept
	}
	if p.ShieldDownTimer > 0 {
		p.ShieldDownTimer -= dt
		if p.ShieldDownTimer <= 0 {
			p.ShieldDownTimer = 0
			p.ShieldHealth = p.shieldMax() * shieldRestoreFrac
			p.ShieldRegenDelay = shieldRegenDelay
		}
		return
	}
	if p.ShieldRegenDelay > 0 {
		p.ShieldRegenDelay -= dt
		return
	}
	if max := p.shieldMax(); p.ShieldHealth < max {
		p.ShieldHealth += p.shieldRegen() * dt
		if p.ShieldHealth > max {
			p.ShieldHealth = max
		}
	}
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
	PartArmor:  {health: 450, weight: 2 * partWeight, color: rl.DarkBlue},
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
}

const (
	shieldMaxHealth     float32 = 50
	shieldRadius                = 2 * cellSize
	shieldRestoreFrac   float32 = 0.25
	shieldDownDuration  float32 = 6
	shieldRegenDelay    float32 = 3
	shieldRegenRate     float32 = 6
	shieldFlashDuration float32 = 0.35
)

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
	if t == PartShield {
		p.ShieldHealth = p.shieldMax()
	}
	return p
}
