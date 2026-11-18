package app

import (
	"strings"
	"testing"

	"github.com/ubgo/agentio"
)

// TestSettingValue pins recipe 3: JSON values parse, anything else is a
// string.
func TestSettingValue(t *testing.T) {
	for in, want := range map[string]string{
		"30": "30", "true": "true", `["a"]`: `["a"]`, `{"k":1}`: `{"k":1}`, `"30"`: `"30"`,
		"opus": `"opus"`, "~/bin/s.sh": `"~/bin/s.sh"`, "gopass show -o x": `"gopass show -o x"`, "": `""`,
	} {
		if got := string(settingValue(in)); got != want {
			t.Errorf("settingValue(%q) = %s, want %s", in, got, want)
		}
	}
}

// TestSettingPaths covers nesting, overlay, deletion and leaf listing.
func TestSettingPaths(t *testing.T) {
	base := agentio.Raw(`{"statusLine":{"type":"command"},"model":"opus"}`)
	got, err := setPath(base, strings.Split("statusLine.command", pathSep), agentio.Raw(`"x"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"model":"opus","statusLine":{"command":"x","type":"command"}}` {
		t.Errorf("setPath %s", got)
	}
	got, err = setPath(got, []string{"model", "inner"}, agentio.Raw(`1`))
	if err != nil || string(got) != `{"model":{"inner":1},"statusLine":{"command":"x","type":"command"}}` {
		t.Errorf("setPath over a scalar: %s %v", got, err)
	}
	if l := strings.Join(leafPaths(got), ","); l != "model.inner,statusLine.command,statusLine.type" {
		t.Errorf("leafPaths %s", l)
	}
	got, err = deletePath(got, []string{"model", "inner"})
	if err != nil || string(got) != `{"statusLine":{"command":"x","type":"command"}}` {
		t.Errorf("deletePath %s %v", got, err)
	}
	if got, _ := deletePath(got, []string{"nope"}); string(got) != `{"statusLine":{"command":"x","type":"command"}}` {
		t.Errorf("deletePath missing %s", got)
	}
	if !pathsOverlap("permissions", "permissions.defaultMode") || pathsOverlap("env", "envX") {
		t.Error("pathsOverlap")
	}
	if _, err := decodeObject(agentio.Raw(`[1]`)); err == nil {
		t.Error("an array is not an object")
	}
	if !isEmptyRaw(nil) || !isEmptyRaw(agentio.Raw(" {} ")) || isEmptyRaw(agentio.Raw(`{"a":1}`)) {
		t.Error("isEmptyRaw")
	}
}

// TestCompactJSON renders strings bare and other values as JSON.
func TestCompactJSON(t *testing.T) {
	for in, want := range map[string]string{`"high"`: "high", `[ "a", "b" ]`: `["a","b"]`, `5`: "5", `"<x>"`: "<x>"} {
		if got := compactJSON([]byte(in)); got != want {
			t.Errorf("compactJSON(%s) = %s, want %s", in, got, want)
		}
	}
}
