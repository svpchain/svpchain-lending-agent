package wire

import (
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/tools"

	"github.com/svpchain/svpchain-lending-agent/internal/agentchain"
	"github.com/svpchain/svpchain-lending-agent/internal/delegated"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
)

// Profile selects which operation families a binary registers.
//
// This binary ships exactly one — LendingProfile — but the indirection stays
// because it is what keeps registration a named, testable composition rather
// than a hardcoded sequence inside BuildProfile. The coverage test reads it to
// prove nothing left in toolbridge is unreachable.
//
// It used to carry BuildEVM / BuildLendora / RunMarkets flags, gating dependency
// construction as well as registration, so an agent handed the shared full
// config would not dial endpoints for families it never served. With only the
// Lendora surface left there is nothing to gate: this binary always builds the
// Lendora deps, and never the swap/bridge DeFi or perps-markets machinery.
type Profile struct {
	Name string

	// Register composes the binary's operation registry. exec is nil on
	// keyless deployments; the execution registrations then advertise
	// informative refusals.
	Register func(r *toolbridge.Registry, h *tools.Handlers, agent *agentchain.Service, exec *delegated.Service)
}

// RegisterDelegationStack adds the families every delegation-capable binary
// serves: self-service auth, the agent-chain identity modules, and the
// domain-agnostic execution core (identity, self-registration, settlement).
//
// Kept separate from the Lendora families it sits beside in LendingProfile
// because the split is real: this is the domain-agnostic half, shared with
// every other SVP-Chain agent, and it is what a keyless deployment still
// advertises.
func RegisterDelegationStack(r *toolbridge.Registry, h *tools.Handlers, agent *agentchain.Service, exec *delegated.Service) {
	r.RegisterAuth(h)
	r.RegisterFaucet(h)
	r.RegisterAgentChain(agent)
	r.RegisterExecutionCore(exec)
}

// LendingProfile serves the Lendora money market plus the EVM landing rail its
// builds settle through, and delegated Lendora execution (supply/redeem/
// withdraw/borrow/repay) under SVP-DT credentials. It builds the Lendora deps
// but not the swap/bridge EVM DeFi surface it does not serve.
var LendingProfile = Profile{
	Name: "lending",
	Register: func(r *toolbridge.Registry, h *tools.Handlers, agent *agentchain.Service, exec *delegated.Service) {
		r.RegisterLendora(h)
		r.RegisterEVMBroadcast(h)
		r.RegisterExecutionLendora(exec)
		RegisterDelegationStack(r, h, agent, exec)
	},
}
