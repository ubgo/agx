// Package picker asks the user at a terminal: pick one of a list, tick
// several, confirm, or type a hidden value. Every caller gets a Picker
// only when stdin and stderr are terminals; without one the caller must
// refuse with the named form of the command (no guessing, principle 4;
// exit 2, or 125 when launching).
//
// The terminal implementation is deliberately plain (a numbered list on
// stderr, an answer line on stdin): it works in every terminal and in
// tmux, and tests drive it through Line with scripted input.
package picker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/ubgo/agx/internal/apperr"
	"github.com/ubgo/agx/internal/textout"
)

// Item is one row of a list.
type Item struct {
	// Label is the main text ("claude").
	Label string
	// Detail is shown after the label ("default profile: personal").
	Detail string
}

// Picker asks questions. Every method returns an apperr with class
// Dismissed when the user gives an empty answer (or Esc), and Interrupted
// when ctx is cancelled (Ctrl-C).
type Picker interface {
	// Choose returns the index of one item.
	Choose(ctx context.Context, title string, items []Item) (int, error)
	// Checklist returns the indexes of the ticked items (at least one).
	Checklist(ctx context.Context, title string, items []Item) ([]int, error)
	// Confirm asks a yes/no question; def is the answer for an empty line.
	Confirm(ctx context.Context, question string, def bool) (bool, error)
	// Secret reads a value without echo (a pasted token).
	Secret(ctx context.Context, prompt string) (string, error)
}

// Dismissed is the error for an empty answer (exit 1, edge 58).
func Dismissed() error {
	return apperr.New(apperr.CodeDismissed, apperr.ClassDismissed, "nothing chosen")
}

// NoTTY is the refusal for a picker without a terminal; named is the
// command line that avoids it.
func NoTTY(named string) error {
	return apperr.New(apperr.CodeNoTTY, apperr.ClassUsage, "no terminal for the picker; "+named)
}

// Line is a Picker over a reader and a writer (the terminal's stdin and
// stderr). fd is stdin's descriptor for hidden input; -1 = echo.
type Line struct {
	In  *bufio.Reader
	Out io.Writer
	FD  int
}

// NewLine builds a Line picker.
func NewLine(in io.Reader, out io.Writer, fd int) *Line {
	return &Line{In: bufio.NewReader(in), Out: out, FD: fd}
}

// esc is the Escape byte; a line holding it is a dismissal.
const esc = "\x1b"

// read reads one answer line, honouring ctx.
func (l *Line) read(ctx context.Context) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		s, err := l.In.ReadString('\n')
		ch <- res{s, err}
	}()
	select {
	case <-ctx.Done():
		return "", apperr.Wrap(ctx.Err(), apperr.CodeInterrupted, apperr.ClassInterrupted, "interrupted")
	case r := <-ch:
		s := strings.TrimSpace(r.s)
		if r.err != nil && s == "" {
			return "", Dismissed()
		}
		if s == "" || strings.Contains(s, esc) {
			return "", Dismissed()
		}
		return s, nil
	}
}

// list prints a numbered list.
func (l *Line) list(title string, items []Item) {
	ew := textout.New(l.Out)
	ew.Line(title)
	for i, it := range items {
		line := fmt.Sprintf("  %d) %s", i+1, it.Label)
		if it.Detail != "" {
			line += "   " + it.Detail
		}
		ew.Line(line)
	}
}

// Choose prints the list and reads a number.
func (l *Line) Choose(ctx context.Context, title string, items []Item) (int, error) {
	l.list(title, items)
	textout.New(l.Out).Str("choose 1-" + strconv.Itoa(len(items)) + " (empty to cancel): ")
	s, err := l.read(ctx)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > len(items) {
		return 0, apperr.Usage(fmt.Sprintf("%q is not a number from 1 to %d", s, len(items)))
	}
	return n - 1, nil
}

// Checklist prints the list and reads numbers separated by spaces or
// commas ("all" ticks every item).
func (l *Line) Checklist(ctx context.Context, title string, items []Item) ([]int, error) {
	l.list(title, items)
	textout.New(l.Out).Str("tick numbers (e.g. 1 3) or all (empty to cancel): ")
	s, err := l.read(ctx)
	if err != nil {
		return nil, err
	}
	if s == "all" {
		out := make([]int, len(items))
		for i := range out {
			out[i] = i
		}
		return out, nil
	}
	var out []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(items) {
			return nil, apperr.Usage(fmt.Sprintf("%q is not a number from 1 to %d", f, len(items)))
		}
		if !slices.Contains(out, n-1) {
			out = append(out, n-1)
		}
	}
	return out, nil
}

// Confirm reads y or n.
func (l *Line) Confirm(ctx context.Context, question string, def bool) (bool, error) {
	hint := " [y/N] "
	if def {
		hint = " [Y/n] "
	}
	textout.New(l.Out).Str(question + hint)
	s, err := l.read(ctx)
	if apperr.As(err).Code == apperr.CodeDismissed {
		return def, nil
	}
	if err != nil {
		return false, err
	}
	switch strings.ToLower(s) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	return false, apperr.Usage(fmt.Sprintf("%q: answer y or n", s))
}

// Secret reads a value without echo when FD is a terminal.
func (l *Line) Secret(ctx context.Context, prompt string) (string, error) {
	textout.New(l.Out).Str(prompt)
	if l.FD >= 0 && term.IsTerminal(l.FD) {
		b, err := term.ReadPassword(l.FD)
		textout.New(l.Out).Line("")
		if err != nil {
			return "", apperr.Wrap(err, apperr.CodeIO, apperr.ClassFailure, "read: "+err.Error())
		}
		if strings.TrimSpace(string(b)) == "" {
			return "", Dismissed()
		}
		return string(b), nil
	}
	return l.read(ctx)
}
