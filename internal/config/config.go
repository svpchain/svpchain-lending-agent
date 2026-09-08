// Package config loads the public Lending Agent configuration.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config contains only the dependencies owned by the public Lending Agent.
// Lendora contracts, assets, and tool policy belong to its private DeFi MCP.
type Config struct {
	ListenAddr string         `toml:"listen_addr"`
	PublicURL  string         `toml:"public_url"`
	DEXChain   DEXChainConfig `toml:"dex_chain"`
	DeFiMCP    DeFiMCPConfig  `toml:"defi_mcp"`
	LLM        LLMConfig      `toml:"llm"`
}

// DEXChainConfig identifies the EVM network used only to broadcast a caller's
// already-signed transaction and query its receipt.
type DEXChainConfig struct {
	ID        string `toml:"id"`
	EVMRPCURL string `toml:"evm_rpc_url"`
}

// DeFiMCPConfig identifies the private MCP endpoint and its relay credential.
type DeFiMCPConfig struct {
	URL       string   `toml:"url"`
	AuthToken string   `toml:"auth_token"`
	Timeout   Duration `toml:"timeout"`
}

// LLMConfig configures the optional natural-language, read-only Lendora
// planner. APIKeyEnv points to a process environment variable.
type LLMConfig struct {
	Provider  string `toml:"provider"`
	BaseURL   string `toml:"base_url"`
	Model     string `toml:"model"`
	APIKeyEnv string `toml:"api_key_env"`
}

// Duration parses TOML durations such as "90s".
type Duration time.Duration

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", string(b), err)
	}
	*d = Duration(v)
	return nil
}

// Load reads and validates a TOML config file.
func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("decode TOML %s: %w", path, err)
	}
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) ApplyDefaults() {
	if c.PublicURL == "" && c.ListenAddr != "" {
		c.PublicURL = "http://localhost" + c.ListenAddr
	}
	if c.DeFiMCP.Timeout == 0 {
		c.DeFiMCP.Timeout = Duration(90 * time.Second)
	}
}

func (c *Config) Validate() error {
	if strings.TrimSpace(c.ListenAddr) == "" {
		return fmt.Errorf("listen_addr is required")
	}
	if strings.TrimSpace(c.DEXChain.ID) == "" {
		return fmt.Errorf("dex_chain.id is required")
	}
	if strings.TrimSpace(c.DEXChain.EVMRPCURL) == "" {
		return fmt.Errorf("dex_chain.evm_rpc_url is required")
	}
	return c.RequireDeFiMCP()
}

func (c *Config) RequireDeFiMCP() error {
	if strings.TrimSpace(c.DeFiMCP.URL) == "" {
		return fmt.Errorf("defi_mcp.url is required")
	}
	if strings.TrimSpace(c.DeFiMCP.AuthToken) == "" {
		return fmt.Errorf("defi_mcp.auth_token is required")
	}
	return nil
}
