package delegated

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	wallettypes "github.com/dydxprotocol/v4-chain/protocol/x/agentwallet/types"
	"github.com/svpchain/svpdt"
)

// ActionEVMContractCall is the chain's generic delegated EVM-call action.
// A Task caveat then narrows that authority to the principal, cToken contract,
// and ABI selector of one Lendora method.
const ActionEVMContractCall = "evm.contract_call"

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

// EVMContractMethodCall is the generic delegated Lendora EVM-call shape. The
// method must be enabled in evm.lendora.methods. Args is decoded according to
// the selected ABI signature: cToken writes take ["uint256"], enterMarkets
// takes [["cToken", ...]], and exitMarket takes ["cToken"].
type EVMContractMethodCall struct {
	Contract string `json:"contract"`
	Method   string `json:"method"`
	Args     any    `json:"args"`
}

// ExecEVMContractMethodInput is intentionally nested under call so a caller
// cannot accidentally flatten contract/method/args into a different tool's
// arguments.
type ExecEVMContractMethodInput struct {
	Proof []string              `json:"proof"`
	Call  EVMContractMethodCall `json:"call"`
}

// preflightEVM checks the caveat facts for an account-level EVM call. The
// chain re-checks these against live state and additionally validates the Task
// caveat against the fully constructed MsgEVMCall.
func preflightEVM(v *svpdt.Verified, contract string) error {
	if !v.Effective.Actions.Has(ActionEVMContractCall) {
		return fmt.Errorf("credential does not grant action %q (granted: %v)", ActionEVMContractCall, v.Effective.Actions)
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
		return "", fmt.Errorf("contract %q is not a valid 0x EVM address", addr)
	}
	return strings.ToLower(common.HexToAddress(addr).Hex()), nil
}

// packLendora is a cToken calldata packer on the Lendora builder.
type packLendora func(amount *big.Int) ([]byte, error)

func (s *Service) permitsLendoraMethod(method string) bool {
	for _, configured := range s.cfg.LendoraMethods {
		if configured == method {
			return true
		}
	}
	return false
}

// executeLendora is the shared body of every delegated Lendora call: verify
// the proof, pre-flight the caveats, pack the calldata for the named amount,
// wrap it in MsgEVMCall for the principal, and broadcast the wrapper.
func (s *Service) executeLendora(
	ctx context.Context,
	proof []string,
	method string,
	p LendoraParams,
	pack packLendora,
) (ExecResult, error) {
	if s.cfg.Lendora == nil {
		return ExecResult{}, fmt.Errorf(
			"delegated Lendora execution is not configured on this deployment (evm.lendora.comptroller_addr)")
	}
	if !s.permitsLendoraMethod(method) {
		return ExecResult{}, fmt.Errorf("Lendora method %q is not configured for delegated execution", method)
	}
	contract, err := canonicalContract(p.CToken)
	if err != nil {
		return ExecResult{}, err
	}
	if s.cfg.IsLendoraContract != nil && !s.cfg.IsLendoraContract(contract) {
		return ExecResult{}, fmt.Errorf("contract %s is not a cToken market of the configured Lendora Comptroller", contract)
	}
	amount, ok := new(big.Int).SetString(strings.TrimSpace(p.Amount), 10)
	if !ok || amount.Sign() < 0 {
		return ExecResult{}, fmt.Errorf("amount %q must be a non-negative integer in the token's base units", p.Amount)
	}

	tokens, verified, err := s.verifyProof(ctx, proof)
	if err != nil {
		return ExecResult{}, err
	}
	if err := preflightEVM(verified, contract); err != nil {
		return ExecResult{}, err
	}

	data, err := pack(amount)
	if err != nil {
		return ExecResult{}, fmt.Errorf("pack %s calldata: %w", method, err)
	}
	return s.executePackedLendora(ctx, tokens, verified, contract, method, data)
}

// executePackedLendora applies the common credential and task binding to
// calldata which has already been ABI encoded for a permitted Lendora target.
func (s *Service) executePackedLendora(
	ctx context.Context,
	tokens [][]byte,
	verified *svpdt.Verified,
	contract, method string,
	data []byte,
) (ExecResult, error) {
	inner := &wallettypes.MsgEVMCall{
		Principal: verified.Principal,
		Contract:  contract,
		Data:      data,
	}
	if err := inner.ValidateBasic(); err != nil {
		return ExecResult{}, err
	}
	if verified.Effective.Task != inner.DelegationMethodTask() {
		return ExecResult{}, fmt.Errorf("credential does not bind Lendora contract method")
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
	return s.executeLendora(ctx, in.Proof, "mint(uint256)", in.Supply, s.cfg.Lendora.PackMint)
}

type ExecLendoraRedeemInput struct {
	Proof  []string      `json:"proof"`
	Redeem LendoraParams `json:"redeem"`
}

// ExecuteLendoraRedeem burns the principal's cTokens for the underlying
// (Compound redeem). Amount is in cToken base units.
func (s *Service) ExecuteLendoraRedeem(ctx context.Context, in ExecLendoraRedeemInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, "redeem(uint256)", in.Redeem, s.cfg.Lendora.PackRedeem)
}

type ExecLendoraWithdrawInput struct {
	Proof    []string      `json:"proof"`
	Withdraw LendoraParams `json:"withdraw"`
}

// ExecuteLendoraWithdraw withdraws a specified amount of underlying from the
// principal's supply (Compound redeemUnderlying). Amount is in underlying base
// units.
func (s *Service) ExecuteLendoraWithdraw(ctx context.Context, in ExecLendoraWithdrawInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, "redeemUnderlying(uint256)", in.Withdraw, s.cfg.Lendora.PackRedeemUnderlying)
}

type ExecLendoraBorrowInput struct {
	Proof  []string      `json:"proof"`
	Borrow LendoraParams `json:"borrow"`
}

// ExecuteLendoraBorrow borrows underlying against the principal's collateral
// (Compound borrow). The borrow is booked against the principal's own account,
// since the EVM sender is the principal.
func (s *Service) ExecuteLendoraBorrow(ctx context.Context, in ExecLendoraBorrowInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, "borrow(uint256)", in.Borrow, s.cfg.Lendora.PackBorrow)
}

type ExecLendoraRepayInput struct {
	Proof []string      `json:"proof"`
	Repay LendoraParams `json:"repay"`
}

// ExecuteLendoraRepay repays the principal's own borrow (Compound
// repayBorrow). Pass the max uint256 as the amount to repay the full balance;
// the principal must have approved the cToken to spend the underlying first.
func (s *Service) ExecuteLendoraRepay(ctx context.Context, in ExecLendoraRepayInput) (ExecResult, error) {
	return s.executeLendora(ctx, in.Proof, "repayBorrow(uint256)", in.Repay, s.cfg.Lendora.PackRepayBorrow)
}

// ExecuteEVMContractMethod is the generic public entry point for the
// configured Lendora cToken writes. It deliberately is not an arbitrary EVM
// call: only the ABI methods this lending agent can encode are accepted, and
// executeLendora additionally checks evm.lendora.methods plus the credential's
// cToken contract and Task caveats before broadcasting.
func (s *Service) ExecuteEVMContractMethod(ctx context.Context, in ExecEVMContractMethodInput) (ExecResult, error) {
	if s.cfg.Lendora == nil {
		return ExecResult{}, fmt.Errorf(
			"delegated Lendora execution is not configured on this deployment (evm.lendora.comptroller_addr)")
	}
	switch in.Call.Method {
	case "mint(uint256)":
		return s.executeCTokenMethod(ctx, in, s.cfg.Lendora.PackMint)
	case "redeem(uint256)":
		return s.executeCTokenMethod(ctx, in, s.cfg.Lendora.PackRedeem)
	case "redeemUnderlying(uint256)":
		return s.executeCTokenMethod(ctx, in, s.cfg.Lendora.PackRedeemUnderlying)
	case "borrow(uint256)":
		return s.executeCTokenMethod(ctx, in, s.cfg.Lendora.PackBorrow)
	case "repayBorrow(uint256)":
		return s.executeCTokenMethod(ctx, in, s.cfg.Lendora.PackRepayBorrow)
	case "enterMarkets(address[])":
		return s.executeEnterMarkets(ctx, in)
	case "exitMarket(address)":
		return s.executeExitMarket(ctx, in)
	default:
		return ExecResult{}, fmt.Errorf("Lendora ABI method %q is not supported by this agent", in.Call.Method)
	}
}

func (s *Service) executeCTokenMethod(ctx context.Context, in ExecEVMContractMethodInput, pack packLendora) (ExecResult, error) {
	args, err := stringArgs(in.Call.Args)
	if err != nil || len(args) != 1 {
		return ExecResult{}, fmt.Errorf("Lendora method %q requires exactly one uint256 argument", in.Call.Method)
	}
	return s.executeLendora(ctx, in.Proof, in.Call.Method,
		LendoraParams{CToken: in.Call.Contract, Amount: args[0]}, pack)
}

func (s *Service) executeEnterMarkets(ctx context.Context, in ExecEVMContractMethodInput) (ExecResult, error) {
	var nested [][]string
	if err := decodeArgs(in.Call.Args, &nested); err != nil || len(nested) != 1 || len(nested[0]) == 0 {
		return ExecResult{}, fmt.Errorf("Lendora method %q requires one non-empty address[] argument", in.Call.Method)
	}
	markets := make([]common.Address, 0, len(nested[0]))
	seen := make(map[string]struct{}, len(nested[0]))
	for _, market := range nested[0] {
		canonical, err := s.requireLendoraMarket(market)
		if err != nil {
			return ExecResult{}, err
		}
		if _, duplicate := seen[canonical]; duplicate {
			return ExecResult{}, fmt.Errorf("Lendora enterMarkets includes duplicate cToken %s", canonical)
		}
		seen[canonical] = struct{}{}
		markets = append(markets, common.HexToAddress(canonical))
	}
	data, err := s.cfg.Lendora.PackEnterMarkets(markets)
	if err != nil {
		return ExecResult{}, fmt.Errorf("pack %s calldata: %w", in.Call.Method, err)
	}
	return s.executeComptrollerMethod(ctx, in.Proof, in.Call.Contract, in.Call.Method, data)
}

func (s *Service) executeExitMarket(ctx context.Context, in ExecEVMContractMethodInput) (ExecResult, error) {
	args, err := stringArgs(in.Call.Args)
	if err != nil || len(args) != 1 {
		return ExecResult{}, fmt.Errorf("Lendora method %q requires exactly one cToken address argument", in.Call.Method)
	}
	market, err := s.requireLendoraMarket(args[0])
	if err != nil {
		return ExecResult{}, err
	}
	data, err := s.cfg.Lendora.PackExitMarket(common.HexToAddress(market))
	if err != nil {
		return ExecResult{}, fmt.Errorf("pack %s calldata: %w", in.Call.Method, err)
	}
	return s.executeComptrollerMethod(ctx, in.Proof, in.Call.Contract, in.Call.Method, data)
}

func (s *Service) executeComptrollerMethod(ctx context.Context, proof []string, suppliedContract, method string, data []byte) (ExecResult, error) {
	if s.cfg.Lendora == nil {
		return ExecResult{}, fmt.Errorf("delegated Lendora execution is not configured on this deployment (evm.lendora.comptroller_addr)")
	}
	if !s.permitsLendoraMethod(method) {
		return ExecResult{}, fmt.Errorf("Lendora method %q is not configured for delegated execution", method)
	}
	contract, err := canonicalContract(suppliedContract)
	if err != nil {
		return ExecResult{}, err
	}
	want := strings.ToLower(s.cfg.Lendora.Comptroller().Hex())
	if contract != want {
		return ExecResult{}, fmt.Errorf("Lendora method %q must target the configured Comptroller %s", method, want)
	}
	tokens, verified, err := s.verifyProof(ctx, proof)
	if err != nil {
		return ExecResult{}, err
	}
	if err := preflightEVM(verified, contract); err != nil {
		return ExecResult{}, err
	}
	return s.executePackedLendora(ctx, tokens, verified, contract, method, data)
}

func (s *Service) requireLendoraMarket(contract string) (string, error) {
	canonical, err := canonicalContract(contract)
	if err != nil {
		return "", err
	}
	if s.cfg.IsLendoraContract != nil && !s.cfg.IsLendoraContract(canonical) {
		return "", fmt.Errorf("contract %s is not a cToken market of the configured Lendora Comptroller", canonical)
	}
	return canonical, nil
}

func stringArgs(args any) ([]string, error) {
	var out []string
	return out, decodeArgs(args, &out)
}

func decodeArgs(args any, out any) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
