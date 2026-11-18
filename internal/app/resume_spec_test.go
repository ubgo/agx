package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ubgo/agentio"

	"github.com/ubgo/agx/internal/apperr"
)

// resumeSpecConfig adds a config-defined agent (mycli, with resume_args)
// next to the shared Claude home of resumeSharedConfig. A spec has no
// conversation reader, so mycli can never be searched for a chat.
const resumeSpecConfig = `{
  "version": 1,
  "agents": {
    "mycli": {
      "spec": {
        "name": "My CLI",
        "binary": "mycli",
        "default_home": ".mycli",
        "home_env": "MYCLI_HOME",
        "new_args": ["{args}"],
        "resume_args": ["--resume", "{id}", "{args}"]
      }
    }
  },
  "profiles": {
    "a": { "agent": "claude", "home": "~/.claude-shared", "default": true },
    "my": { "agent": "mycli", "home": "~/.mycli", "default": true }
  }
}`

// resumeSpecOnlyConfig has only the config-defined agent: nothing at all
// can be searched for a conversation.
const resumeSpecOnlyConfig = `{
  "version": 1,
  "agents": {
    "mycli": {
      "spec": { "name": "My CLI", "binary": "mycli", "default_home": ".mycli", "new_args": ["{args}"], "resume_args": ["--resume", "{id}", "{args}"] }
    }
  },
  "profiles": { "my": { "agent": "mycli", "home": "~/.mycli", "default": true } }
}`

// TestResumeConfigAgentRefusedUpFront: resuming on a config-defined agent
// is refused before any search with agentio's own reason (a spec has no
// conversation reader), exit 125 as a launching command, never the old
// "not found in 0 configured home(s)". Unnamed, a config agent next to a
// searchable one is still only named in the not-found hint.
func TestResumeConfigAgentRefusedUpFront(t *testing.T) {
	const wantMsg = "mycli cannot resume conversations: config-only agent"
	for _, c := range []struct {
		name    string
		cfg     string
		agent   agentio.ID
		profile string
		id      string
		last    bool
	}{
		{"--agent", resumeSpecConfig, "mycli", "", "abc123", false},
		{"--profile", resumeSpecConfig, "", "my", "abc123", false},
		{"--last", resumeSpecConfig, "mycli", "", "", true},
		{"only agent", resumeSpecOnlyConfig, "", "", "abc123", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, _ := testApp(t, c.cfg, nil)
			o := LaunchOptions{Mode: ModeResume, ResumeID: c.id, Resume: ResumeOptions{Last: c.last}}
			o.Agent, o.Profile = c.agent, c.profile
			err := a.resumeTarget(context.Background(), &o)
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Code != apperr.CodeUnsupported || !strings.Contains(ae.Message, wantMsg) {
				t.Fatalf("got %v, want an unsupported refusal containing %q", err, wantMsg)
			}
			if got := apperr.Exit(err, true); got != apperr.ExitLaunchFailed {
				t.Fatalf("exit %d, want %d", got, apperr.ExitLaunchFailed)
			}
		})
	}
	a, _ := testApp(t, resumeSpecConfig, nil)
	o := LaunchOptions{Mode: ModeResume, ResumeID: "nosuch"}
	err := a.resumeTarget(context.Background(), &o)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodeNotFound || !strings.Contains(ae.Hint, "not searched (no conversations): mycli (config-only agent") {
		t.Fatalf("unnamed config agent: %v", err)
	}
}

// TestResumeLimitZeroIsNoCap: --limit 0 means no cap, as in conversation
// list (both document "0 = all"); it used to turn into the default 50 in
// checkResumeFlags and into "keep none" in resumeList. A negative limit is
// still a usage error.
func TestResumeLimitZeroIsNoCap(t *testing.T) {
	o := LaunchOptions{Mode: ModeResume}
	if err := checkResumeFlags(&o); err != nil || o.Resume.Limit != 0 {
		t.Fatalf("limit 0: %v, kept %d", err, o.Resume.Limit)
	}
	o = LaunchOptions{Mode: ModeResume, Resume: ResumeOptions{Limit: -1}}
	if err := checkResumeFlags(&o); apperr.Exit(err, false) != apperr.ExitUsage {
		t.Fatalf("limit -1: %v", err)
	}
	a, _ := testApp(t, resumeSharedConfig, nil)
	cfg, err := a.Config()
	if err != nil {
		t.Fatal(err)
	}
	homes, _, err := a.resumeHomes(cfg.Config, &LaunchOptions{Mode: ModeResume})
	if err != nil {
		t.Fatal(err)
	}
	all, _, err := a.resumeList(context.Background(), homes, "", 0, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("limit 0 listed %d, want 1: %v", len(all), err)
	}
	if one, _, _ := a.resumeList(context.Background(), homes, "", 1, ""); len(one) != 1 {
		t.Fatalf("limit 1 listed %d", len(one))
	}
}
