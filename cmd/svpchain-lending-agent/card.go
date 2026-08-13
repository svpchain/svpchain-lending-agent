package main

import (
	"github.com/svpchain/svpchain-agent-core/a2aserver"
	"github.com/svpchain/svpchain-agent-core/toolbridge"
)

// identity is this agent's public face: the name, version, and description its
// Agent Card advertises.
//
// It lives here rather than in the core library because it is this agent's
// product identity — changing it is this repo's decision, not a change to the
// shared library every agent depends on.
//
// ★ These bytes are load-bearing. The served card is hashed and published on
// chain by agent_self_register, and a verifier fetches the card and recomputes
// that hash. Editing anything here changes the card, so the deployment must run
// agent_self_update afterwards or the agent reads as unverified. The golden
// test beside this file is what makes such a change deliberate.
var identity = a2aserver.CardIdentity{
	Name:    "svpchain-lending-agent",
	Version: "0.1.0",
	Description: "Lending agent for the Lendora money market on SVP-Chain: market and " +
		"account reads, risk assessment, unsigned supply/withdraw/borrow/repay/" +
		"collateral tx building with an EVM landing rail, self-service auth, faucet, " +
		"agent registry, delegations, and SVP-DT settlement and self-registration " +
		"(delegated lending writes are future work; builds are caller-signed).",
	SkillDescOverrides: map[string]string{
		toolbridge.SkillEVM: "EVM transaction landing rail for Lendora builds: broadcast " +
			"a caller-signed raw transaction and track its status. This agent serves " +
			"no swap, bridge, or token-transfer building.",
	},
}
