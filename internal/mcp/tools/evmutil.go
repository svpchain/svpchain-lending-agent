package tools

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/svpchain/svpchain-lending-agent/internal/mcp/builder"
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/chain"
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/payload"
)

// ★ Rescued from upstream tools/swap.go and tools/erc.go during the absorption
// of svpchain-mcp v0.1.0 (see internal/mcp/doc.go). Those two files carried the
// swap and ERC-20/721 tool families, which this binary does not serve and which
// no internal/toolbridge Register* reaches, so the handlers were pruned. But
// they also held the shared EVM plumbing — the eth_call wrappers, the token
// decimals/balance reads, the deployment's token registry, and the ERC tx
// assembler — that the Lendora, faucet, account and EVM-broadcast families all
// build on. That plumbing is what lives here, moved verbatim.

// maxUint256 is the conventional "infinite" ERC-20 allowance (2^256 - 1).
var maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

// -- token / plan helpers (pure, unit-tested) --------------------------

// knownToken is one registered ERC-20 for this deployment: its address plus
// whether that balance is also represented by an x/bank denom.
//
// bankLinked tokens (e.g. USDC, the EVM side of the erc20/usdc trading
// collateral) already surface through get_balance's bank read, so they are NOT
// additionally contract-read there — that would double-count the same balance.
// Pure ERC-20s (USDV) have no bank denom and ARE contract-read. The distinction
// only affects get_balance; swap aliases and faucet labels use every entry.
type knownToken struct {
	address    common.Address
	bankLinked bool
}

// knownSwapTokens maps lower-case symbol aliases to this deployment's ERC-20s,
// so an agent can pass token_in/token_out="usdv" / "usdc" instead of the raw 0x
// address (native SVP is named separately, in parseSwapToken). These are
// convenience aliases only — a caller can always pass any 0x address, or
// discover faucet-dispensed tokens via list_faucet_tokens. Hardcoded like
// knownDenoms in account.go; decimals are still read on chain at call time. Also
// the source for labeling known ERC-20s by symbol in faucet output (faucet.go)
// and for the contract-read balances in get_balance (account.go).
var knownSwapTokens = map[string]knownToken{
	"usdv": {address: common.HexToAddress("0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951")},
	"usdc": {address: common.HexToAddress("0x732F6Ea7AfD5EdC02e7ba052075dd0780e285489"), bankLinked: true},
}

// knownTokenSymbol reverse-maps an ERC-20 address to its upper-cased symbol
// alias, if one is registered in knownSwapTokens.
func knownTokenSymbol(addr common.Address) (string, bool) {
	for sym, kt := range knownSwapTokens {
		if kt.address == addr {
			return strings.ToUpper(sym), true
		}
	}
	return "", false
}

// parseSwapToken resolves a tool's token argument to either native SVP or an
// ERC-20 address. Empty, "native", "svp", or the zero address all mean native;
// a known symbol (see knownSwapTokens) resolves to its address; anything else
// must be a valid 0x address.
func parseSwapToken(s string) (addr common.Address, native bool, err error) {
	t := strings.TrimSpace(s)
	key := strings.ToLower(t)
	switch key {
	case "", "native", "svp":
		return common.Address{}, true, nil
	}
	if kt, ok := knownSwapTokens[key]; ok {
		return kt.address, false, nil
	}
	if !common.IsHexAddress(t) {
		return common.Address{}, false, fmt.Errorf(
			"invalid token %q: use a 0x address, a known symbol (usdv), or empty/\"native\"/\"svp\" for native SVP", s)
	}
	addr = common.HexToAddress(t)
	if addr == (common.Address{}) {
		return common.Address{}, true, nil // 0x0 is the native sentinel
	}
	return addr, false, nil
}
func (h *Handlers) evmCall(ctx context.Context, to common.Address, data []byte) ([]byte, error) {
	return evmCallOn(ctx, h.Deps.Chain.EVM, to, data)
}

// evmCallOn is evmCall against an explicit client — used by inbound bridging to
// read state (e.g. an allowance) on a foreign chain's RPC rather than the home one.
func evmCallOn(ctx context.Context, client chain.EVMClient, to common.Address, data []byte) ([]byte, error) {
	return client.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data})
}

// tokenDecimals returns a token's decimals — 18 for native SVP, otherwise the
// on-chain decimals() getter. Used to convert human amounts at the boundary.
func (h *Handlers) tokenDecimals(ctx context.Context, native bool, token common.Address) (int64, error) {
	if native {
		return 18, nil
	}
	uni := h.Deps.EVM.Uniswap
	data, err := uni.PackDecimals()
	if err != nil {
		return 0, err
	}
	out, err := h.evmCall(ctx, token, data)
	if err != nil {
		return 0, fmt.Errorf("read decimals for %s (is it an ERC-20?): %w", token.Hex(), err)
	}
	dec, err := uni.UnpackDecimals(out)
	if err != nil {
		return 0, fmt.Errorf("read decimals for %s: %w", token.Hex(), err)
	}
	return int64(dec), nil
}

// erc20Balance reads balanceOf(account) for an ERC-20 token off chain.
func (h *Handlers) erc20Balance(ctx context.Context, token, account common.Address) (*big.Int, error) {
	uni := h.Deps.EVM.Uniswap
	data, err := uni.PackBalanceOf(account)
	if err != nil {
		return nil, err
	}
	out, err := h.evmCall(ctx, token, data)
	if err != nil {
		return nil, fmt.Errorf("read balanceOf for %s: %w", token.Hex(), err)
	}
	return uni.UnpackBalanceOf(out)
}
func (h *Handlers) requireEVM() error {
	if h.Deps.Chain.EVM == nil || h.Deps.EVM.Assembler == nil {
		return userErrf("EVM is not enabled on this server (no evm_rpc_url configured)")
	}
	return nil
}

// assembleERC is the shared tail: stamp a ready-to-sign EVMTxPayload for a
// value-0 contract call from the tenant owner to `to` with `data`, using the
// given assembler (home for most tools; a foreign assembler when a tool targets
// another chain). The assembler's bound client fixes the stamped chain id.
func (h *Handlers) assembleERC(
	ctx context.Context, asm *builder.EVMAssembler, owner string, to common.Address, data []byte, clientID, toolName, desc string,
) (*payload.EVMTxPayload, error) {
	from, err := ownerEthAddress(owner)
	if err != nil {
		return nil, err
	}
	return asm.Assemble(ctx, builder.EVMArgs{
		ClientID: clientID,
		From:     from,
		To:       to,
		Data:     data,
		Summary: payload.EVMSummary{
			ToolName:    toolName,
			Description: desc,
		},
	})
}

// -- build_erc20_transfer ----------------------------------------------
