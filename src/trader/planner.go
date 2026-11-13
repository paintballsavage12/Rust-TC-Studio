// Package trader plans conservative Outpost metal-fragment purchases from
// manually verified capacity. It NEVER claims to inspect a Rust inventory.
package trader

// Inputs must be explicitly provided by the player; in-game state is unknown.
type Inputs struct {
	Verified              bool
	FreeMainSlots         int
	FreeBackpackSlots     int
	FreeSpaceInMainStacks int // fragment units, not slots
	Scrap                 int
	FragmentsPerTrade     int
	ScrapPerTrade         int
	FragmentStackSize     int
}

type Plan struct {
	Ready            bool
	MaxBatch         int
	Fragments        int
	ScrapCost        int
	MainCapacity     int
	BackpackCapacity int
	TransferFirst    bool
	Reason           string
}

func Calculate(v Inputs) Plan {
	if !v.Verified {
		return Plan{Reason: "Enter and confirm inventory space and scrap before planning"}
	}
	if v.FreeMainSlots < 0 || v.FreeBackpackSlots < 0 || v.FreeSpaceInMainStacks < 0 || v.Scrap < 0 || v.FragmentsPerTrade <= 0 || v.ScrapPerTrade <= 0 || v.FragmentStackSize <= 0 {
		return Plan{Reason: "Invalid capacity or price"}
	}
	if v.FreeMainSlots > 30 || v.FreeBackpackSlots > 40 || v.FreeSpaceInMainStacks > 30000 {
		return Plan{Reason: "Invalid slot count; recheck inventory"}
	}
	mainCapacity := v.FreeMainSlots*v.FragmentStackSize + v.FreeSpaceInMainStacks
	backpackCapacity := v.FreeBackpackSlots * v.FragmentStackSize
	p := Plan{MainCapacity: mainCapacity, BackpackCapacity: backpackCapacity}
	if mainCapacity < v.FragmentsPerTrade {
		if backpackCapacity > 0 {
			p.TransferFirst = true
			p.Reason = "Move verified fragments into backpack, then recheck inventory"
		} else {
			p.Reason = "Main inventory has insufficient space; stop"
		}
		return p
	}
	if v.Scrap < v.ScrapPerTrade {
		p.Reason = "Not enough scrap for another trade; stop"
		return p
	}
	batch := mainCapacity / v.FragmentsPerTrade
	if affordable := v.Scrap / v.ScrapPerTrade; batch > affordable {
		batch = affordable
	}
	if batch > 10 {
		batch = 10
	}
	if batch < 1 {
		p.Reason = "No safe purchase quantity"
		return p
	}
	p.Ready = true
	p.MaxBatch = batch
	p.Fragments = batch * v.FragmentsPerTrade
	p.ScrapCost = batch * v.ScrapPerTrade
	p.Reason = "Estimate only: reconfirm game state and actual vending price before purchase"
	return p
}
