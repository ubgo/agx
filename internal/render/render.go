// Package render prints a command's result: the JSON envelope (spec
// 14.1) with --field selection, or the human form (text, or a table built
// from the same data so --field data.*.KEY can pick its columns).
//
// Why one printer: every command must give the same envelope shape, the
// same --field behaviour and the same error object; doing it once here
// keeps commands to "produce typed data plus an optional text renderer".
package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/datefmt"
	"github.com/ubgo/agx/internal/field"
	"github.com/ubgo/agx/internal/report"
	"github.com/ubgo/agx/internal/textout"
)

// SchemaVersion is the envelope version; bumped only on a breaking change
// to the envelope or any command's data (spec 14.1).
const SchemaVersion = 1

// Decision and Warning are the report types (internal/report), aliased
// so commands can name them through the renderer.
type (
	Decision = report.Decision
	Warning  = report.Warning
)

// ErrorJSON is the envelope's error object.
type ErrorJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Envelope is the one JSON shape every command prints with --json.
type Envelope struct {
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	OK            bool            `json:"ok"`
	Data          json.RawMessage `json:"data"`
	Decisions     []Decision      `json:"decisions"`
	Warnings      []Warning       `json:"warnings"`
	Error         *ErrorJSON      `json:"error"`
}

// null is the JSON for an absent data value.
var null = json.RawMessage("null")

// Column is one human table column: its header and the key of each data
// row it shows.
type Column struct {
	Header string
	Key    string
	// Wrap makes this the column that takes the remaining line width and
	// wraps (default: the last inline column).
	Wrap bool
	// Below prints the cell on its own indented line under the row.
	Below bool
	// Tone colours each cell from its text (a status column); nil is
	// plain. Only on a terminal (Options.OutColor).
	Tone func(text string) Tone
}

// Output is what a command produced, before printing.
type Output struct {
	// Command is the command path ("profile list").
	Command string
	// Data is the command's typed result, already encoded; nil = null.
	Data json.RawMessage
	// Decisions are resolved values with sources (launches, project show).
	Decisions []Decision
	// Warnings are non-fatal notes.
	Warnings []Warning
	// Text renders the human form; nil = Table or the JSON data.
	Text func(w io.Writer) error
	// Table are the default columns when Data is a list of objects; with
	// --field data.*.KEY the user's keys replace them.
	Table []Column
	// Empty is printed instead of a header-only Table when the list has
	// no rows ("no logins — add one with …"), as agx v0.3 did; "" keeps
	// the bare header.
	Empty string
	// KeepData keeps Data in the envelope (and prints the human Text or
	// Table) even when the command fails: doctor reports every check and
	// still exits 1 when one failed. A partial error (apperr.ClassPartial,
	// exit 3) always keeps its data.
	KeepData bool
}

// keeps reports whether o's data is shown despite err.
func (o Output) keeps(err error) bool {
	return err == nil || o.KeepData || apperr.As(err).Class == apperr.ClassPartial
}

// Encode marshals a typed value into Output.Data.
func Encode[T any](v T) (json.RawMessage, error) { return json.Marshal(v) }

// Options are the global output flags.
type Options struct {
	JSON   bool
	Fields []string
	Quiet  bool
	// Width is the human line width (the terminal's, COLUMNS, or
	// DefaultWidth); 0 = DefaultWidth.
	Width int
	// Color colours agx's own stderr diagnostics (color.go). It is the
	// CLI's resolved decision (--color, NO_COLOR, stderr a terminal),
	// never the raw flag value.
	Color bool
	// OutColor colours the human output on stdout (bold headers, tones):
	// the same decision made for stdout. Never applies to --json.
	OutColor bool
	// Date writes timestamp cells (datefmt; --date-format, config
	// date_format); the zero Format is the iso preset.
	Date datefmt.Format
	// Declared are the command's declared --field paths (help --json).
	// They check a path the data cannot: one past a "*" over an empty
	// list (field.SelectDeclared). Empty = only the data checks paths.
	Declared []string
}

// Envelope builds the envelope of o (success) or of err (failure).
func (o Output) Envelope(err error) Envelope {
	env := Envelope{SchemaVersion: SchemaVersion, Command: o.Command, OK: err == nil, Data: o.Data, Decisions: o.Decisions, Warnings: userWarnings(o.Warnings)}
	if env.Data == nil || !o.keeps(err) {
		env.Data = null
	}
	if env.Decisions == nil {
		env.Decisions = []Decision{}
	}
	if env.Warnings == nil {
		env.Warnings = []Warning{}
	}
	if err != nil {
		e := apperr.As(err)
		env.Error = &ErrorJSON{Code: string(e.Code), Message: e.Message, Hint: e.Hint}
	}
	return env
}

// userWarnings is ws with library prefixes stripped from each message
// (apperr.UserText), so a warning reads the same in text and --json.
func userWarnings(ws []Warning) []Warning {
	if ws == nil {
		return nil
	}
	out := make([]Warning, len(ws))
	for i, w := range ws {
		out[i] = Warning{Code: w.Code, Message: apperr.UserText(w.Message)}
	}
	return out
}

// Print writes o (or err) to stdout/stderr per opts. It returns a
// *apperr.Error for a missing --field path (exit 2, edge 56) only when the
// command succeeded; a failed command keeps its own error and exit code.
func Print(stdout, stderr io.Writer, o Output, err error, opts Options) error {
	env := o.Envelope(err)
	if len(opts.Fields) > 0 {
		if !opts.JSON && o.keeps(err) {
			if cols, ok := fieldColumns(opts.Fields); ok && isList(o.Data) {
				// The keys are checked as --json checks them: an unknown
				// key is the same "no such path" error, never a column of
				// dashes that looks like empty data.
				if _, err := selectFields(env, opts.Fields, opts.Declared); err != nil {
					return fieldPathError(err)
				}
				return printTable(stdout, o.Data, cols, opts)
			}
		}
		return printFields(stdout, stderr, env, opts, err)
	}
	if opts.JSON {
		return writeJSON(stdout, env)
	}
	if !o.keeps(err) {
		printError(stderr, err, opts.Color)
		return nil
	}
	if !opts.Quiet {
		ew := textout.New(stderr)
		for _, w := range env.Warnings {
			ew.Line(WarningLine(w.Message, opts.Color))
		}
		if ew.Err() != nil {
			return ew.Err()
		}
	}
	var berr error
	switch {
	case o.Text != nil:
		berr = o.Text(stdout)
	case len(o.Table) > 0 && o.Empty != "" && isEmptyList(o.Data):
		_, berr = io.WriteString(stdout, o.Empty+"\n")
	case len(o.Table) > 0:
		berr = printTable(stdout, o.Data, o.Table, opts)
	case o.Data != nil:
		berr = writeJSON(stdout, json.RawMessage(o.Data))
	}
	if berr == nil && err != nil {
		printError(stderr, err, opts.Color) // the report above, the summary line last
	}
	return berr
}

// printError writes "agx: message" (red when color) and the hint lines
// to stderr.
func printError(w io.Writer, err error, color bool) {
	e := apperr.As(err)
	ew := textout.New(w)
	ew.Line(paint("agx: "+e.Message, sgrError, color))
	if e.Hint != "" {
		for line := range strings.SplitSeq(e.Hint, "\n") {
			ew.Line("  " + line)
		}
	}
	// stderr is the last place to report to; a failed write there has
	// nowhere else to go, and the exit code still says what happened.
}

// PrintError prints a failure in the chosen mode (for errors raised before
// a command produced an Output, such as a flag parse error).
func PrintError(stdout, stderr io.Writer, command string, err error, opts Options) {
	if opts.JSON {
		_ = writeJSON(stdout, Output{Command: command}.Envelope(err)) // a failed stdout write leaves only the exit code, which is set anyway
		return
	}
	printError(stderr, err, opts.Color)
}

// isList reports data that is a JSON array (a table); other data takes
// --field paths through the envelope, where a missing path is exit 2.
func isList(data json.RawMessage) bool {
	t := bytes.TrimSpace(data)
	return len(t) > 0 && t[0] == '['
}

// isEmptyList reports data that is null or an empty JSON array.
func isEmptyList(data json.RawMessage) bool {
	t := strings.Join(strings.Fields(string(data)), "")
	return t == "" || t == "null" || t == "[]"
}

// fieldColumns turns --field data.*.KEY paths into table columns.
func fieldColumns(paths []string) ([]Column, bool) {
	cols := make([]Column, 0, len(paths))
	for _, p := range paths {
		k, ok := field.Column(p)
		if !ok {
			return nil, false
		}
		cols = append(cols, Column{Header: strings.ToUpper(k), Key: k})
	}
	return cols, true
}

// printFields prints the selected envelope paths: one bare, several as an
// object keyed by path.
//
// cmdErr is the command's own failure (nil on success). A failed command
// has null data, so a data path the user meant for the success case is
// missing; reporting that as a usage error (exit 2) would hide the real
// failure and its exit code. So on failure a missing path prints the
// command's error on stderr instead and returns nil, leaving the caller to
// exit with cmdErr's code. Paths that exist (error.message) still print.
func printFields(w, stderr io.Writer, env Envelope, opts Options, cmdErr error) error {
	paths := opts.Fields
	vals, err := selectFields(env, paths, opts.Declared)
	if err != nil {
		var missing *field.MissingError
		if cmdErr != nil && errors.As(err, &missing) {
			printError(stderr, cmdErr, opts.Color)
			return nil
		}
		return fieldPathError(err)
	}
	if len(vals) == 1 {
		_, err := fmt.Fprintln(w, field.Bare(vals[0]))
		return err
	}
	obj, err := field.Many(paths, vals)
	if err != nil {
		return err
	}
	return writeJSON(w, obj)
}

// selectFields selects every path from the envelope, in order, checking
// what the data cannot against declared. A missing path is a
// *field.MissingError (fieldPathError makes it exit 2).
func selectFields(env Envelope, paths, declared []string) ([]json.RawMessage, error) {
	doc, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	vals := make([]json.RawMessage, 0, len(paths))
	for _, p := range paths {
		v, err := field.SelectDeclared(doc, p, declared)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	return vals, nil
}

// fieldPathError makes a missing --field path a usage error (exit 2, edge
// 56); any other error (encoding the envelope) passes through.
func fieldPathError(err error) error {
	var missing *field.MissingError
	if errors.As(err, &missing) {
		return apperr.Wrap(err, apperr.CodeFieldPath, apperr.ClassUsage, err.Error())
	}
	return err
}

// writeJSON writes v indented with a trailing newline.
func writeJSON[T Envelope | json.RawMessage](w io.Writer, v T) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// Dash is shown for an empty cell.
const Dash = "—"

// printTable prints data (a JSON list of objects) as aligned columns.
func printTable(w io.Writer, data json.RawMessage, cols []Column, opts Options) error {
	width, st := opts.Width, NewStyle(opts.OutColor)
	date := opts.Date
	if date.IsZero() {
		date = datefmt.Preset(datefmt.PresetISO)
	}
	var rows []map[string]json.RawMessage
	if len(data) > 0 && string(data) != "null" {
		if err := json.Unmarshal(data, &rows); err != nil {
			return fmt.Errorf("render: table data is not a list of objects: %w", err)
		}
	}
	spec := TableSpec{Wrap: -1, Width: width, Style: st}
	for i, c := range cols {
		spec.Head = append(spec.Head, c.Header)
		spec.Tones = append(spec.Tones, c.Tone)
		if c.Wrap {
			spec.Wrap = i
		}
		if c.Below {
			spec.Below = append(spec.Below, i)
		}
	}
	cells := make([][]string, len(rows))
	for n, r := range rows {
		cells[n] = make([]string, len(cols))
		for i, c := range cols {
			cells[n][i] = Cell(r[c.Key], date)
		}
	}
	return WriteTable(w, spec, cells)
}

// Cell renders one JSON value as table text: a string as itself (an RFC
// 3339 timestamp in the local zone and date's format; --json keeps the
// full value), null or absent as a dash, a list of strings comma-joined,
// anything else compact.
func Cell(v json.RawMessage, date datefmt.Format) string {
	t := bytes.TrimSpace(v)
	if len(t) == 0 || string(t) == "null" || string(t) == `""` {
		return Dash
	}
	var s string
	if json.Unmarshal(t, &s) == nil {
		if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return date.Date(ts.Local())
		}
		return s
	}
	var list []string
	if json.Unmarshal(t, &list) == nil {
		if len(list) == 0 {
			return Dash
		}
		return strings.Join(list, ",")
	}
	return field.Bare(t)
}
