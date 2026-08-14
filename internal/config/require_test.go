package config

import (
	"strings"
	"testing"
)

// This binary hard-requires the family it exists to serve: main.go calls
// RequireLendora before wiring, so a missing endpoint or comptroller fails at
// boot with a named key rather than turning every Lendora tool into a
// call-time refusal.
//
// The RequireEVM half of this file went with the EVM DeFi surface.
func TestRequireLendoraNeedsEndpointAndComptroller(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.RequireLendora(); err == nil || !strings.Contains(err.Error(), "evm_rpc_url") {
		t.Errorf("expected an error naming evm_rpc_url, got %v", err)
	}

	withEVM := minimal + `dex_chain.evm_rpc_url = "http://127.0.0.1:8545"` + "\n"
	cfg, err = Load(writeConfig(t, withEVM))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.RequireLendora(); err == nil || !strings.Contains(err.Error(), "comptroller_addr") {
		t.Errorf("expected an error naming comptroller_addr, got %v", err)
	}

	full := withEVM + `evm.lendora.comptroller_addr = "0x00000000000000000000000000000000000000aa"` + "\n"
	cfg, err = Load(writeConfig(t, full))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.RequireLendora(); err != nil {
		t.Errorf("expected the Lendora requirement satisfied, got %v", err)
	}
}
