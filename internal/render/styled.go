package render

import (
	"io"
	"strings"

	"github.com/ubgo/agx/internal/textout"
)

// StyledTable is the table of agx v0.3 (internal/render/styled.go there),
// ported as is because the owner reads it at a glance: a bold header row,
// columns two spaces apart, each column as wide as its widest cell, every
// cell coloured by its tone, no wrapping. The usage table uses it.

// StyledCell is one cell of a StyledTable.
type StyledCell struct {
	Text string
	Tone Tone
}

// StyledTable writes rows under a bold header with columns aligned by
// their visible width (runes), padding BEFORE colouring so escape
// sequences never skew the alignment the way they would inside
// text/tabwriter. With colour off it is byte for byte the plain aligned
// table. A row shorter than the header is padded with empty cells.
func StyledTable(w io.Writer, header []string, rows [][]StyledCell, st Style) error {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = textWidth(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) {
				widths[i] = max(widths[i], textWidth(c.Text))
			}
		}
	}
	ew := textout.New(w)
	hdr := make([]StyledCell, len(header))
	for i, h := range header {
		hdr[i] = StyledCell{Text: h, Tone: ToneBold}
	}
	ew.Line(styledLine(hdr, widths, st))
	for _, r := range rows {
		ew.Line(styledLine(r, widths, st))
	}
	return ew.Err()
}

// styledLine is one table line: each cell padded to its column's width
// (the last one not at all), then coloured over its text only, so a
// coloured cell occupies exactly the width of a plain one.
func styledLine(cells []StyledCell, widths []int, st Style) string {
	parts := make([]string, len(widths))
	for i := range widths {
		var c StyledCell
		if i < len(cells) {
			c = cells[i]
		}
		pad := ""
		if i < len(widths)-1 {
			pad = strings.Repeat(" ", widths[i]-textWidth(c.Text))
		}
		parts[i] = st.Paint(c.Tone, c.Text) + pad
	}
	return strings.TrimRight(strings.Join(parts, strings.Repeat(" ", colGap)), " ")
}
