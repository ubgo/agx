package agents

import (
	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/pi"
)

// Compat is the vendor version range agentio's module for agent id was
// tested against (spec 13.8 "tested range", edge 8). ok is false for a
// config-defined agent, which has no range. It lives here because the
// ranges are vendor-module functions and this package is the only one
// allowed to import vendor modules.
func Compat(id agentio.ID) (c agentio.Compat, ok bool) {
	switch id {
	case claude.ID:
		return claude.Compat(), true
	case codex.ID:
		return codex.Compat(), true
	case gemini.ID:
		return gemini.Compat(), true
	case pi.ID:
		return pi.Compat(), true
	}
	return agentio.Compat{}, false
}
