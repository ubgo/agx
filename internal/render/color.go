package render

import "github.com/ubgo/agx/internal/apperr"

// What agx colours (--color, spec 13 global flags). agx's own diagnostics
// on stderr: the error summary line ("agx: …") is red and each warning
// line ("agx: warning: …") is yellow. The human output on stdout uses the
// v0.3 visual language (the owner's reference, docsi/memory
// v03-output-is-reference.md): bold table headers and section titles, dim
// indices and placeholders, blue for comfortably fine, amber close to a
// cap, red for blocked or failed. The JSON envelope is never coloured.
//
// Whether colour is on is decided once per stream by the CLI (--color,
// NO_COLOR, whether that stream is a terminal) and arrives as
// Options.Color (stderr) and Options.OutColor / a Style (stdout), so a
// pipe or a file never sees an escape sequence unless --color always.

// SGR sequences of the palette: the standard 8-colour foregrounds plus
// bold and dim, which every terminal that takes colour at all renders,
// and the reset. Blue (not green) is "fine" because v0.3 mirrored
// claude.ai's usage bars, and the owner asked for that look back.
const (
	sgrError   = "\x1b[31m" // red: the error summary line
	sgrWarning = "\x1b[33m" // yellow: a warning line
	sgrReset   = "\x1b[0m"
	sgrBold    = "\x1b[1m"
	sgrDim     = "\x1b[2m"
	sgrRed     = "\x1b[31m"
	sgrYellow  = "\x1b[33m"
	sgrBlue    = "\x1b[34m"
)

// warningPrefix starts every warning line agx prints itself.
const warningPrefix = "agx: warning: "

// paint wraps line in sgr and the reset when on; off it is line as is.
func paint(line, sgr string, on bool) string {
	if !on || line == "" || sgr == "" {
		return line
	}
	return sgr + line + sgrReset
}

// WarningLine is one warning as agx prints it on stderr ("agx: warning:
// msg"), yellow when color is on. The CLI's launch path and Print share
// it so a warning looks the same wherever it is raised.
func WarningLine(msg string, color bool) string {
	return paint(warningPrefix+apperr.UserText(msg), sgrWarning, color)
}

// Tone is the colour intent of a piece of human output. Callers say what
// a value means (fine, close to a cap, blocked, secondary); render alone
// picks the escape sequence, so no command ever builds one.
type Tone int

// Tones.
const (
	// TonePlain renders the text as is.
	TonePlain Tone = iota
	// ToneGood is comfortably within limits, or a passed check (blue).
	ToneGood
	// ToneWarn is close to a cap, or a check worth a look (amber).
	ToneWarn
	// ToneBad is blocked: a cap reached, a login refused, a failure (red).
	ToneBad
	// ToneDim is secondary text: indices, placeholders such as "—",
	// sources and details.
	ToneDim
	// ToneBold is a header, a title or the row's own name.
	ToneBold
)

// toneCodes maps each tone to its SGR sequence; TonePlain has none.
var toneCodes = map[Tone]string{
	ToneGood: sgrBlue,
	ToneWarn: sgrYellow,
	ToneBad:  sgrRed,
	ToneDim:  sgrDim,
	ToneBold: sgrBold,
}

// Style paints human stdout output. The zero Style is off (plain text),
// which is what every test, pipe and file gets.
type Style struct{ on bool }

// NewStyle is a Style that colours when on.
func NewStyle(on bool) Style { return Style{on: on} }

// On reports whether this Style colours.
func (s Style) On() bool { return s.on }

// Paint wraps text in the tone's sequence when colour is on. Empty text
// stays empty so padding arithmetic never sees a bare escape.
func (s Style) Paint(t Tone, text string) string { return paint(text, toneCodes[t], s.on) }

// Bold is Paint(ToneBold, text).
func (s Style) Bold(text string) string { return s.Paint(ToneBold, text) }

// Dim is Paint(ToneDim, text).
func (s Style) Dim(text string) string { return s.Paint(ToneDim, text) }
