package thumbnail

import "image"

// The frame and the fixed proportions of the design.
//
// These are constants rather than settings on purpose. A settings table full of
// pixel values is a second place to change a layout and a first place to get it
// wrong; the backend that exists to be tweaked without a rebuild is the
// browser one, and it can hold its layout in CSS where layouts belong.
const (
	// width and height are YouTube's thumbnail dimensions.
	width  = 1280
	height = 720

	backgroundFile = "background.jpg"
	defaultFont    = "CabinSketch-Bold.ttf"
	defaultRows    = 2

	margin = width / 20

	headlineTop = height / 14
	// The headline is set as large as it will go between these, stepping down
	// until it fits. Two lines is the floor: a hook that needs three is a hook
	// that has already lost the glance it was written for.
	headlineMax   = 132
	headlineMin   = 54
	headlineStep  = 4
	headlineLead  = 8  // gap between wrapped lines
	headlineLines = 2  // the most lines a headline may take
	headlineTrack = 22 // tracking as a fraction of the size: size/22

	ruleGap    = 18
	ruleHeight = 9
	// The rule runs under the back half of the headline, as the references do.
	ruleWidth = width / 3

	gridGap    = 34 // headline block to first row
	tileGap    = 18
	tileBorder = 3
	tilePad    = 10

	captionGap = 12
	captionMax = 30
	captionMin = 13
	// captionBand is the height reserved under every tile for its caption,
	// whatever size the grid settles on. Reserving the same band for all of them
	// is what keeps the rows aligned.
	captionBand = captionMax + 4
	// gridBottom is the margin under the last row of captions. It is tighter than
	// the side margin: vertical room is what the headline competes for.
	gridBottom = margin / 2
	// A caption is one line under a tile; below the floor it is cut rather than
	// shrunk further, because a caption nobody can read is not worth the room.
	captionStep = 1
)

// grid is the resolved geometry of one thumbnail's tiles.
type grid struct {
	top      int
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

// layOutGrid fits rows x cols of square tiles plus their captions into what is
// left of the frame under the headline.
func layOutGrid(cells, rows, top int) grid {
	if rows < 1 {
		rows = 1
	}
	if rows > cells {
		rows = cells
	}
	cols := (cells + rows - 1) / rows

	top += gridGap
	available := height - top - gridBottom

	// Tiles are as wide as a row of them can be, then shrunk only if the rows do
	// not fit the height that is left. Sizing by height first is what made the
	// first cut of this look like a filmstrip stranded in the middle of the
	// frame: the reference grid runs nearly edge to edge.
	tile := (width - 2*margin - (cols-1)*tileGap) / cols
	for tile > 1 && rows*(tile+captionGap+captionBand)+(rows-1)*tileGap > available {
		tile -= 2
	}
	if tile < 1 {
		tile = 1
	}

	g := grid{top: top, tileSize: tile, rows: rows, cols: cols}
	g.counts = make([]int, rows)
	g.rowX = make([]int, rows)
	g.rowY = make([]int, rows)

	remaining := cells
	for r := range rows {
		n := min(cols, remaining)
		remaining -= n
		g.counts[r] = n
		rowWidth := n*tile + (n-1)*tileGap
		g.rowX[r] = (width - rowWidth) / 2
		g.rowY[r] = top + r*(tile+captionGap+captionBand+tileGap)
	}

	// Vertically centre the whole block in what the headline left behind, so a
	// short headline does not leave the grid stranded at the top.
	blockHeight := rows*(tile+captionGap+captionBand) + (rows-1)*tileGap
	if slack := (available - blockHeight) / 2; slack > 0 {
		for r := range rows {
			g.rowY[r] += slack
		}
	}
	return g
}

// tile returns the box the i-th icon is drawn in.
func (g grid) tile(i int) image.Rectangle {
	r, c := g.rowOf(i)
	x := g.rowX[r] + c*(g.tileSize+tileGap)
	y := g.rowY[r]
	return image.Rect(x, y, x+g.tileSize, y+g.tileSize)
}

// captionTop returns the top of the box the i-th caption is drawn in.
func (g grid) captionTop(i int) int {
	r, _ := g.rowOf(i)
	return g.rowY[r] + g.tileSize + captionGap
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
