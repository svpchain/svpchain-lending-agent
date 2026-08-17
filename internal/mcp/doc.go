// Package mcp is the root of this repo's copy of svpchain-mcp's lib/mcp: the
// MCP tool handlers the A2A tool bridge dispatches into, plus the chain
// clients, tx builders, EVM contract bindings, indexer client, policy engine
// and auth stores they stand on.
//
// ★ This subtree was absorbed from github.com/svpchain/svpchain-mcp at tag
// v0.1.0 (commit a9ef41f), which used to be a module dependency of this repo.
// It follows svpchain-agent-core, absorbed into internal/ by d3fe198 for the
// same reason: this binary is meant to build from its own source plus tagged
// third-party modules, not from a sibling checkout's moving library.
//
// Unlike agent-core, svpchain-mcp is NOT retired — it still ships
// cmd/mcp-server, and svpchain-research-agent still requires it. So this is a
// fork, and upstream fixes no longer arrive on their own. That is the cost of
// the absorption; the mitigation is that everything here is kept diffable
// against the tag (see "Re-syncing" below).
//
// # What was pruned
//
// This binary serves the Lendora money market plus the EVM landing rail its
// builds settle through, so the Lendora and EVM-broadcast halves of upstream
// are load-bearing here. What went:
//
//   - The swap, bridge and ERC-20/721 tool families — tools/{swap,bridge,erc}.go,
//     builder/bridge.go and the bridge/ package. d3fe198 already removed their
//     registrations; no internal/toolbridge Register* reaches them.
//   - tools/registry.go and tools/authgate.go — Register(srv *mcp.Server, …)
//     and the auth middleware helpers around it. That is the MCP-server
//     registration path; this binary serves A2A, and internal/toolbridge binds
//     these same handlers to A2A operations instead. Handlers and New, all
//     anything here used registry.go for, live in tools/handlers.go.
//
// Two things that look prunable are NOT. builder/uniswap.go and builder/erc.go
// stay: they are per-contract ABI layers, not tool families, and the retained
// get_balance, evm_tx_status and lendora/ market cache read them for token
// labels and ERC-20 decimals(). And the perps tool families stay — this binary
// does not register them (see internal/wire's TestLendingProfileServesExactly
// ItsFamilies) but internal/toolbridge deliberately keeps their Register*
// functions, because the shared dispatch and delegated-read tests exercise the
// credential machinery against them.
//
// # Files that are not verbatim
//
// Everything here is byte-identical to the tag except its import paths, apart
// from tools/handlers.go and tools/evmutil.go (symbols rescued from deleted
// files, marked at the top of each), tools/foreign_evm_mock_test.go (likewise,
// a fixture evm_test.go shares with the pruned bridge tests), tools/deps.go
// (the bridge fields), and the comments — package docs that described a server
// this binary is not, and cross-references that named the old lib/mcp path.
//
// The tree is also gofmt-clean, which upstream's is not (18 files differ there,
// mostly import ordering and struct-tag alignment). That is deliberate: this
// repo's `make fmt` runs gofmt -w over everything, so leaving them unformatted
// would mean the next fmt run silently rewrote a third of the subtree.
//
// # Re-syncing
//
// To compare a file against its upstream original:
//
//	git -C ../svpchain-mcp show v0.1.0:lib/mcp/<pkg>/<file>.go | gofmt | diff - internal/mcp/<pkg>/<file>.go
//
// The only expected hunks are the import block and the exceptions listed above.
// internal/wire mirrors upstream's cmd/mcp-server wiring by hand; drift between
// the two is a bug in whichever copied last.
//
// # One hazard worth naming
//
// auth/recover.go, mcpcodec/codec.go and signer/signer.go each have an init()
// that calls appconfig.SetAddressPrefixes(), setting the svp bech32 prefix
// process-wide. All three are retained, so it fires. A future prune that drops
// every one of them from a binary's import graph would silently change every
// sdk.AccAddress string in that binary.
package mcp
