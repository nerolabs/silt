package bond

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fileBlocks is a plot held in a file, addressed by index at a fixed stride — the
// shape a real plot store writes, and the shape this measurement needs so the plot
// itself is not counted as the process's memory.
type fileBlocks struct {
	f *os.File
	n int
}

func newFileBlocks(t *testing.T, n int) *fileBlocks {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "plot"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return &fileBlocks{f: f, n: n}
}

func (p *fileBlocks) ReadBlock(i int, into []byte) error {
	if i < 0 || i >= p.n {
		return fmt.Errorf("block %d out of range", i)
	}
	_, err := p.f.ReadAt(into, int64(i)*int64(BlockSize))
	return err
}

func (p *fileBlocks) WriteBlock(i int, b []byte) error {
	if i < 0 || i >= p.n {
		return fmt.Errorf("block %d out of range", i)
	}
	_, err := p.f.WriteAt(b, int64(i)*int64(BlockSize))
	return err
}

// The floor box is the smallest machine silt must stay a full participant on: a small
// operator's box, one core, 2 GiB of memory, 10 GiB of disk. Its disk budget splits into
// the bond plot, the chain state and served chunks, and headroom.
//
// Fitting that box is a DESIGN-TIME gate, not an end-of-project hope, and the resource it
// gates is the cost to PRODUCE an artifact — not only to verify, store or transmit it. A
// mechanism whose output is small but whose production blows the floor is disqualified
// however elegant, because an unbounded system on a small box is not inefficient, it is
// unsafe.
const (
	floorBoxMemoryBytes = 2 << 30  // total RAM on the floor box
	floorBoxDiskBytes   = 10 << 30 // total disk on the floor box
	floorBoxPlotBytes   = 5 << 30  // the plot's share of that disk — what buys standing

	// What the plot may use while it is being sealed. The rest of the box's memory is
	// carrying the daemon, the chain state it validates against, and the chunks it
	// serves, all of which are live while a re-plot runs.
	floorBoxSealMemoryBudget = floorBoxMemoryBytes / 2
)

// TestSealFitsTheFloorBoxMemoryBudget measures what Seal costs per plot byte and rejects
// a plot size the floor box could not produce.
//
// Standing is proportional to bonded size, so the largest plot the floor box can seal is
// the ceiling on the influence a small operator can ever hold. If that ceiling is set by
// MEMORY rather than by the disk the operator actually bought, the floor tier is capped
// far below its real contribution and consensus weight concentrates on larger machines —
// a re-centralization arrived at through a resource bound rather than through any policy
// anyone chose.
//
// The cost is measured at sizes safe to run anywhere and extrapolated, rather than
// sealing a floor-sized plot to find out: the measurement itself must fit the box.
func TestSealFitsTheFloorBoxMemoryBudget(t *testing.T) {
	// Residency per plot byte on the path a node with a real plot store takes,
	// measured over a range wide enough that fixed overhead stops dominating.
	var ratio float64
	var perMiB time.Duration
	for _, size := range []int64{4 << 20, 16 << 20, 64 << 20} {
		dst := newFileBlocks(t, NumBlocks(size))
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		c, err := SealInto(pk(7), size, dst)
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		if err != nil {
			t.Fatalf("sealing a %s plot: %v", sizeLabel(size), err)
		}
		runtime.KeepAlive(c)

		ratio = float64(after.HeapAlloc-before.HeapAlloc) / float64(size)
		perMiB = elapsed / time.Duration(size>>20)
		t.Logf("%s plot: %d blocks, %.3fx resident, %v to seal (%v/MiB)",
			sizeLabel(size), NumBlocks(size), ratio, elapsed.Round(time.Millisecond), perMiB.Round(time.Millisecond))
	}
	if ratio <= 0 {
		t.Fatal("GATE VACUOUS: measured no residency at all — the measurement is not observing Seal")
	}

	need := int64(ratio * float64(floorBoxPlotBytes))
	t.Logf("floor box: %d MiB plot needs ~%d MiB resident at %.2fx; the seal budget is %d MiB",
		floorBoxPlotBytes>>20, need>>20, ratio, floorBoxSealMemoryBudget>>20)

	if need > floorBoxSealMemoryBudget {
		maxPlot := int64(float64(floorBoxSealMemoryBudget) / ratio)
		t.Fatalf("THE FLOOR BOX CANNOT SEAL ITS OWN PLOT — producing the %d MiB plot its disk budget allows "+
			"needs ~%d MiB resident (%.3fx the plot size), against a %d MiB seal budget on a %d MiB box. "+
			"The largest bond a small operator can post is therefore ~%d MiB, set by RAM rather than by the "+
			"%d GiB of disk they bought. Standing is proportional to bonded size, so this caps the floor "+
			"tier's consensus weight at roughly a %dth of what its disk could back, and concentrates weight "+
			"on larger machines by resource bound rather than by design. Sealing is not slow — about %v per "+
			"MiB — so the constraint is residency, not work.",
			floorBoxPlotBytes>>20, need>>20, ratio, floorBoxSealMemoryBudget>>20, floorBoxMemoryBytes>>20,
			maxPlot>>20, floorBoxDiskBytes>>30, floorBoxPlotBytes/maxPlot, perMiB.Round(time.Millisecond))
	}
	t.Logf("the floor box can produce the plot its disk budget allows: %d MiB needs ~%d MiB resident",
		floorBoxPlotBytes>>20, need>>20)
}
