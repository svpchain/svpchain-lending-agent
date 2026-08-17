package wire

import (
	"testing"

	"github.com/svpchain/svpchain-lending-agent/internal/mcp/tools"

	"github.com/svpchain/svpchain-lending-agent/internal/agentchain"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
)

// ★ This binary's whole surface, pinned by skill.
//
// It used to compare the union of four profiles against the full registry, back
// when core was shared and no single agent served everything. With only the
// lending profile left the useful assertion inverts: state exactly which
// families this binary serves, so adding or dropping one is a deliberate edit
// here rather than a silent change to what the agent card advertises.
//
// The perps families (market data, account, trading, funds, Cosmos broadcast)
// remain available in toolbridge but are deliberately NOT registered — they are
// another binary's surface. Keeping them in the package is what lets the shared
// dispatch and delegated-read tests go on exercising the credential machinery
// against a realistic tool set; the delegated read layer covers only account
// tools, so there is nothing on this agent's own surface to test it with.
func TestLendingProfileServesExactlyItsFamilies(t *testing.T) {
	h := &tools.Handlers{}
	agentSvc := agentchain.New(nil, nil, nil, nil, nil, nil, nil)

	r := toolbridge.NewEmpty()
	LendingProfile.Register(r, h, agentSvc, nil)

	want := map[string]bool{
		toolbridge.SkillLendora:          true,
		toolbridge.SkillEVM:              true, // the landing rail only
		toolbridge.SkillExecutionLendora: true,
		toolbridge.SkillAuth:             true,
		toolbridge.SkillFaucet:           true,
		toolbridge.SkillAgentRegistry:    true,
		toolbridge.SkillDelegation:       true,
		toolbridge.SkillExecution:        true,
	}
	got := r.BySkill()
	for skill := range want {
		if len(got[skill]) == 0 {
			t.Errorf("the lending profile registers nothing under %s", skill)
		}
	}
	for skill := range got {
		if !want[skill] {
			t.Errorf("the lending profile registers %s, which is not part of this binary's surface", skill)
		}
	}
}

// ★ The EVM skill carries the landing rail and nothing else.
//
// This is the assertion behind cmd/svpchain-lending-agent/card.go's
// SkillDescOverrides, which tells callers this agent serves no swap, bridge or
// token-transfer building. If the DeFi tools ever came back under this skill the
// card would be lying, and the override is static text that cannot notice.
func TestLendingProfileServesOnlyTheEVMLandingRail(t *testing.T) {
	h := &tools.Handlers{}
	agentSvc := agentchain.New(nil, nil, nil, nil, nil, nil, nil)

	r := toolbridge.NewEmpty()
	LendingProfile.Register(r, h, agentSvc, nil)

	got := r.BySkill()[toolbridge.SkillEVM]
	want := []string{"broadcast_evm_tx", "evm_tx_status"}
	if len(got) != len(want) {
		t.Fatalf("EVM skill serves %v, expected only the landing rail %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("EVM skill serves %v, expected only the landing rail %v", got, want)
			break
		}
	}
}

// The profile registers the shared delegation stack: auth to mint bearers, the
// agent-chain identity modules, and the execution core.
func TestLendingProfileServesTheDelegationStack(t *testing.T) {
	h := &tools.Handlers{}
	agentSvc := agentchain.New(nil, nil, nil, nil, nil, nil, nil)

	r := toolbridge.NewEmpty()
	LendingProfile.Register(r, h, agentSvc, nil)
	for _, tool := range []string{
		"auth_challenge", "auth_verify",
		"get_agent", "build_register_agent",
		"get_delegation", "build_create_delegation",
		"agent_identity", "agent_self_register", "execute_record_spend", "agent_claim",
	} {
		if _, ok := r.Lookup(tool); !ok {
			t.Errorf("profile %s missing delegation-stack tool %q", LendingProfile.Name, tool)
		}
	}
}
