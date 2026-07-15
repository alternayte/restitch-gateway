package main

import "testing"

func TestRunCmd_MutualExclusivity(t *testing.T) {
	code := runCmd([]string{"-config", "x.yaml", "-registry-url", "http://example.com"})
	if code != 1 {
		t.Errorf("expected exit 1 for mutual exclusivity, got %d", code)
	}
}
