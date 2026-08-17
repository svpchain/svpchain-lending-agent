package tools

import (
	"bytes"
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// ★ Rescued from upstream tools/bridge_test.go during the absorption of
// svpchain-mcp v0.1.0 (see internal/mcp/doc.go). The bridge tool family and its
// tests were pruned — this binary does not serve them — but evm_test.go, which
// covers the broadcast_evm_tx / evm_tx_status rail this binary DOES serve,
// shares this foreign-chain client fixture with them.

// mockForeignEVM is a chain.EVMClient standing in for a foreign chain's RPC:
// CallContract answers allowance() with a fixed value; the chain-state calls
// return fixed values (ChainID is the foreign chain id) so a real EVMAssembler
// can stamp a payload bound to that chain.
type mockForeignEVM struct {
	chainID   int64
	allowance *big.Int
	decimals  uint8
}

func (m *mockForeignEVM) CallContract(_ context.Context, msg ethereum.CallMsg) ([]byte, error) {
	if bytes.Equal(msg.Data[:4], crypto.Keccak256([]byte("allowance(address,address)"))[:4]) {
		a := m.allowance
		if a == nil {
			a = big.NewInt(0)
		}
		return common.LeftPadBytes(a.Bytes(), 32), nil
	}
	if bytes.Equal(msg.Data[:4], crypto.Keccak256([]byte("decimals()"))[:4]) {
		return common.LeftPadBytes(big.NewInt(int64(m.decimals)).Bytes(), 32), nil
	}
	return nil, nil
}
func (m *mockForeignEVM) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 3, nil
}
func (m *mockForeignEVM) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 80_000, nil
}
func (m *mockForeignEVM) SuggestGasTipCap(context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}
func (m *mockForeignEVM) BaseFee(context.Context) (*big.Int, error) {
	return big.NewInt(2_000_000_000), nil
}
func (m *mockForeignEVM) BlockNumber(context.Context) (uint64, error) { return 0, nil }
func (m *mockForeignEVM) ChainID(context.Context) (*big.Int, error) {
	return big.NewInt(m.chainID), nil
}
func (m *mockForeignEVM) SendTransaction(context.Context, *ethtypes.Transaction) (string, error) {
	return "", nil
}
func (m *mockForeignEVM) TransactionReceipt(context.Context, common.Hash) (*ethtypes.Receipt, error) {
	return nil, nil
}
