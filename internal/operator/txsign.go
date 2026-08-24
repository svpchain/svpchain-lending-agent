package operator

import (
	"context"
	"fmt"
	"math"
	"math/big"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/cosmos/gogoproto/proto"

	clobtypes "github.com/dydxprotocol/v4-chain/protocol/x/clob/types"

	"github.com/svpchain/svpchain-lending-agent/internal/mcp/builder"
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/chain"
	"github.com/svpchain/svpchain-lending-agent/internal/mcp/payload"
)

// FeeSpec is the fee stamped onto fee-paying (non-short-term-CLOB) txs,
// mirroring the [fee] config the payload assembler uses.
type FeeSpec struct {
	Denom    string
	Amount   string
	GasLimit uint64
}

// DynamicFeeSpec configures simulation-backed fee pricing. GasPrice is in the
// fee denom's smallest unit per Cosmos gas unit.
type DynamicFeeSpec struct {
	Enabled       bool
	GasPrice      string
	GasAdjustment float64
	MaxGasLimit   uint64
}

// SignTxWithSimulatedFee signs once for simulation at MaxGasLimit, then signs
// again using the simulated gas with the configured adjustment. The simulated
// fee is deliberately priced at the maximum limit so the ante handler accepts
// it even on chains enforcing a minimum gas price.
func SignTxWithSimulatedFee(
	ctx context.Context,
	priv *ethsecp256k1.PrivKey,
	chainID string,
	acct chain.AccountInfo,
	msgs []sdk.Msg,
	fee FeeSpec,
	dynamic DynamicFeeSpec,
	gasFree bool,
	simulator chain.SimulationClient,
) ([]byte, FeeSpec, error) {
	if gasFree || !dynamic.Enabled {
		raw, err := SignTx(priv, chainID, acct, msgs, fee, gasFree)
		return raw, fee, err
	}
	if simulator == nil {
		return nil, FeeSpec{}, fmt.Errorf("dynamic fee is enabled but transaction simulation is unavailable")
	}
	if dynamic.MaxGasLimit == 0 {
		return nil, FeeSpec{}, fmt.Errorf("dynamic fee max gas limit must be positive")
	}
	if dynamic.GasAdjustment < 1 || math.IsNaN(dynamic.GasAdjustment) || math.IsInf(dynamic.GasAdjustment, 0) {
		return nil, FeeSpec{}, fmt.Errorf("dynamic fee gas adjustment must be finite and at least 1")
	}

	price, ok := sdkmath.NewIntFromString(dynamic.GasPrice)
	if !ok || !price.IsPositive() {
		return nil, FeeSpec{}, fmt.Errorf("dynamic fee gas price %q must be a positive integer", dynamic.GasPrice)
	}
	provisional := fee
	provisional.GasLimit = dynamic.MaxGasLimit
	provisional.Amount = feeAmount(price, provisional.GasLimit)
	raw, err := SignTx(priv, chainID, acct, msgs, provisional, false)
	if err != nil {
		return nil, FeeSpec{}, err
	}
	simulated, err := simulator.Simulate(ctx, raw)
	if err != nil {
		return nil, FeeSpec{}, fmt.Errorf("simulate transaction gas: %w", err)
	}
	gasLimit := uint64(math.Ceil(float64(simulated.GasUsed) * dynamic.GasAdjustment))
	if gasLimit == 0 {
		return nil, FeeSpec{}, fmt.Errorf("simulate transaction gas: gas used is zero")
	}
	if gasLimit > dynamic.MaxGasLimit {
		return nil, FeeSpec{}, fmt.Errorf("simulated gas limit %d exceeds configured fee.max_gas_limit %d", gasLimit, dynamic.MaxGasLimit)
	}
	finalFee := fee
	finalFee.GasLimit = gasLimit
	finalFee.Amount = feeAmount(price, gasLimit)
	raw, err = SignTx(priv, chainID, acct, msgs, finalFee, false)
	if err != nil {
		return nil, FeeSpec{}, err
	}
	return raw, finalFee, nil
}

func feeAmount(price sdkmath.Int, gasLimit uint64) string {
	gas := new(big.Int).SetUint64(gasLimit)
	return new(big.Int).Mul(price.BigInt(), gas).String()
}

// SignTx builds and signs a transaction in one step: Any-packs msgs into a
// TxBody, builds a SIGN_MODE_DIRECT AuthInfo, signs with priv, and returns
// the marshaled TxRaw ready for BroadcastSync.
//
// This exists because signer.Sign — the client-side payload signer — always
// stamps an empty fee (the CLOB path). A delegated stateful order or a
// self-registration pays fee, and a tx whose declared fee is absent from
// AuthInfo is rejected. gasFree selects between the two regimes; use
// GasFreeFor to derive it from the messages.
func SignTx(priv *ethsecp256k1.PrivKey, chainID string, acct chain.AccountInfo, msgs []sdk.Msg, fee FeeSpec, gasFree bool) ([]byte, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages to sign")
	}

	anys := make([]*codectypes.Any, 0, len(msgs))
	for i, m := range msgs {
		a, err := codectypes.NewAnyWithValue(m)
		if err != nil {
			return nil, fmt.Errorf("pack msg[%d]: %w", i, err)
		}
		anys = append(anys, a)
	}
	bodyBytes, err := proto.Marshal(&txtypes.TxBody{Messages: anys})
	if err != nil {
		return nil, fmt.Errorf("marshal TxBody: %w", err)
	}

	var feeCoins []sdk.Coin
	if !gasFree {
		amt, ok := sdkmath.NewIntFromString(fee.Amount)
		if !ok {
			return nil, fmt.Errorf("fee amount %q is not a valid integer", fee.Amount)
		}
		if !amt.IsZero() {
			feeCoins = []sdk.Coin{{Denom: fee.Denom, Amount: amt}}
		}
	}

	pub := priv.PubKey()
	pubAny, err := codectypes.NewAnyWithValue(pub)
	if err != nil {
		return nil, fmt.Errorf("pack pubkey: %w", err)
	}
	authInfo := &txtypes.AuthInfo{
		SignerInfos: []*txtypes.SignerInfo{{
			PublicKey: pubAny,
			ModeInfo: &txtypes.ModeInfo{
				Sum: &txtypes.ModeInfo_Single_{
					Single: &txtypes.ModeInfo_Single{Mode: signing.SignMode_SIGN_MODE_DIRECT},
				},
			},
			Sequence: acct.Sequence,
		}},
		Fee: &txtypes.Fee{Amount: feeCoins, GasLimit: fee.GasLimit},
	}
	authInfoBytes, err := proto.Marshal(authInfo)
	if err != nil {
		return nil, fmt.Errorf("marshal AuthInfo: %w", err)
	}

	signBytes, err := payload.DirectSignBytes(bodyBytes, authInfoBytes, chainID, acct.AccountNumber)
	if err != nil {
		return nil, fmt.Errorf("compute sign-bytes: %w", err)
	}
	sig, err := priv.Sign(signBytes)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	txRaw, err := proto.Marshal(&txtypes.TxRaw{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		Signatures:    [][]byte{sig},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal TxRaw: %w", err)
	}
	return txRaw, nil
}

// GasFreeFor reports whether msgs ride the chain's gas-free short-term CLOB
// route. A MsgAgentExecDelegated wrapper is judged by its *inner* message —
// the clob ante unwraps it the same way — so a wrapped short-term order stays
// fee-free while a wrapped stateful order pays.
func GasFreeFor(msgs []sdk.Msg) bool {
	unwrapped := make([]sdk.Msg, len(msgs))
	for i, m := range msgs {
		unwrapped[i] = clobtypes.UnwrapDelegatedMsg(m)
	}
	return builder.IsShortTermClobMsgs(unwrapped)
}
