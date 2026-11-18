package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agentio/claude"

	"github.com/ubgo/agx/internal/app"
)

// TestImportTextTrailersKeepColumnsNarrow pins that the skipped/wrote
// lines under the profile import table neither widen its NAME column nor
// leave trailing spaces: written through the tabwriter, a long skip
// reason once stretched NAME to the reason's length.
func TestImportTextTrailersKeepColumnsNarrow(t *testing.T) {
	r := app.ProfileImportResult{
		Profiles: []app.ImportRow{{Name: "work", Agent: claude.ID, Home: "~/.claude-work", FoundBy: []string{"name"}}},
		Skipped:  []app.SkippedHome{{Agent: claude.ID, Home: "~/.claude", Reason: "already a profile, with a reason long enough to widen any column"}},
		Written:  true,
	}
	var b bytes.Buffer
	if err := importText(&b, r); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	// The NAME column is as wide as its widest cell ("work", 4) plus the
	// tabwriter's padding of 2.
	const wantHeader = "NAME  AGENT"
	if !strings.HasPrefix(lines[0], wantHeader) {
		t.Errorf("header %q: want it to start %q", lines[0], wantHeader)
	}
	for _, l := range lines {
		if strings.TrimRight(l, " ") != l {
			t.Errorf("trailing spaces in %q", l)
		}
	}
	if got, want := lines[len(lines)-1], "wrote 1 profile"; got != want {
		t.Errorf("last line %q, want %q", got, want)
	}
}
