// Package agenttools contains the Lending Agent's small local tool surface.
// DeFi-specific operations live behind the private MCP client.
package agenttools

import (
	"context"
	"errors"
	"fmt"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Service struct {
	client *ethclient.Client
}

func New(rpcURL string) (*Service, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial EVM RPC: %w", err)
	}
	return &Service{client: client}, nil
}

func (s *Service) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

type SignedTx struct {
	RawTxHex string `json:"raw_tx_hex" jsonschema:"0x-prefixed signed transaction bytes"`
}
type BroadcastInput struct {
	ClientID string   `json:"client_id"`
	SignedTx SignedTx `json:"signed_tx"`
}
type BroadcastOutput struct {
	TxHash string `json:"tx_hash"`
}

func (s *Service) Broadcast(ctx context.Context, in BroadcastInput) (BroadcastOutput, error) {
	if in.ClientID == "" {
		return BroadcastOutput{}, fmt.Errorf("missing client_id")
	}
	raw, err := hexutil.Decode(in.SignedTx.RawTxHex)
	if err != nil {
		return BroadcastOutput{}, fmt.Errorf("decode raw_tx_hex: %w", err)
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return BroadcastOutput{}, fmt.Errorf("decode signed evm tx: %w", err)
	}
	if err := s.client.SendTransaction(ctx, &tx); err != nil {
		return BroadcastOutput{}, fmt.Errorf("broadcast evm tx: %w", err)
	}
	return BroadcastOutput{TxHash: tx.Hash().Hex()}, nil
}

type TxStatusInput struct {
	TxHash string `json:"tx_hash" jsonschema:"0x transaction hash"`
}
type TxStatusOutput struct {
	TxHash      string `json:"tx_hash"`
	Status      string `json:"status"`
	BlockNumber int64  `json:"block_number,omitempty"`
	GasUsed     uint64 `json:"gas_used,omitempty"`
}

func (s *Service) TxStatus(ctx context.Context, in TxStatusInput) (TxStatusOutput, error) {
	receipt, err := s.client.TransactionReceipt(ctx, common.HexToHash(in.TxHash))
	if errors.Is(err, ethereum.NotFound) {
		return TxStatusOutput{TxHash: in.TxHash, Status: "pending"}, nil
	}
	if err != nil {
		return TxStatusOutput{}, fmt.Errorf("evm receipt %s: %w", in.TxHash, err)
	}
	status := "failed"
	if receipt.Status == types.ReceiptStatusSuccessful {
		status = "success"
	}
	return TxStatusOutput{TxHash: in.TxHash, Status: status, BlockNumber: receipt.BlockNumber.Int64(), GasUsed: receipt.GasUsed}, nil
}
