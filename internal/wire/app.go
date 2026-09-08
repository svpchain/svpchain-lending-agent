package wire

import (
	"context"

	"github.com/svpchain/svpchain-lending-agent/internal/agenttools"
	"github.com/svpchain/svpchain-lending-agent/internal/toolbridge"
)

// App is the deliberately small lending relay runtime.
type App struct {
	Registry *toolbridge.Registry
	Tools    *agenttools.Service
}

func (a *App) Close() {
	if a.Tools != nil {
		a.Tools.Close()
	}
}
func (a *App) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
