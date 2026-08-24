package chain

import (
	"context"
	"fmt"

	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"google.golang.org/grpc"
)

// SimulationResult is the gas consumed when the chain executes a transaction
// without committing state.
type SimulationResult struct {
	GasUsed uint64
}

// SimulationClient is the Tx.Service simulation surface used to price
// operator-signed stateful transactions before they are broadcast.
type SimulationClient interface {
	Simulate(ctx context.Context, txBytes []byte) (SimulationResult, error)
}

type simulationClient struct {
	inner sdktx.ServiceClient
}

// NewSimulationClient returns a simulation client backed by Cosmos gRPC.
func NewSimulationClient(conn *grpc.ClientConn) SimulationClient {
	return &simulationClient{inner: sdktx.NewServiceClient(conn)}
}

func (c *simulationClient) Simulate(ctx context.Context, txBytes []byte) (SimulationResult, error) {
	resp, err := c.inner.Simulate(ctx, &sdktx.SimulateRequest{TxBytes: txBytes})
	if err != nil {
		return SimulationResult{}, fmt.Errorf("Tx.Service/Simulate: %w", err)
	}
	if resp.GasInfo == nil {
		return SimulationResult{}, fmt.Errorf("Tx.Service/Simulate: empty gas info")
	}
	return SimulationResult{GasUsed: resp.GasInfo.GasUsed}, nil
}
