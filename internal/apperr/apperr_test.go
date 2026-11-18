package apperr_test

import (
	"errors"
	"testing"

	"github.com/ubgo/agx/internal/apperr"
)

func TestExit(t *testing.T) {
	cases := []struct {
		class     apperr.Class
		launching bool
		want      int
	}{
		{apperr.ClassFailure, false, 1}, {apperr.ClassFailure, true, 125},
		{apperr.ClassUsage, false, 2}, {apperr.ClassUsage, true, 125},
		{apperr.ClassPartial, false, 3},
		{apperr.ClassUnsupported, false, 4}, {apperr.ClassUnsupported, true, 125},
		{apperr.ClassNotFoundBinary, true, 127}, {apperr.ClassNotExecutable, true, 126},
		{apperr.ClassInterrupted, true, 130}, {apperr.ClassInterrupted, false, 130},
		{apperr.ClassDismissed, true, 1}, {apperr.ClassDismissed, false, 1},
	}
	for _, c := range cases {
		if got := apperr.Exit(apperr.New(apperr.CodeIO, c.class, "x"), c.launching); got != c.want {
			t.Errorf("class %d launching %v: %d, want %d", c.class, c.launching, got, c.want)
		}
	}
	if apperr.Exit(nil, true) != 0 || apperr.Exit(errors.New("plain"), false) != 1 {
		t.Fatal("nil / plain")
	}
}

func TestWrapKeepsCause(t *testing.T) {
	cause := errors.New("cause")
	e := apperr.Wrap(cause, apperr.CodeSecret, apperr.ClassFailure, "login x: failed")
	if !errors.Is(e, cause) || e.Error() != "login x: failed" {
		t.Fatal(e)
	}
}
