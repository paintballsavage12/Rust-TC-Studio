package trader

import "testing"

func TestPlannerNeverOverfills(t *testing.T) {
	for slots := 0; slots <= 30; slots++ {
		for scrap := 0; scrap <= 1000; scrap += 5 {
			v := Inputs{Verified: true, FreeMainSlots: slots, FreeBackpackSlots: 10, Scrap: scrap, FragmentsPerTrade: 250, ScrapPerTrade: 25, FragmentStackSize: 1000}
			p := Calculate(v)
			if p.MaxBatch > 10 || p.MaxBatch < 0 || p.Fragments > slots*1000 || p.ScrapCost > scrap {
				t.Fatalf("unsafe plan slots=%d scrap=%d: %+v", slots, scrap, p)
			}
		}
	}
}
func TestPlannerNeedsVerifiedInputs(t *testing.T) {
	if Calculate(Inputs{FreeMainSlots: 10, Scrap: 1000, FragmentsPerTrade: 250, ScrapPerTrade: 25, FragmentStackSize: 1000}).Ready {
		t.Fatal("unverified inputs accepted")
	}
}
func TestPlannerPartialStackAndTransfer(t *testing.T) {
	v := Inputs{Verified: true, FreeMainSlots: 0, FreeBackpackSlots: 3, FreeSpaceInMainStacks: 800, Scrap: 2000, FragmentsPerTrade: 250, ScrapPerTrade: 25, FragmentStackSize: 1000}
	p := Calculate(v)
	if !p.Ready || p.MaxBatch != 3 {
		t.Fatalf("partial stacks: %+v", p)
	}
	v.FreeSpaceInMainStacks = 0
	p = Calculate(v)
	if p.Ready || !p.TransferFirst {
		t.Fatalf("transfer first: %+v", p)
	}
	v.FreeBackpackSlots = 0
	p = Calculate(v)
	if p.Ready || p.TransferFirst {
		t.Fatalf("must stop when all full: %+v", p)
	}
}
