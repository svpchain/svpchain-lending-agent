package delegated

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	wallettypes "github.com/dydxprotocol/v4-chain/protocol/x/agentwallet/types"
	"github.com/svpchain/svpdt"
)

// Lendora EVM-call actions in the chain's delegatable namespace. Each string
// must stay byte-identical to the fork's app/delegation/evm.go constants — a
// caveat written against "lendora.supply" means the same thing at our edge and
// at the chain's Authorize.
const (
	ActionLendoraSupply   = "lendora.supply"
	ActionLendoraRedeem   = "lendora.redeem"
	ActionLendoraWithdraw = "lendora.withdraw"
	ActionLendoraBorrow   = "lendora.borrow"
	ActionLendoraRepay    = "lendora.repay"
)

// LendoraParams is the shared shape of every delegated Lendora call: which
// cToken market, and how much. The principal is deliberately absent — the EVM
// sender is always the credential's principal, forced on chain, so a caller
// cannot act on an account the delegation does not cover.
type LendoraParams struct {
	// CToken is the Lendora cToken market contract, a 0x address. It must be
	// inside the credential's contracts caveat and the delegation's on-chain
	// contract limits, or the chain refuses.
	CToken string `json:"ctoken"`

	// Amount is in the token's own base units, as a decimal integer string —
	// the underlying's base units for supply/withdraw/borrow/repay, the
	// cToken's base units for redeem. No decimal conversion happens here:
	// the exact integer is what the contract receives.
	Amount string `json:"amount"`
}

// preflightEVM checks the caveat facts for an account-level EVM call: the
// action is granted, subaccount 0 is inside the grant (EVM calls pin 0, having
// no subaccount), and the contract is inside the grant. The chain's Authorize
// re-checks all three against live state, plus the delegation's own contract
// limits; this gives the caller a real reason instead of a CheckTx code.
func preflightEVM(v *svpdt.Verified, action, contract string) error {
	if !v.Effective.Actions.Has(action) {
		return fmt.Errorf("credential does not grant action %q (granted: %v)", action, v.Effective.Actions)
	}
	if !v.Effective.Subaccounts.Has(0) {
		return fmt.Errorf("credential does not grant subaccount 0, required for account-level EVM calls (granted: %v)", v.Effective.Subaccounts)
	}
	if !v.Effective.Contracts.Has(contract) {
		return fmt.Errorf("credential does not grant contract %s (granted: %v)", contract, v.Effective.Contracts)
	}
	return nil
}

// canonicalContract normalizes a caller-supplied address to the lowercase
// 0x form contract caveats are written in, so the containment check is byte
// equality regardless of how the caller spelled it. Rejects a non-address up
// front rather than letting the chain refuse an unparseable string.
func canonicalContract(addr string) (string, error) {
	if !common.IsHexAddress(addr) {
		return "", fmt.Errorf("ctoken %q is not a valid 0x EVM address", addr)
	}
	return strings.ToLower(common.HexToAddress(addr).Hex()), nil
}

// packLendora is a cToken calldata packer on the Lendora builder.
type packLendora func(amount *big.Int) ([]byte, error)

// executeLendora is the shared body of every delegated Lendora call: verify
// the proof, pre-flight the caveats, pack the calldata for the named amount,
// wrap it in MsgEVMCall for the principal, and broadcast the wrapper.
func (s *Service) executeLendora(
	ctx context.Context,
	proof []string,
	action string,
	p LendoraParams,
	pack packLendora,
) (ExecResult, error) {
	if s.cfg.Lendora == nil {
		return ExecResult{}, fmt.Errorf(
			"delegated Lendora execution is not configured on this deployment (evm.lendora.comptroller_addr)")
	}
	contract, err := canonicalContract(p.CToken)
	if err != nil {
		return ExecResult{}, err
	}
	amount, ok := new(big.Int).SetString(strings.TrimSpace(p.Amount), 10)
	if !ok || amount.Sign() < 0 {
		return ExecResult{}, fmt.Errorf("amount %q must be a non-negative integer in the token's base units", p.Amount)
	}

	tokens, verified, err := s.verifyProof(ctx, proof)
	if err != nil {
		return ExecResult{}, err
	}
	if err := preflightEVM(verified, action, contract); err != nil {
		return ExecResult{}, err
	}

	data, err := pack(amount)
	if err != nil {
		return ExecResult{}, fmt.Errorf("pack %s calldata: %w", action, err)
	}
	inner := &wallettypes.MsgEVMCall{
		Principal: verified.Principal,
		Contract:  contract,
		Data:      data,
	}
	if err := inner.ValidateBasic(); err != nil {
		return ExecResult{}, err
	}
	return s.execute(ctx, tokens, verified, inner)
}

type ExecLendoraSupplyInput struct {
	Proof  []string      `json:"proof"`
	Supply LendoraParams `json:"supply"`
}

// ExecuteLendoraSupply supplies the principal's underlying into a Lendora
// market (Compound mint), receiving cTokens. The principal must have approved
// the cToken to spend the underlying first — a user action, not delegatable,
// which is the real per-token cap.
func (s *Service) ExecuteLendoraSupply(ctx context.Context, in ExecLendoraSupplyInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, ActionLendoraSupply, in.Supply, s.cfg.Lendora.PackMint)
}

type ExecLendoraRedeemInput struct {
	Proof  []string      `json:"proof"`
	Redeem LendoraParams `json:"redeem"`
}

// ExecuteLendoraRedeem burns the principal's cTokens for the underlying
// (Compound redeem). Amount is in cToken base units.
func (s *Service) ExecuteLendoraRedeem(ctx context.Context, in ExecLendoraRedeemInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, ActionLendoraRedeem, in.Redeem, s.cfg.Lendora.PackRedeem)
}

type ExecLendoraWithdrawInput struct {
	Proof    []string      `json:"proof"`
	Withdraw LendoraParams `json:"withdraw"`
}

// ExecuteLendoraWithdraw withdraws a specified amount of underlying from the
// principal's supply (Compound redeemUnderlying). Amount is in underlying base
// units.
func (s *Service) ExecuteLendoraWithdraw(ctx context.Context, in ExecLendoraWithdrawInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, ActionLendoraWithdraw, in.Withdraw, s.cfg.Lendora.PackRedeemUnderlying)
}

type ExecLendoraBorrowInput struct {
	Proof  []string      `json:"proof"`
	Borrow LendoraParams `json:"borrow"`
}

// ExecuteLendoraBorrow borrows underlying against the principal's collateral
// (Compound borrow). The borrow is booked against the principal's own account,
// since the EVM sender is the principal.
func (s *Service) ExecuteLendoraBorrow(ctx context.Context, in ExecLendoraBorrowInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, ActionLendoraBorrow, in.Borrow, s.cfg.Lendora.PackBorrow)
}

type ExecLendoraRepayInput struct {
	Proof []string      `json:"proof"`
	Repay LendoraParams `json:"repay"`
}

// ExecuteLendoraRepay repays the principal's own borrow (Compound
// repayBorrow). Pass the max uint256 as the amount to repay the full balance;
// the principal must have approved the cToken to spend the underlying first.
func (s *Service) ExecuteLendoraRepay(ctx context.Context, in ExecLendoraRepayInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, ActionLendoraRepay, in.Repay, s.cfg.Lendora.PackRepayBorrow)
}
