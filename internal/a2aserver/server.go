package a2aserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/svpchain/svpchain-lending-agent/internal/config"
	"github.com/svpchain/svpchain-lending-agent/internal/wire"
)

func StartFullFor(ctx context.Context, cfg *config.Config, app *wire.App, ident CardIdentity, intent IntentRunner) error {
	executor := NewExecutor(app.Registry, intent)
	card := BuildAgentCardFor(ident, cfg.PublicURL, app.Registry)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cacheErr := make(chan error, 1)
	go func() { cacheErr <- app.Run(ctx) }()
	serveErr := make(chan error, 1)
	go func() { serveErr <- serve(ctx, cfg.ListenAddr, cfg.PublicURL, executor, card) }()
	select {
	case err := <-cacheErr:
		cancel()
		<-serveErr
		return err
	case err := <-serveErr:
		cancel()
		return err
	}
}

func serve(ctx context.Context, listenAddr, publicURL string, executor *Executor, card *a2a.AgentCard) error {
	mux := http.NewServeMux()
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(executor)))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", listenAddr, err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	fmt.Fprintf(os.Stderr, "%s: listening on %s\n", card.Name, listenAddr)
	fmt.Fprintf(os.Stderr, "%s: agent card at %s%s\n", card.Name, publicURL, a2asrv.WellKnownAgentCardPath)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
