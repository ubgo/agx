package render

import (
	"io"
	"strings"
	"unicode/utf8"

	"github.com/ubgo/agx/internal/textout"
)

// Table layout sizes. Why: a cell such as a doctor message holding a long
// plugin list, or a deep folder path, used to set the width of its whole
// column (text/tabwriter pads every row to the widest cell), so each line
// grew hundreds of characters wide. The layout below sizes the fixed
// columns by their own content, gives one column the rest of the width
// and wraps it, and moves "below" columns to an indented line.
const (
	// DefaultWidth is the line width when stdout is not a terminal (and
	// COLUMNS is unset): readable in a 120-column window or a log.
	DefaultWidth = 120
	// MinWidth is the smallest width honoured; narrower terminals get it.
	MinWidth = 60
	// minWrapWidth is the narrowest a wrapped column may get; past it the
	// line overflows instead of becoming one word per line.
	minWrapWidth = 24
	// colGap separates columns.
	colGap = 2
	// belowIndent indents a "below" column's line under its row.
	belowIndent = 4
)

// TableSpec says how a table is laid out.
type TableSpec struct {
	// Head are the column headers (also the column count).
	Head []string
	// Wrap is the index of the column that takes the remaining width and
	// wraps onto continuation lines; -1 = the last column.
	Wrap int
	// Below are column indexes printed on their own indented line under
	// the row ("fix: …"), skipped when empty or a dash.
	Below []int
	// Width is the line width; 0 = DefaultWidth.
	Width int
	// Style colours the output: the header bold and each cell by Tones.
	// The zero Style prints plain text.
	Style Style
	// Tones gives column i's cells their tone from their text (a status
	// column: ok blue, fail red); a missing or nil entry is plain, and a
	// dash placeholder is always dim.
	Tones []func(text string) Tone
}

// toneOf is the tone of a cell of column i holding text.
func (spec TableSpec) toneOf(i int, text string) Tone {
	if text == Dash {
		return ToneDim
	}
	if i < len(spec.Tones) && spec.Tones[i] != nil {
		return spec.Tones[i](text)
	}
	return TonePlain
}

// textWidth is the display width of s in runes (agx output is ASCII but
// paths and titles may hold UTF-8).
func textWidth(s string) int { return utf8.RuneCountInString(s) }

// effectiveWidth applies the default and the floor.
func effectiveWidth(w int) int {
	if w <= 0 {
		w = DefaultWidth
	}
	return max(w, MinWidth)
}

// WriteTable prints rows under spec.
func WriteTable(w io.Writer, spec TableSpec, rows [][]string) error {
	width := effectiveWidth(spec.Width)
	below := map[int]bool{}
	for _, b := range spec.Below {
		below[b] = true
	}
	var cols []int // inline columns, in order
	for i := range spec.Head {
		if !below[i] {
			cols = append(cols, i)
		}
	}
	wrap := spec.Wrap
	if wrap < 0 || below[wrap] {
		wrap = cols[len(cols)-1]
	}
	sizes := make([]int, len(spec.Head))
	for _, i := range cols {
		sizes[i] = textWidth(spec.Head[i])
		for _, r := range rows {
			sizes[i] = max(sizes[i], textWidth(r[i]))
		}
	}
	fixed := 0
	for _, i := range cols {
		if i != wrap {
			fixed += sizes[i] + colGap
		}
	}
	sizes[wrap] = min(sizes[wrap], max(width-fixed, minWrapWidth))

	ew := textout.New(w)
	// line pads each cell on its plain text, then paints only the text,
	// so colour never changes the alignment. tone is the cell's tone.
	line := func(cells []string, tone func(i int, text string) Tone) {
		var b strings.Builder
		for n, i := range cols {
			b.WriteString(spec.Style.Paint(tone(i, cells[i]), cells[i]))
			if n == len(cols)-1 {
				break
			}
			b.WriteString(strings.Repeat(" ", sizes[i]-textWidth(cells[i])+colGap))
		}
		ew.Line(strings.TrimRight(b.String(), " "))
	}
	line(spec.Head, func(int, string) Tone { return ToneBold })
	for _, r := range rows {
		parts := Wrap(r[wrap], sizes[wrap])
		cells := append([]string(nil), r...)
		cells[wrap] = parts[0]
		// A wrapped cell keeps the tone of its whole text on every line.
		rowTone := func(i int, text string) Tone {
			if i == wrap {
				return spec.toneOf(i, r[wrap])
			}
			return spec.toneOf(i, text)
		}
		line(cells, rowTone)
		for _, p := range parts[1:] {
			cont := make([]string, len(r))
			cont[wrap] = p
			line(cont, rowTone)
		}
		for _, i := range spec.Below {
			if r[i] != "" && r[i] != Dash {
				ew.Line(strings.Repeat(" ", belowIndent) + spec.Style.Dim(strings.ToLower(spec.Head[i])+":") + " " + r[i])
			}
		}
	}
	return ew.Err()
}

// Wrap splits s into lines of at most width runes, at spaces when it can,
// after a comma inside a longer word, and inside a word only as a last
// resort. It always returns
// at least one line.
func Wrap(s string, width int) []string {
	if width <= 0 || textWidth(s) <= width {
		return []string{s}
	}
	var out []string
	cur := ""
	flush := func() {
		if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	for _, word := range strings.Fields(s) {
		for textWidth(word) > width {
			flush()
			r := []rune(word)
			cut := width
			// A comma-joined list ("installer,usage,…") breaks after a
			// comma rather than inside a name.
			if i := strings.LastIndex(string(r[:width]), ","); i > 0 {
				cut = utf8.RuneCountInString(string(r[:width])[:i+1])
			}
			out = append(out, string(r[:cut]))
			word = string(r[cut:])
		}
		switch {
		case cur == "":
			cur = word
		case textWidth(cur)+1+textWidth(word) <= width:
			cur += " " + word
		default:
			flush()
			cur = word
		}
	}
	flush()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// DecisionLine is one "name  value  from source" line of --dry-run and
// project show.
type DecisionLine struct {
	Name, Value, Source string
}

// valueShare is the most of the line the value column is padded to; a
// longer value (a deep folder, a long command) keeps its full text and its
// "from" moves to the next line instead of pushing every other line's
// "from" far right.
const valueShare = 55

// WriteDecisions prints decision lines: the name column, the value padded
// to the widest value that fits, then "from source". A name repeated on
// consecutive lines is shown once.
//
// The name column is the widest name plus colGap, computed per call: a
// fixed width left an 8-letter name ("settings") one space from its value,
// which reads as one word and which column parsers take as one cell.
//
// With colour (st on) the name is bold and "from source" dim, so the
// decided values stand out, as v0.3's labelled dry run read.
func WriteDecisions(w io.Writer, width int, lines []DecisionLine, st Style) error {
	width = effectiveWidth(width)
	limit := width * valueShare / 100
	pad, labelWidth := 0, 0
	for _, l := range lines {
		if n := textWidth(l.Value); n <= limit {
			pad = max(pad, n)
		}
		labelWidth = max(labelWidth, textWidth(l.Name)+colGap)
	}
	ew := textout.New(w)
	last := ""
	for _, l := range lines {
		name := l.Name
		if name == last {
			name = ""
		} else {
			last = l.Name
		}
		head := st.Bold(name) + strings.Repeat(" ", labelWidth-textWidth(name))
		if l.Source == "" {
			ew.Line(strings.TrimRight(head+l.Value, " "))
			continue
		}
		from := "from " + l.Source
		v := l.Value
		if textWidth(v) <= pad && labelWidth+pad+colGap+textWidth(from) <= width {
			ew.Line(head + v + strings.Repeat(" ", pad-textWidth(v)+colGap) + st.Dim(from))
			continue
		}
		if labelWidth+textWidth(v)+colGap+textWidth(from) <= width {
			ew.Line(head + v + strings.Repeat(" ", colGap) + st.Dim(from))
			continue
		}
		ew.Line(strings.TrimRight(head+v, " "))
		ew.Line(strings.Repeat(" ", labelWidth+belowIndent) + st.Dim(from))
	}
	return ew.Err()
}
