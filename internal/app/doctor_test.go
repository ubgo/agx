package app

import (
	"errors"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/textdata/expand"
)

// lookupOnly answers HOME_SET as set and every other name as unset.
func lookupOnly(name string) (string, bool) { return "", name == "HOME_SET" }

// TestScanRef: parsing never resolves; unknown schemes, unset variables
// and unbalanced brackets are reported; commands are recorded, not run.
func TestScanRef(t *testing.T) {
	var (
		unset  *expand.UnsetError
		scheme *expand.UnknownSchemeError
	)
	if sc := scanRef("${gopass:a/b}", lookupOnly); sc.err != nil {
		t.Errorf("gopass: %v", sc.err)
	}
	if sc := scanRef("${gopas:a}", lookupOnly); !errors.As(sc.err, &scheme) || scheme.Scheme != "gopas" {
		t.Errorf("unknown scheme: %v", sc.err)
	}
	if sc := scanRef("${NOT_SET_HERE}", lookupOnly); !errors.As(sc.err, &unset) {
		t.Errorf("unset: %v", sc.err)
	}
	if sc := scanRef("${HOME_SET}-${NOT_SET:-dflt}", lookupOnly); sc.err != nil {
		t.Errorf("set and defaulted: %v", sc.err)
	}
	if sc := scanRef("${OPEN", lookupOnly); !errors.Is(sc.err, errUnbalanced) {
		t.Errorf("unbalanced: %v", sc.err)
	}
	sc := scanRef("$(op read x) and $(gopass show -o y)", lookupOnly)
	if sc.err != nil || len(sc.commands) != 2 || sc.commands[0] != "op read x" {
		t.Errorf("commands: %+v", sc)
	}
}

// TestFirstWord: the program of a command line.
func TestFirstWord(t *testing.T) {
	for in, want := range map[string]string{"gopass show -o x": "gopass", `  "/opt/my tool" a`: "/opt/my", "": "", "'op' read": "op"} {
		if got := firstWord(in); got != want {
			t.Errorf("firstWord(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLooksSecret: agentio's secret-name rule, reused.
func TestLooksSecret(t *testing.T) {
	if !looksSecret("MY_SERVICE_TOKEN") || looksSecret("DEBUG") {
		t.Errorf("looksSecret wrong: token %v debug %v", looksSecret("MY_SERVICE_TOKEN"), looksSecret("DEBUG"))
	}
}

// TestCompatText: the wording of edges 8 and 9 and the in-range line.
func TestCompatText(t *testing.T) {
	c := agentio.Compat{Min: agentio.MustVersion("2.1.323"), Max: agentio.MustVersion("2.1.325")}
	cases := []struct {
		v      string
		status agentio.CheckStatus
		msg    string
	}{
		{"2.1.330", agentio.CheckWarn, "claude 2.1.330 is newer than tested (2.1.325)"},
		{"2.1.300", agentio.CheckWarn, "claude 2.1.300 is older than tested (2.1.323)"},
		{"2.1.324", agentio.CheckOK, "claude 2.1.324 is tested (2.1.323..2.1.325)"},
		{"", agentio.CheckWarn, "claude version unknown (no install metadata); tested 2.1.323..2.1.325"},
	}
	for _, tc := range cases {
		var v agentio.Version
		if tc.v != "" {
			v = agentio.MustVersion(tc.v)
		}
		st, msg := compatText("claude", v, c)
		if st != tc.status || msg != tc.msg {
			t.Errorf("%s: %s %q", tc.v, st, msg)
		}
	}
	if dc := versionCheck("mycli", agentio.Version{}, agentio.Compat{}, false); dc.Status != agentio.CheckInfo {
		t.Errorf("custom unknown version: %+v", dc)
	}
}

// TestFailedText: the doctor failure sentence.
func TestFailedText(t *testing.T) {
	if failedText(1) != "1 check failed" || failedText(3) != "3 checks failed" {
		t.Errorf("%q %q", failedText(1), failedText(3))
	}
}
