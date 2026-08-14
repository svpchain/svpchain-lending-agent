// Command svpchain-lending-agent is the lending A2A agent for the Lendora
// money market on SVP-Chain: market and account reads, risk assessment,
// unsigned supply/withdraw/borrow/repay/collateral tx building with an EVM
// landing rail, self-service auth, faucet, the chain's agent/agentwallet
// modules, the SVP-DT execution core (identity, self-registration, settlement),
// and delegated Lendora execution — supply, redeem, withdraw, borrow and repay
// under an SVP-DT credential — when an operator key is configured.
//
// Everything it serves is implemented under internal/, which was the shared
// svpchain-agent-core library until that repo was retired. The perps and EVM
// DeFi families live in their own binaries.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/svpchain/svpchain-lending-agent/internal/a2aserver"
	"github.com/svpchain/svpchain-lending-agent/internal/config"
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
	// This binary exists to serve the Lendora family — a missing EVM endpoint
	// or comptroller is a boot failure here, not a call-time refusal.
	if err := cfg.RequireLendora(); err != nil {
		return err
	}
	app, err := wire.BuildProfile(ctx, cfg, wire.LendingProfile)
	if err != nil {
		return err
	}
	defer app.Close()
	return a2aserver.StartFullFor(ctx, cfg, app, identity)
}
