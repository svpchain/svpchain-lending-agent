package delegated

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/svpchain/svpdt"

	wallettypes "github.com/dydxprotocol/v4-chain/protocol/x/agentwallet/types"
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/builder"
)

// The cToken the fixture's Lendora credential is scoped to. Mixed case on
// input, lowercase in the caveat — the service must reconcile them.
const (
	lendCTokenMixed = "0x00000000000000000000000000000000000000Ce"
	lendCTokenLower = "0x00000000000000000000000000000000000000ce"
	lendComptroller = "0x000000000000000000000000000000000000c0de"
)

var lendoraMethods = []string{
	"mint(uint256)",
	"redeem(uint256)",
	"redeemUnderlying(uint256)",
	"borrow(uint256)",
	"repayBorrow(uint256)",
	"enterMarkets(address[])",
	"exitMarket(address)",
}

func withLendora(t *testing.T, f *fixture) {
	t.Helper()
	l, err := builder.NewLendora(common.HexToAddress(lendComptroller))
	if err != nil {
		t.Fatal(err)
	}
	f.svc.cfg.Lendora = l
	f.svc.cfg.LendoraMethods = append([]string(nil), lendoraMethods...)
}

// lendoraProof grants the generic contract-call action, cToken contract, and
// the method Task derived from the principal, contract, and ABI selector.
func lendoraProof(t *testing.T, f *fixture, contract, method string) []string {
	t.Helper()
	selector := crypto.Keccak256([]byte(method))[:wallettypes.EVMSelectorLen]
	call := &wallettypes.MsgEVMCall{Principal: testDelegator, Contract: contract, Data: selector}
	return f.issue(t, func(p *svpdt.IssueParams) {
		p.Caveats.Actions = svpdt.StringSet{ActionEVMContractCall}
		p.Caveats.Subaccounts = svpdt.Uint32Set{0}
		p.Caveats.Contracts = svpdt.StringSet{contract}
		p.Caveats.Task = call.DelegationMethodTask()
	})
}

// A granted supply broadcasts a wrapper whose inner MsgEVMCall names the
// principal, the lowercased cToken, and mint(uint256) calldata for the amount.
func TestExecuteLendoraSupplyBuildsAnEVMCallForThePrincipal(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	res, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, lendCTokenLower, "mint(uint256)"),
		Supply: LendoraParams{CToken: lendCTokenMixed, Amount: "1000000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Principal != testDelegator {
		t.Fatalf("principal = %s", res.Principal)
	}

	var wrapper wallettypes.MsgAgentExecDelegated
	decodeSoleTxMsg(t, f.broadcast.txBytes, "/dydxprotocol.agentwallet.MsgAgentExecDelegated", &wrapper)

	var inner wallettypes.MsgEVMCall
	if err := inner.Unmarshal(wrapper.InnerMsg.Value); err != nil {
		t.Fatal(err)
	}
	if inner.Principal != testDelegator {
		t.Errorf("inner principal = %s, want %s", inner.Principal, testDelegator)
	}
	if inner.Contract != lendCTokenLower {
		t.Errorf("inner contract = %s, want lowercase %s", inner.Contract, lendCTokenLower)
	}

	wantSelector := crypto.Keccak256([]byte("mint(uint256)"))[:4]
	if got := inner.Data[:4]; string(got) != string(wantSelector) {
		t.Errorf("selector = %x, want mint %x", got, wantSelector)
	}
	// The 32-byte argument must be the amount, big-endian.
	wantArg := make([]byte, 32)
	big.NewInt(1_000_000).FillBytes(wantArg)
	if got := inner.Data[4:]; string(got) != string(wantArg) {
		t.Errorf("amount arg = %x, want %x", got, wantArg)
	}
}

func TestExecuteLendoraRepayUsesTheRepaySelector(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	_, err := f.svc.ExecuteLendoraRepay(context.Background(), ExecLendoraRepayInput{
		Proof: lendoraProof(t, f, lendCTokenLower, "repayBorrow(uint256)"),
		Repay: LendoraParams{CToken: lendCTokenLower, Amount: "5"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var wrapper wallettypes.MsgAgentExecDelegated
	decodeSoleTxMsg(t, f.broadcast.txBytes, "/dydxprotocol.agentwallet.MsgAgentExecDelegated", &wrapper)
	var inner wallettypes.MsgEVMCall
	if err := inner.Unmarshal(wrapper.InnerMsg.Value); err != nil {
		t.Fatal(err)
	}
	want := crypto.Keccak256([]byte("repayBorrow(uint256)"))[:4]
	if string(inner.Data[:4]) != string(want) {
		t.Errorf("selector = %x, want repayBorrow %x", inner.Data[:4], want)
	}
}

func TestExecuteEVMContractMethodBuildsConfiguredLendoraCall(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	_, err := f.svc.ExecuteEVMContractMethod(context.Background(), ExecEVMContractMethodInput{
		Proof: lendoraProof(t, f, lendCTokenLower, "borrow(uint256)"),
		Call: EVMContractMethodCall{
			Contract: lendCTokenMixed,
			Method:   "borrow(uint256)",
			Args:     []string{"42"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var wrapper wallettypes.MsgAgentExecDelegated
	decodeSoleTxMsg(t, f.broadcast.txBytes, "/dydxprotocol.agentwallet.MsgAgentExecDelegated", &wrapper)
	var inner wallettypes.MsgEVMCall
	if err := inner.Unmarshal(wrapper.InnerMsg.Value); err != nil {
		t.Fatal(err)
	}
	want := crypto.Keccak256([]byte("borrow(uint256)"))[:4]
	if string(inner.Data[:4]) != string(want) {
		t.Errorf("selector = %x, want borrow %x", inner.Data[:4], want)
	}
}

func TestExecuteEVMContractMethodBuildsComptrollerEnterMarkets(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)
	f.svc.cfg.IsLendoraContract = func(contract string) bool { return contract == lendCTokenLower }

	_, err := f.svc.ExecuteEVMContractMethod(context.Background(), ExecEVMContractMethodInput{
		Proof: lendoraProof(t, f, lendComptroller, "enterMarkets(address[])"),
		Call: EVMContractMethodCall{
			Contract: lendComptroller,
			Method:   "enterMarkets(address[])",
			Args:     [][]string{{lendCTokenMixed}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var wrapper wallettypes.MsgAgentExecDelegated
	decodeSoleTxMsg(t, f.broadcast.txBytes, "/dydxprotocol.agentwallet.MsgAgentExecDelegated", &wrapper)
	var inner wallettypes.MsgEVMCall
	if err := inner.Unmarshal(wrapper.InnerMsg.Value); err != nil {
		t.Fatal(err)
	}
	if inner.Contract != lendComptroller {
		t.Errorf("contract = %s, want Comptroller %s", inner.Contract, lendComptroller)
	}
	want := crypto.Keccak256([]byte("enterMarkets(address[])"))[:4]
	if string(inner.Data[:4]) != string(want) {
		t.Errorf("selector = %x, want enterMarkets %x", inner.Data[:4], want)
	}
}

func TestExecuteEVMContractMethodRefusesComptrollerCallOutsideConfiguredMarkets(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)
	f.svc.cfg.IsLendoraContract = func(string) bool { return false }

	_, err := f.svc.ExecuteEVMContractMethod(context.Background(), ExecEVMContractMethodInput{
		Call: EVMContractMethodCall{
			Contract: lendComptroller,
			Method:   "enterMarkets(address[])",
			Args:     [][]string{{lendCTokenLower}},
		},
	})
	if err == nil {
		t.Fatal("expected refusal for a market not discovered from the configured Comptroller")
	}
}

func TestExecuteEVMContractMethodRefusesComptrollerMethodOnAnotherContract(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	_, err := f.svc.ExecuteEVMContractMethod(context.Background(), ExecEVMContractMethodInput{
		Call: EVMContractMethodCall{
			Contract: lendCTokenLower,
			Method:   "exitMarket(address)",
			Args:     []string{lendCTokenLower},
		},
	})
	if err == nil {
		t.Fatal("expected refusal for a Comptroller method on a cToken target")
	}
}

func TestExecuteEVMContractMethodRefusesInvalidArguments(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	_, err := f.svc.ExecuteEVMContractMethod(context.Background(), ExecEVMContractMethodInput{
		Call: EVMContractMethodCall{Method: "mint(uint256)", Args: []string{"1", "2"}},
	})
	if err == nil {
		t.Fatal("expected refusal for an invalid ABI argument count")
	}
	if f.broadcast.txBytes != nil {
		t.Error("nothing should have been broadcast")
	}
}

func TestExecuteEVMContractMethodRefusesMethodAbsentFromConfig(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)
	f.svc.cfg.LendoraMethods = []string{"mint(uint256)"}

	_, err := f.svc.ExecuteEVMContractMethod(context.Background(), ExecEVMContractMethodInput{
		Proof: lendoraProof(t, f, lendCTokenLower, "borrow(uint256)"),
		Call: EVMContractMethodCall{
			Contract: lendCTokenLower,
			Method:   "borrow(uint256)",
			Args:     []string{"1"},
		},
	})
	if err == nil {
		t.Fatal("expected refusal for a method absent from config")
	}
	if f.broadcast.txBytes != nil {
		t.Error("nothing should have been broadcast")
	}
}

func TestExecuteEVMContractMethodRefusesAContractOutsideConfiguredMarkets(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)
	f.svc.cfg.IsLendoraContract = func(string) bool { return false }

	_, err := f.svc.ExecuteEVMContractMethod(context.Background(), ExecEVMContractMethodInput{
		Call: EVMContractMethodCall{
			Contract: lendCTokenLower,
			Method:   "mint(uint256)",
			Args:     []string{"1"},
		},
	})
	if err == nil {
		t.Fatal("expected refusal for a contract outside configured Lendora markets")
	}
	if f.broadcast.txBytes != nil {
		t.Error("nothing should have been broadcast")
	}
}

func TestExecuteLendoraRefusesAContractOutsideTheCredential(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	// Credential grants a different contract than the one the call names.
	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, "0x0000000000000000000000000000000000000abc", "mint(uint256)"),
		Supply: LendoraParams{CToken: lendCTokenLower, Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for an ungranted contract")
	}
	if f.broadcast.txBytes != nil {
		t.Error("nothing should have been broadcast")
	}
}

func TestExecuteLendoraRefusesAMismatchedMethodTask(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	// Credential grants borrow, but the call is a supply.
	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, lendCTokenLower, "borrow(uint256)"),
		Supply: LendoraParams{CToken: lendCTokenLower, Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for a mismatched method task")
	}
}

func TestExecuteLendoraRefusesALegacyLendoraAction(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	proof := f.issue(t, func(p *svpdt.IssueParams) {
		p.Caveats.Actions = svpdt.StringSet{"lendora.supply"}
		p.Caveats.Subaccounts = svpdt.Uint32Set{0}
		p.Caveats.Contracts = svpdt.StringSet{lendCTokenLower}
	})
	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  proof,
		Supply: LendoraParams{CToken: lendCTokenLower, Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for a legacy Lendora action")
	}
	if f.broadcast.txBytes != nil {
		t.Error("nothing should have been broadcast")
	}
}

func TestExecuteLendoraRefusesAMethodAbsentFromConfig(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)
	f.svc.cfg.LendoraMethods = []string{"borrow(uint256)"}

	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, lendCTokenLower, "mint(uint256)"),
		Supply: LendoraParams{CToken: lendCTokenLower, Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for a method absent from config")
	}
	if f.broadcast.txBytes != nil {
		t.Error("nothing should have been broadcast")
	}
}

func TestExecuteLendoraRefusesWhenUnconfigured(t *testing.T) {
	f := newFixture(t) // no withLendora: cfg.Lendora stays nil

	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, lendCTokenLower, "mint(uint256)"),
		Supply: LendoraParams{CToken: lendCTokenLower, Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal when Lendora is not configured")
	}
}

func TestExecuteLendoraRefusesAMalformedContract(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, lendCTokenLower, "mint(uint256)"),
		Supply: LendoraParams{CToken: "not-an-address", Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for a malformed contract")
	}
}
