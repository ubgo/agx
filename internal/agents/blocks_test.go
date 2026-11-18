package agents

import (
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
)

// TestBlockSchemas: the agents with launch config (claude, codex) have a
// closed block with their option keys plus settings; the rest have none.
func TestBlockSchemas(t *testing.T) {
	bs := blockSchemas()
	for _, id := range []agentio.ID{"claude", "codex"} {
		s, ok := bs[id]
		if !ok {
			t.Fatalf("%s has no block schema", id)
		}
		if _, ok := s.Properties[SettingsKey]; !ok {
			t.Errorf("%s block has no settings property", id)
		}
		if s.AdditionalProperties == nil {
			t.Errorf("%s block is open; unknown keys would pass", id)
		}
	}
	if _, ok := bs["claude"].Properties["effort"]; !ok {
		t.Error("claude block lacks effort")
	}
	if _, ok := bs["codex"].Properties["codex_profile"]; !ok {
		t.Error("codex block lacks codex_profile")
	}
	for _, id := range []agentio.ID{"gemini", "pi"} {
		if _, ok := bs[id]; ok {
			t.Errorf("%s has a block but no launch config", id)
		}
	}
}

// TestBlockSchemaOrderListsEachKeyOnce: the key order of every block names
// each property once and every property, settings included, even when
// agentio's own schema already lists settings.
func TestBlockSchemaOrderListsEachKeyOnce(t *testing.T) {
	for id, s := range blockSchemas() {
		seen := map[string]bool{}
		for _, k := range s.PropertyOrder {
			if seen[k] {
				t.Errorf("%s: %q twice in %v", id, k, s.PropertyOrder)
			}
			seen[k] = true
		}
		if len(seen) != len(s.Properties) || !seen[SettingsKey] {
			t.Errorf("%s: order %v, properties %d", id, s.PropertyOrder, len(s.Properties))
		}
	}
}

// TestSettingsIsAgxsKeyOnly: agentio's neutral claude options leave
// "settings" to LaunchConfig.Settings, so "settings" in a profile block
// means only agx's settings (a file or an object) and the block schema
// lists it once. If agentio ever offers a raw settings option again, the
// --option keys and the schema would name it twice: this fails first.
func TestSettingsIsAgxsKeyOnly(t *testing.T) {
	set, err := builtin.New(agentio.Host{})
	if err != nil {
		t.Fatal(err)
	}
	ag, ok := set.ByID("claude")
	if !ok {
		t.Fatal("no claude")
	}
	lc, ok := agentio.LaunchConfigOf(ag)
	if !ok {
		t.Fatal("claude has no launch config")
	}
	keys, err := lc.OptionKeys()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentio.FindOptionKey(keys, SettingsKey); err == nil {
		t.Error("agentio's claude option keys offer settings again; agx would show two meanings of settings")
	}
	s, ok := blockSchema(ag)
	if !ok {
		t.Fatal("no claude block schema")
	}
	n := 0
	for _, k := range s.PropertyOrder {
		if k == SettingsKey {
			n++
		}
	}
	if n != 1 {
		t.Errorf("block schema lists settings %d times, want 1", n)
	}
}
