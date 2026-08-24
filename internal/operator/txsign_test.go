package operator

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/cosmos/gogoproto/proto"

	"github.com/svpchain/svpchain-lending-agent/internal/mcp/chain"
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/payload"
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/signer"
)

func newKey(t *testing.T) *ethsecp256k1.PrivKey {
	t.Helper()
	bz := make([]byte, 32)
	if _, err := rand.Read(bz); err != nil {
		t.Fatal(err)
	}
	return &ethsecp256k1.PrivKey{Key: bz}
}

func signAndDecode(t *testing.T, priv *ethsecp256k1.PrivKey, gasFree bool) (*txtypes.TxRaw, *txtypes.AuthInfo) {
	t.Helper()
	owner := signer.DeriveAddress(priv)
	msg := &banktypes.MsgSend{
		FromAddress: owner,
		ToAddress:   owner,
		Amount:      sdk.NewCoins(),
	}
	fee := FeeSpec{Denom: "asvp", Amount: "25000000000000000", GasLimit: 1_000_000}
	raw, err := SignTx(priv, "test-chain", chain.AccountInfo{AccountNumber: 3, Sequence: 9}, []sdk.Msg{msg}, fee, gasFree)
	if err != nil {
		t.Fatal(err)
	}
	var txRaw txtypes.TxRaw
	if err := proto.Unmarshal(raw, &txRaw); err != nil {
		t.Fatal(err)
	}
	var authInfo txtypes.AuthInfo
	if err := proto.Unmarshal(txRaw.AuthInfoBytes, &authInfo); err != nil {
		t.Fatal(err)
	}
	return &txRaw, &authInfo
}

// The signature must re-verify against the exact SIGN_MODE_DIRECT sign-bytes
// the chain will recompute — the core property signer.Sign has, minus its
// fee bug.
func TestSignTxSignatureVerifies(t *testing.T) {
	priv := newKey(t)
	txRaw, _ := signAndDecode(t, priv, false)

	signBytes, err := payload.DirectSignBytes(txRaw.BodyBytes, txRaw.AuthInfoBytes, "test-chain", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(txRaw.Signatures) != 1 || !priv.PubKey().VerifySignature(signBytes, txRaw.Signatures[0]) {
		t.Fatal("signature does not verify against the direct sign-bytes")
	}
}

// The fee must be present exactly when the tx is not gas-free — the defect in
// signer.Sign this path exists to avoid.
func TestSignTxFeePresenceFollowsGasFree(t *testing.T) {
	priv := newKey(t)

	_, feePaying := signAndDecode(t, priv, false)
	if len(feePaying.Fee.Amount) != 1 || feePaying.Fee.Amount[0].Denom != "asvp" {
		t.Errorf("fee-paying tx must carry the configured fee, got %v", feePaying.Fee.Amount)
	}
	if feePaying.SignerInfos[0].Sequence != 9 {
		t.Errorf("sequence = %d", feePaying.SignerInfos[0].Sequence)
	}

	_, gasFree := signAndDecode(t, priv, true)
	if len(gasFree.Fee.Amount) != 0 {
		t.Errorf("gas-free tx must carry an empty fee, got %v", gasFree.Fee.Amount)
	}
}

type fixedSimulation struct {
	gasUsed uint64
	txBytes []byte
}

func (s *fixedSimulation) Simulate(_ context.Context, txBytes []byte) (chain.SimulationResult, error) {
	s.txBytes = txBytes
	return chain.SimulationResult{GasUsed: s.gasUsed}, nil
}

func TestSignTxWithSimulatedFeeRepricesAndResigns(t *testing.T) {
	priv := newKey(t)
	owner := signer.DeriveAddress(priv)
	msg := &banktypes.MsgSend{FromAddress: owner, ToAddress: owner}
	sim := &fixedSimulation{gasUsed: 800_000}

	raw, fee, err := SignTxWithSimulatedFee(
		context.Background(), priv, "test-chain", chain.AccountInfo{AccountNumber: 3, Sequence: 9}, []sdk.Msg{msg},
		FeeSpec{Denom: "asvp", Amount: "1", GasLimit: 1},
		DynamicFeeSpec{Enabled: true, GasPrice: "25000000000", GasAdjustment: 1.25, MaxGasLimit: 2_000_000},
		false, sim,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(sim.txBytes) == 0 {
		t.Fatal("simulation was not called")
	}
	if fee.GasLimit != 1_000_000 || fee.Amount != "25000000000000000" {
		t.Fatalf("final fee = %+v, want 1,000,000 gas and 0.025 SVP", fee)
	}

	var txRaw txtypes.TxRaw
	if err := proto.Unmarshal(raw, &txRaw); err != nil {
		t.Fatal(err)
	}
	var authInfo txtypes.AuthInfo
	if err := proto.Unmarshal(txRaw.AuthInfoBytes, &authInfo); err != nil {
		t.Fatal(err)
	}
	if authInfo.Fee.GasLimit != fee.GasLimit || authInfo.Fee.Amount[0].Amount.String() != fee.Amount {
		t.Fatalf("signed fee = %+v, want %+v", authInfo.Fee, fee)
	}
	signBytes, err := payload.DirectSignBytes(txRaw.BodyBytes, txRaw.AuthInfoBytes, "test-chain", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !priv.PubKey().VerifySignature(signBytes, txRaw.Signatures[0]) {
		t.Fatal("final dynamic-fee signature does not verify")
	}
}

func TestSignTxWithSimulatedFeeRejectsLimitExceeded(t *testing.T) {
	priv := newKey(t)
	owner := signer.DeriveAddress(priv)
	_, _, err := SignTxWithSimulatedFee(
		context.Background(), priv, "test-chain", chain.AccountInfo{}, []sdk.Msg{&banktypes.MsgSend{FromAddress: owner, ToAddress: owner}},
		FeeSpec{Denom: "asvp", Amount: "1", GasLimit: 1},
		DynamicFeeSpec{Enabled: true, GasPrice: "25000000000", GasAdjustment: 1.25, MaxGasLimit: 1_000_000},
		false, &fixedSimulation{gasUsed: 900_000},
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds configured") {
		t.Fatalf("expected configured limit error, got %v", err)
	}
}
