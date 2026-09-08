// Command svpchain-lending-agent is the public Lendora A2A relay. It
// synchronizes a token-scoped private DeFi MCP catalog at startup, serves its
// Lendora tools, and provides the caller-signed EVM broadcast landing rail.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/svpchain/svpchain-lending-agent/internal/a2aserver"
	"github.com/svpchain/svpchain-lending-agent/internal/agentrunner"
	"github.com/svpchain/svpchain-lending-agent/internal/config"
	"github.com/svpchain/svpchain-lending-agent/internal/defimcp"
	"github.com/svpchain/svpchain-lending-agent/internal/llm"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
	"github.com/svpchain/svpchain-lending-agent/internal/wire"
)

func main() {
	configPath := flag.String("config", "", "TOML config (see internal/config)")
	flag.Parse()

	// Stop cleanly on SIGINT/SIGTERM so a container orchestrator gets a prompt
	// exit rather than a killed process.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *configPath); err != nil {
		fmt.Fprintf(os.Stderr, "svpchain-lending-agent: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("-config is required")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.RequireDeFiMCP(); err != nil {
		return err
	}
	app, err := wire.BuildProxy(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.Close()
	mcpClient, err := defimcp.Connect(ctx, cfg.DeFiMCP.URL, cfg.DeFiMCP.AuthToken, time.Duration(cfg.DeFiMCP.Timeout))
	if err != nil {
		return err
	}
	defer mcpClient.Close()
	registered := 0
	for _, tool := range mcpClient.Tools() {
		if !isLendoraTool(tool.Name) {
			continue
		}
		name := tool.Name
		if err := app.Registry.AddProxy(toolbridge.SkillLendora, name, tool.InputSchema, func(callCtx context.Context, args map[string]any) (string, error) {
			return mcpClient.Call(callCtx, name, args)
		}); err != nil {
			return err
		}
		registered++
	}
	if registered == 0 {
		return fmt.Errorf("private defi mcp catalog contains no lendora_* tools for this lending-agent token")
	}
	app.Registry.RegisterMeta()
	var runner a2aserver.IntentRunner
	if keyEnv := strings.TrimSpace(cfg.LLM.APIKeyEnv); keyEnv != "" && strings.TrimSpace(os.Getenv(keyEnv)) != "" {
		runner = agentrunner.New(llm.Config{
			Provider: cfg.LLM.Provider,
			BaseURL:  cfg.LLM.BaseURL,
			Model:    cfg.LLM.Model,
			APIKey:   os.Getenv(keyEnv),
		}, mcpClient)
	}
	return a2aserver.StartFullFor(ctx, cfg, app, identity, runner)
}

func isLendoraTool(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "lendora_")
}
