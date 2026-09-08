package wire

import (
	"context"
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-lending-agent/internal/agenttools"
	"github.com/svpchain/svpchain-lending-agent/internal/config"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
)

// BuildProxy creates the MCP-backed lending runtime. BuildProfile remains for
// historical source compatibility but is not used by the production binary.
func BuildProxy(_ context.Context, cfg *config.Config) (*App, error) {
	if err := cfg.RequireDeFiMCP(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.DEXChain.EVMRPCURL) == "" {
		return nil, fmt.Errorf("dex_chain.evm_rpc_url is required for broadcast_evm_tx")
	}
	service, err := agenttools.New(cfg.DEXChain.EVMRPCURL)
	if err != nil {
		return nil, err
	}
	registry := toolbridge.NewEmpty()
	for _, item := range []struct {
		name  string
		bound toolbridge.Bound
	}{
		{name: "broadcast_evm_tx", bound: toolbridge.Native(service.Broadcast)},
		{name: "evm_tx_status", bound: toolbridge.Native(service.TxStatus)},
	} {
		if err := registry.Add(toolbridge.SkillEVM, item.name, item.bound); err != nil {
			service.Close()
			return nil, err
		}
	}
	return &App{Registry: registry, Tools: service}, nil
}
