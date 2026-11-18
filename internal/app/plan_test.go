package app

import (
	"strings"
	"testing"
)

// TestPinnedFixesEveryChoice: the replay options carry the plan's
// profile, agent, login and folder, and none of the flags that would ask,
// probe or create again.
func TestPinnedFixesEveryChoice(t *testing.T) {
	o := LaunchOptions{Mode: ModeNew, SessionName: "x", PlanFile: "/tmp/p", Model: "opus", Env: []string{"A=${gopass:a}"}}
	o.AutoProfile, o.AutoLogin = true, true
	p := &LaunchPlan{Mode: ModeNew, Profile: "work", Agent: "claude", Login: "carol", Folder: "/s/x_1"}
	got := pinned(o, p)
	if got.Mode != ModeRun || got.Dir != "/s/x_1" || got.SessionName != "" || got.PlanFile != "" {
		t.Errorf("new must replay as run in the created folder: %+v", got)
	}
	if got.Profile != "work" || got.Agent != "claude" || got.Login != "carol" || got.AutoProfile || got.AutoLogin || got.HomeLogin {
		t.Errorf("choices not pinned: %+v", got.Inputs)
	}
	if got.Model != "opus" || got.Env[0] != "A=${gopass:a}" {
		t.Errorf("options must stay as typed (references unresolved): %+v", got)
	}

	home := pinned(LaunchOptions{Mode: ModeRun}, &LaunchPlan{Mode: ModeRun, Login: loginHomeText, LoginKind: LoginKindHome, Folder: "/w"})
	if !home.HomeLogin || home.Login != "" || home.Dir != "/w" {
		t.Errorf("home login: %+v", home.Inputs)
	}
	ref := LaunchOptions{Mode: ModeRun}
	ref.TokenRef, ref.TokenKind = "${VAR}", "api-key"
	one := pinned(ref, &LaunchPlan{Mode: ModeRun, Login: loginOneOffText, LoginKind: LoginKindTokenRef})
	if one.TokenRef != "${VAR}" || one.TokenKind != "api-key" || one.Login != "" || one.HomeLogin {
		t.Errorf("one-off token: %+v", one.Inputs)
	}
	// A login that happens to be named "home" pins as that login, never
	// as the home login: the kind decides, not the text.
	named := pinned(LaunchOptions{Mode: ModeRun}, &LaunchPlan{Mode: ModeRun, Login: "home", LoginKind: LoginKindNamed})
	if named.Login != "home" || named.HomeLogin {
		t.Errorf("login named home: %+v", named.Inputs)
	}
	res := LaunchOptions{Mode: ModeResume}
	res.Resume.Last = true
	r := pinned(res, &LaunchPlan{Mode: ModeResume, ResumeID: "3f2a", Folder: "/c"})
	if r.Mode != ModeResume || r.ResumeID != "3f2a" || r.Resume.Last {
		t.Errorf("resume must replay the found id, not --last: %+v", r)
	}
}

func TestDriftNamesTheChange(t *testing.T) {
	a := PlanSummary{Agent: "claude", Profile: "work", Home: "/h", Login: "carol", Binary: "/b", Folder: "/f"}
	if d := drift(a, a); d != "" {
		t.Errorf("no change reported %q", d)
	}
	b := a
	b.Mode, b.Command.Line = PlanModeRun, "different"
	if d := drift(a, b); d != "" {
		t.Errorf("mode and command are not compared: %q", d)
	}
	b.Home = "/other"
	if d := drift(a, b); !strings.HasPrefix(d, "home /h is now /other") {
		t.Errorf("drift %q", d)
	}
}
