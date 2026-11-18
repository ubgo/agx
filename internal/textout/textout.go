// Package textout is a writer that remembers its first error, so text
// renderers can print line by line and report one error at the end
// instead of checking (or silently dropping) every fmt.Fprint result.
package textout

import "io"

// Writer wraps an io.Writer; after the first failed write every later
// write is skipped and Err returns that failure.
type Writer struct {
	w   io.Writer
	err error
}

// New wraps w.
func New(w io.Writer) *Writer { return &Writer{w: w} }

// Write implements io.Writer with the sticky error.
func (t *Writer) Write(p []byte) (int, error) {
	if t.err != nil {
		return 0, t.err
	}
	n, err := t.w.Write(p)
	t.err = err
	return n, err
}

// Line writes s and a newline.
func (t *Writer) Line(s string) { t.str(s + "\n") }

// Str writes s.
func (t *Writer) Str(s string) { t.str(s) }

// str writes s unless an earlier write failed.
func (t *Writer) str(s string) {
	if t.err == nil {
		_, t.err = io.WriteString(t.w, s)
	}
}

// Err is the first write error.
func (t *Writer) Err() error { return t.err }
