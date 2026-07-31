package thumbnail

import "image"

// ------------------------------------------------------------------ grid ----

// grid is the resolved geometry of one thumbnail's tiles.
type grid struct {
	tileSize int
	rows     int
	cols     int
	// counts is how many tiles each row holds. A grid that does not divide
	// evenly puts the remainder in the last row and centres it, rather than
	// leaving a hole at the end of a row.
	counts []int
	rowY   []int
	rowX   []int
}

// layOutGrid sizes the tiles from the frame width. Where the block sits is
// decided later by place, once the headline above it has been fitted.
//
// Width first, deliberately. Sizing the tiles from whatever height a headline
// left behind is what made the first cut of this look like a filmstrip
// stranded mid-frame: the tiles came out small and the frame kept a wide empty
// gutter down both sides. Here the tiles always span the frame, and it is the
// headline that gives way.
func layOutGrid(cells, rows int) grid {
	if rows < 1 {
		rows = 1
	}
	if rows > cells {
		rows = cells
	}
	cols := (cells + rows - 1) / rows

	tile := (frameWidth - 2*gridSideMargin - (cols-1)*tileSpacing) / cols
	// The one case where the tiles give way instead: a grid so tall that the
	// headline would not get its floor.
	for tile > 1 && frameHeight-blockHeight(rows, tile)-gridBottomMargin-headlineToGridGap < headlineTopMargin+headlineFontMin {
		tile -= 2
	}
	if tile < 1 {
		tile = 1
	}

	g := grid{tileSize: tile, rows: rows, cols: cols}
	g.counts = make([]int, rows)
	g.rowX = make([]int, rows)
	g.rowY = make([]int, rows)

	remaining := cells
	for r := range rows {
		n := min(cols, remaining)
		remaining -= n
		g.counts[r] = n
		rowWidth := n*tile + (n-1)*tileSpacing
		g.rowX[r] = (frameWidth - rowWidth) / 2
	}
	g.place(0)
	return g
}

// headlineBudget is how much height is left above the grid for the headline.
func (g grid) headlineBudget() int {
	return frameHeight - gridBottomMargin - blockHeight(g.rows, g.tileSize) - headlineToGridGap - headlineTopMargin
}

// place centres the block in what the headline left, rather than pinning it to
// the bottom of the frame.
//
// Pinned, a grid of small tiles leaves a band of empty background between the
// headline and the first row that reads as a mistake. Centred, the leftover
// space is split above and below and a fourteen-tile grid sits as comfortably
// as a ten-tile one.
func (g *grid) place(headlineBottom int) {
	bandTop := max(headlineBottom+headlineToGridGap, headlineTopMargin)
	bandBottom := frameHeight - gridBottomMargin
	block := blockHeight(g.rows, g.tileSize)

	top := bandTop + max(bandBottom-bandTop-block, 0)/2
	for r := range g.rows {
		g.rowY[r] = top + r*(g.tileSize+tileToCaptionGap+captionRowHeight+tileSpacing)
	}
}

// blockHeight is how tall rows of tiles plus their captions come to.
func blockHeight(rows, tile int) int {
	return rows*(tile+tileToCaptionGap+captionRowHeight) + (rows-1)*tileSpacing
}

// tile returns the box the i-th icon is drawn in.
func (g grid) tile(i int) image.Rectangle {
	r, c := g.rowOf(i)
	x := g.rowX[r] + c*(g.tileSize+tileSpacing)
	y := g.rowY[r]
	return image.Rect(x, y, x+g.tileSize, y+g.tileSize)
}

// captionTop returns the top of the box the i-th caption is drawn in.
func (g grid) captionTop(i int) int {
	r, _ := g.rowOf(i)
	return g.rowY[r] + g.tileSize + tileToCaptionGap
}

func (g grid) rowOf(i int) (row, col int) {
	for r := range g.rows {
		if i < g.counts[r] {
			return r, i
		}
		i -= g.counts[r]
	}
	return g.rows - 1, 0
}
