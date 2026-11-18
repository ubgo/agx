package app

import (
	"context"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/picker"
	"github.com/ubgo/agx/internal/resolve"
)

// rememberLabel is the picker row that also writes default_agent (6.4).
const rememberSuffix = "   (and remember as default_agent)"

// choosers wires the resolver's interactive and network choices: the
// agent picker when a terminal exists, and the auto-pick policies.
func (a *App) choosers(ctx context.Context) resolve.Choosers {
	c := resolve.Choosers{
		AutoProfile: func(ag agentio.Agent, names []string) (string, string, error) {
			return a.autoProfile(ctx, ag, names)
		},
		AutoLogin: func(ag agentio.Agent, names []string) (string, string, error) {
			return a.autoLogin(ctx, ag, names)
		},
	}
	if a.d.Picker != nil {
		c.Agent = func(cands []resolve.AgentChoice) (agentio.ID, bool, error) {
			// Each agent appears twice: once for this run only, once to
			// remember it; a plain list keeps the line picker usable.
			var items []picker.Item
			for _, ch := range cands {
				items = append(items, picker.Item{Label: string(ch.ID), Detail: "default profile: " + ch.DefaultProfile})
			}
			for _, ch := range cands {
				items = append(items, picker.Item{Label: string(ch.ID) + rememberSuffix})
			}
			i, err := a.d.Picker.Choose(ctx, "No agent named. Start which agent?", items)
			if err != nil {
				return "", false, err
			}
			if i >= len(cands) {
				return cands[i-len(cands)].ID, true, nil
			}
			return cands[i].ID, false, nil
		}
	}
	return c
}
