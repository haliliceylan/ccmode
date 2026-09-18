package main

import (
	"encoding/json"
	"testing"
)

func TestApplyProfileTouchesOnlyManagedKeys(t *testing.T) {
	settings := map[string]json.RawMessage{
		"env":         json.RawMessage(`{"OLD":"1"}`),
		"statusLine":  json.RawMessage(`{"type":"command"}`),
		"permissions": json.RawMessage(`{"allow":["Bash"]}`),
	}
	profile := map[string]json.RawMessage{
		"env": json.RawMessage(`{"NEW":"2"}`),
	}

	applyProfile(settings, profile)

	if string(settings["env"]) != `{"NEW":"2"}` {
		t.Fatalf("env not replaced: %s", settings["env"])
	}
	if _, ok := settings["statusLine"]; ok {
		t.Fatal("statusLine should be removed when profile omits it")
	}
	if string(settings["permissions"]) != `{"allow":["Bash"]}` {
		t.Fatalf("unmanaged key modified: %s", settings["permissions"])
	}
}
