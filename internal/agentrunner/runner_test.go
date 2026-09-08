package agentrunner

import "testing"

func TestReadOnlyAllowsOnlyLendoraReads(t *testing.T) {
	for _, name := range []string{
		"lendora_get_all_markets",
		"lendora_get_protocol_dashboard",
		"lendora_assess_risk",
	} {
		if !readOnly(name) {
			t.Errorf("%s should be read-only", name)
		}
	}
	for _, name := range []string{
		"lendora_build_supply_tx",
		"broadcast_evm_tx",
		"get_balance",
		"quote_swap",
	} {
		if readOnly(name) {
			t.Errorf("%s must not be available to the intent runner", name)
		}
	}
}
