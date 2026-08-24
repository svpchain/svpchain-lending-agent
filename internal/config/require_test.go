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

	withoutMethods := withEVM + `evm.lendora.comptroller_addr = "0x00000000000000000000000000000000000000aa"` + "\n"
	cfg, err = Load(writeConfig(t, withoutMethods))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.RequireLendora(); err == nil || !strings.Contains(err.Error(), "evm.lendora.methods") {
		t.Errorf("expected an error naming evm.lendora.methods, got %v", err)
	}

	full := withEVM + `evm.lendora.comptroller_addr = "0x00000000000000000000000000000000000000aa"` + "\n" +
		`evm.lendora.methods = ["mint(uint256)"]` + "\n"
	cfg, err = Load(writeConfig(t, full))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.RequireLendora(); err != nil {
		t.Errorf("expected the Lendora requirement satisfied, got %v", err)
	}
}

func TestLendoraMethodsMustBeValidAndUnique(t *testing.T) {
	base := minimal + `
dex_chain.evm_rpc_url = "http://127.0.0.1:8545"
evm.lendora.comptroller_addr = "0x00000000000000000000000000000000000000aa"
`
	for _, methods := range []string{
		`["mint(uint256)", "mint(uint256)"]`,
		`["mint (uint256)"]`,
	} {
		if _, err := Load(writeConfig(t, base+"evm.lendora.methods = "+methods+"\n")); err == nil {
			t.Fatalf("methods %s should be rejected", methods)
		}
	}
	if cfg, err := Load(writeConfig(t, base+`evm.lendora.methods = ["mint(uint256)"]`+"\n")); err != nil || len(cfg.EVM.Lendora.Methods) != 1 {
		t.Fatalf("valid methods should load, cfg=%+v err=%v", cfg, err)
	}
}

func TestLendoraComptrollerAddressNormalizesToLowercase(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimal+`
dex_chain.evm_rpc_url = "http://127.0.0.1:8545"
evm.lendora.comptroller_addr = "0xd925E663fdEA0aB911c25717F8ab9D7042397228"
evm.lendora.methods = ["mint(uint256)"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.EVM.Lendora.ComptrollerAddr, "0xd925e663fdea0ab911c25717f8ab9d7042397228"; got != want {
		t.Errorf("comptroller_addr = %q, want normalized %q", got, want)
	}
}
