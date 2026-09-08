package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-lending-agent/internal/a2aserver"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
)

func TestProxyCardPublishesMetaLendoraAndBroadcast(t *testing.T) {
	reg := toolbridge.NewEmpty()
	require.NoError(t, reg.AddProxy(toolbridge.SkillLendora, "lendora_build_supply_tx", map[string]any{"type": "object"}, func(context.Context, map[string]any) (string, error) { return "{}", nil }))
	require.NoError(t, reg.Add(toolbridge.SkillEVM, "broadcast_evm_tx", toolbridge.Native(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })))
	require.NoError(t, reg.Add(toolbridge.SkillEVM, "evm_tx_status", toolbridge.Native(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })))
	reg.RegisterMeta()
	card := a2aserver.BuildAgentCardFor(identity, "https://lending-agent.example.test", reg)
	require.Len(t, card.Skills, 3)
	for _, skill := range card.Skills {
		require.Contains(t, []string{toolbridge.SkillMeta, toolbridge.SkillLendora, toolbridge.SkillEVM}, skill.ID)
	}
	require.Empty(t, card.Capabilities.Extensions)
	require.True(t, strings.Contains(card.Description, "private DeFi MCP"))
}
