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

func withLendora(t *testing.T, f *fixture) {
	t.Helper()
	l, err := builder.NewLendora(common.HexToAddress(lendComptroller))
	if err != nil {
		t.Fatal(err)
	}
	f.svc.cfg.Lendora = l
}

// lendoraProof grants a Lendora action on the cToken, plus subaccount 0 which
// account-level EVM calls pin.
func lendoraProof(t *testing.T, f *fixture, action string, contracts ...string) []string {
	t.Helper()
	return f.issue(t, func(p *svpdt.IssueParams) {
		p.Caveats.Actions = svpdt.StringSet{action}
		p.Caveats.Subaccounts = svpdt.Uint32Set{0}
		p.Caveats.Contracts = svpdt.StringSet(contracts)
	})
}

// A granted supply broadcasts a wrapper whose inner MsgEVMCall names the
// principal, the lowercased cToken, and mint(uint256) calldata for the amount.
func TestExecuteLendoraSupplyBuildsAnEVMCallForThePrincipal(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	res, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, ActionLendoraSupply, lendCTokenLower),
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
		Proof: lendoraProof(t, f, ActionLendoraRepay, lendCTokenLower),
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

func TestExecuteLendoraRefusesAContractOutsideTheCredential(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	// Credential grants a different contract than the one the call names.
	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, ActionLendoraSupply, "0x0000000000000000000000000000000000000abc"),
		Supply: LendoraParams{CToken: lendCTokenLower, Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for an ungranted contract")
	}
	if f.broadcast.txBytes != nil {
		t.Error("nothing should have been broadcast")
	}
}

func TestExecuteLendoraRefusesAnUngrantedAction(t *testing.T) {
	f := newFixture(t)
	withLendora(t, f)

	// Credential grants borrow, but the call is a supply.
	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, ActionLendoraBorrow, lendCTokenLower),
		Supply: LendoraParams{CToken: lendCTokenLower, Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for an ungranted action")
	}
}

func TestExecuteLendoraRefusesWhenUnconfigured(t *testing.T) {
	f := newFixture(t) // no withLendora: cfg.Lendora stays nil

	_, err := f.svc.ExecuteLendoraSupply(context.Background(), ExecLendoraSupplyInput{
		Proof:  lendoraProof(t, f, ActionLendoraSupply, lendCTokenLower),
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
		Proof:  lendoraProof(t, f, ActionLendoraSupply, lendCTokenLower),
		Supply: LendoraParams{CToken: "not-an-address", Amount: "1"},
	})
	if err == nil {
		t.Fatal("expected refusal for a malformed contract")
	}
}
