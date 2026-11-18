package cli

import (
	"strings"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/app"
	"github.com/ubgo/agx/internal/render"
)

// Status colours shared by the tables built from data (render.Column
// Tone): the v0.3 visual language, so every screen reads the same way.
// Blue is a pass, amber worth a look, red a failure, dim skipped or
// informational.

// toneBy maps a cell's whole text, or its first word before ":" or a
// space, to a tone through m; anything else is plain.
func toneBy(m map[string]render.Tone) func(string) render.Tone {
	return func(text string) render.Tone {
		if t, ok := m[text]; ok {
			return t
		}
		word, _, _ := strings.Cut(text, ":")
		word, _, _ = strings.Cut(word, " ")
		return m[word]
	}
}

// checkTones colours login check's RESOLVE and PROBE cells.
var checkTones = toneBy(map[string]render.Tone{
	string(app.CheckOK):      render.ToneGood,
	string(app.CheckFailed):  render.ToneBad,
	string(app.CheckSkipped): render.ToneDim,
})

// compatTones colours agent list's COMPAT: a tested version blue, outside
// the tested range amber, unknown dim.
var compatTones = toneBy(map[string]render.Tone{
	agentio.CompatInRange.String(): render.ToneGood,
	agentio.CompatNewer.String():   render.ToneWarn,
	agentio.CompatOlder.String():   render.ToneWarn,
	agentio.CompatUnknown.String(): render.ToneDim,
})

// livenessTones colours a run's LIVENESS: alive blue, ended dim, unknown
// amber (it cannot be cleaned safely).
var livenessTones = toneBy(map[string]render.Tone{
	agentio.LivenessAlive.String():   render.ToneGood,
	agentio.LivenessEnded.String():   render.ToneDim,
	agentio.LivenessUnknown.String(): render.ToneWarn,
})
