package deps_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svpchain/svpchain-lending-agent/internal/config"
)

// scripts/deploy.sh renders this agent's agent.toml itself. This pins the two
// together: whatever the script prints must parse and validate under core's
// config package, so a schema change that would brick a deploy fails here
// rather than on a remote host.
//
// It lives beside the script rather than in core, because after the split the
// script is this repo's and core cannot see it.
func TestDeployScriptConfigParses(t *testing.T) {
	script, err := filepath.Abs(filepath.Join("scripts", "deploy.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Skipf("deploy script not found: %v", err)
	}

	// A keyed variant needs a real key file: the script validates it is 32
	// bytes of hex before rendering an [operator] block for it.
	keyFile := filepath.Join(t.TempDir(), "operator.key")
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("a1", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]string{
		"keyless": {"--print-config", "--host", "www@agent.example.com"},
		"keyed": {
			"--print-config", "--host", "www@agent.example.com",
			"--operator-key-file", keyFile,
			"--public-url", "https://agents.example.com",
		},
		// The faucet is the only family this agent can be deployed without.
		// Lendora is not optional here: main.go calls cfg.RequireLendora, so
		// a config with no evm_rpc_url or comptroller is rejected at boot —
		// see TestDeployedConfigSatisfiesLendoraGuard.
		"faucet-off": {
			"--print-config", "--host", "www@agent.example.com",
			"--faucet-url", "",
		},
		// The other direction: every optional block the script can render, on
		// at once, so a typo in one of those heredocs fails here rather than
		// on a remote host. [agent_chain] is both-or-neither in core, which
		// this also pins.
		"all-optionals": {
			"--print-config", "--host", "www@agent.example.com",
			"--agent-chain-id", "svp-agent-1",
			"--agent-chain-rest", "http://127.0.0.1:1317",
			"--deposit-max-usdc", "1000", "--withdraw-max-usdc", "500",
			"--transfer-max-usdc", "250", "--daily-withdraw-cap-usdc", "2000",
			"--markets-refresh", "60s",
		},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := exec.Command("bash", append([]string{script}, args...)...).Output()
			if err != nil {
				t.Fatalf("render config: %v", err)
			}
			path := filepath.Join(t.TempDir(), "agent.toml")
			if err := os.WriteFile(path, out, 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("rendered config does not parse/validate:\n%s\nerror: %v", out, err)
			}
			if cfg.PublicURL == "" {
				t.Error("rendered config must carry a public_url")
			}
			if name == "keyed" && cfg.Operator.KeyFile == "" {
				t.Error("keyed variant must set key_file")
			}
			// Every variant must clear the guard the binary applies at boot:
			// the deploy script has no business rendering a config this
			// agent's own main.go would reject.
			if err := cfg.RequireLendora(); err != nil {
				t.Errorf("rendered config would not boot: %v\n%s", err, out)
			}
			if name == "all-optionals" {
				if cfg.AgentChain.RestURL == "" {
					t.Error("all-optionals must render [agent_chain]")
				}
				if cfg.Limits.DepositMaxUSDC != 1000 {
					t.Errorf("deposit_max_usdc = %d, want 1000", cfg.Limits.DepositMaxUSDC)
				}
			}
		})
	}
}

// The route and the card must agree. An agent advertises public_url inside its
// Agent Card, a verifier fetches that URL to recompute the capability hash, and
// nginx is what makes the URL resolve. If the location block and public_url
// disagree on the path segment, the agent advertises a URL that 404s and reads
// as unverified — with every process healthy and nothing in the logs.
//
// Both come from the same two constants, AGENT_PORT and AGENT_SEGMENT, so this
// asserts they stay wired to them rather than to two hand-maintained copies of
// the same fact.
func TestDeployScriptNginxRouteMatchesConfig(t *testing.T) {
	script, err := filepath.Abs(filepath.Join("scripts", "deploy.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Skipf("deploy script not found: %v", err)
	}

	const base = "https://agents.example.com"
	run := func(mode string) string {
		out, err := exec.Command("bash", script, mode, "--host", "www@agent.example.com",
			"--public-url", base).Output()
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		return string(out)
	}

	path := filepath.Join(t.TempDir(), "agent.toml")
	if err := os.WriteFile(path, []byte(run("--print-config")), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	wantSeg := strings.TrimPrefix(cfg.PublicURL, base) // "/perps"
	if wantSeg == "" || wantSeg == cfg.PublicURL {
		t.Fatalf("public_url %q does not extend the base %q with a path segment", cfg.PublicURL, base)
	}
	wantPort := cfg.ListenAddr[strings.LastIndex(cfg.ListenAddr, ":"):] // ":8082"

	nginx := run("--print-nginx")
	if want := "location " + wantSeg + "/ {"; !strings.Contains(nginx, want) {
		t.Errorf("nginx block does not route the advertised path.\nwant %q\ngot:\n%s", want, nginx)
	}
	if want := "proxy_pass http://127.0.0.1" + wantPort + "/;"; !strings.Contains(nginx, want) {
		t.Errorf("nginx block does not proxy to the configured port.\nwant %q\ngot:\n%s", want, nginx)
	}
	// Without the trailing slash on proxy_pass the prefix is forwarded, and the
	// agent — which binds at root — 404s every request through the proxy.
	if strings.Contains(nginx, "proxy_pass http://127.0.0.1"+wantPort+";") {
		t.Error("proxy_pass must end in / so the path segment is stripped")
	}
}
