package main

import (
	"github.com/svpchain/svpchain-lending-agent/internal/a2aserver"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
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
// agent_self_update afterwards or the agent reads as unverified.
var identity = a2aserver.CardIdentity{
	Name:    "svpchain-lending-agent",
	Version: "0.1.0",
	Description: "Lending agent for the Lendora money market on SVP-Chain. It synchronizes " +
		"the token-scoped Lendora catalog from its private DeFi MCP at startup. Every " +
		"write is built for the caller, signed locally, then broadcast through the " +
		"published execution path.",
	SkillDescOverrides: map[string]string{
		toolbridge.SkillLendora: "Private DeFi MCP-backed Lendora (Compound V2 fork) reads, " +
			"risk assessment, and unsigned supply/withdraw/borrow/repay/collateral builds. " +
			"Only tools synchronized at startup are published.",
		toolbridge.SkillEVM: "Caller-signed EVM transaction broadcast and transaction-status lookup. " +
			"It does not build DeFi transactions; builds are provided by the private Lendora MCP catalog.",
	},
}
