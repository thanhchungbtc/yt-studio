package thumbnail

import "testing"

// The tiles are as large as the frame allows. This is the property the layout
// exists to hold: sizing them from the height a headline left behind is what
// produced a filmstrip stranded mid-frame with a wide empty gutter down both
// sides, and nothing in the drawing code says out loud that it must not happen
// again.
//
// "As large as the frame allows" is two constraints, and one of them is always
// tight: either another step would overflow the width, or it would leave the
// headline under its floor.
func TestTilesAreAsLargeAsTheFrameAllows(t *testing.T) {
	t.Parallel()
	cases := []struct{ cells, rows int }{
		{1, 1}, {4, 1}, {4, 2}, {10, 2}, {12, 2}, {14, 2}, {24, 3}, {7, 2},
	}
	for _, tc := range cases {
		g := layOutGrid(tc.cells, tc.rows)
		grow := g.tileSize + 2

		tooWide := g.cols*grow+(g.cols-1)*tileSpacing > frameWidth-2*gridSideMargin
		tooTall := frameHeight-blockHeight(g.rows, grow)-gridBottomMargin-headlineToGridGap < headlineTopMargin+headlineFontMin
		if !tooWide && !tooTall {
			t.Errorf("%d cells in %d rows: tiles are %dpx and could have been larger",
				tc.cells, tc.rows, g.tileSize)
		}

		// And whatever the tiles came out as, the grid fits the frame with room
		// left for a headline.
		bottom := g.rowY[len(g.rowY)-1] + g.tileSize + tileToCaptionGap + captionRowHeight
		if bottom > frameHeight {
			t.Errorf("%d cells in %d rows: grid ends at %d, past the %dpx frame",
				tc.cells, tc.rows, bottom, frameHeight)
		}
		if budget := g.headlineBudget(); budget < headlineFontMin {
			t.Errorf("%d cells in %d rows: headline left %dpx, under its %dpx floor",
				tc.cells, tc.rows, budget, headlineFontMin)
		}
	}
}

// At the counts a thumbnail actually uses, it is the width that binds — which
// is what "edge to edge" means. Only a grid too sparse to fill a row without
// tiles taller than the frame falls back to the height.
func TestUsefulGridsAreWidthBound(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ cells, rows int }{{10, 2}, {12, 2}, {14, 2}, {24, 3}} {
		g := layOutGrid(tc.cells, tc.rows)
		widest := 0
		for _, n := range g.counts {
			widest = max(widest, n*g.tileSize+(n-1)*tileSpacing)
		}
		// Within one tile of the full span: the remainder is integer division.
		if slack := frameWidth - 2*gridSideMargin - widest; slack < 0 || slack >= g.tileSize {
			t.Errorf("%d cells in %d rows: widest row is %dpx of an available %dpx",
				tc.cells, tc.rows, widest, frameWidth-2*gridSideMargin)
		}
	}
}

// Every cell gets a box, and no two share one.
func TestEveryCellIsPlacedOnce(t *testing.T) {
	t.Parallel()
	const cells, rows = 14, 2
	g := layOutGrid(cells, rows)

	seen := make(map[[2]int]bool, cells)
	total := 0
	for _, n := range g.counts {
		total += n
	}
	if total != cells {
		t.Fatalf("rows hold %d cells, want %d", total, cells)
	}
	for i := range cells {
		box := g.tile(i)
		key := [2]int{box.Min.X, box.Min.Y}
		if seen[key] {
			t.Fatalf("cell %d lands on a box already taken", i)
		}
		seen[key] = true
		if box.Min.X < 0 || box.Max.X > frameWidth {
			t.Fatalf("cell %d spills the frame: %v", i, box)
		}
	}
}

// A grid whose rows do not divide evenly centres the short row rather than
// leaving a hole at the end of it.
func TestShortRowIsCentred(t *testing.T) {
	t.Parallel()
	g := layOutGrid(7, 2)
	if g.counts[0] == g.counts[1] {
		t.Fatalf("7 cells split evenly across 2 rows: %v", g.counts)
	}
	full := g.rowX[0]
	short := g.rowX[1]
	if short <= full {
		t.Fatalf("the short row starts at %d, not indented from the full row at %d", short, full)
	}
}
