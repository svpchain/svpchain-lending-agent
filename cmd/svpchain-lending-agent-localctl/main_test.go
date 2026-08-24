package main

import "testing"

func TestSkillForTool(t *testing.T) {
	tests := map[string]string{
		"auth_challenge":      authSkill,
		"auth_verify":         authSkill,
		"agent_identity":      executionSkill,
		"agent_self_register": executionSkill,
		"agent_self_update":   executionSkill,
	}
	for tool, want := range tests {
		if got := skillForTool(tool); got != want {
			t.Errorf("skillForTool(%q) = %q, want %q", tool, got, want)
		}
	}
}
