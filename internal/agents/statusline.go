package agents

import (
	"encoding/json"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// StatusLineSpec is how agx gives an agent a status line that runs a
// command: the top-level settings key a user's own status line lives
// under (so agx can leave theirs alone), and the settings object that
// sets the command.
type StatusLineSpec struct {
	// Key is the top-level settings key ("statusLine").
	Key string
	// Settings returns the settings object that makes the agent run
	// command for its status line.
	Settings func(command string) (agentio.Raw, error)
}

// statusLineCommand is the type of a status line that runs a command.
const statusLineCommand = "command"

// claudeStatusLineKey is Claude Code's settings key of its status line.
const claudeStatusLineKey = "statusLine"

// StatusLine is agent id's status line, ok false when agx knows of none.
// It lives here because the setting's shape is vendor knowledge (agentio
// has no status-line capability yet), and this package is the only one
// allowed to import vendor modules.
func StatusLine(id agentio.ID) (StatusLineSpec, bool) {
	if id != claude.ID {
		return StatusLineSpec{}, false
	}
	return StatusLineSpec{Key: claudeStatusLineKey, Settings: func(command string) (agentio.Raw, error) {
		line, err := json.Marshal(claude.StatusLine{Type: statusLineCommand, Command: command})
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]json.RawMessage{claudeStatusLineKey: line})
	}}, true
}
