module github.com/svpchain/svpchain-lending-agent

go 1.25.0

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c
	github.com/a2aproject/a2a-go/v2 v2.3.1
	github.com/ethereum/go-ethereum v1.15.11
	github.com/google/jsonschema-go v0.3.0
	github.com/modelcontextprotocol/go-sdk v1.0.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/bits-and-blooms/bitset v1.24.3 // indirect
	github.com/cockroachdb/errors v1.12.0 // indirect
	github.com/cockroachdb/fifo v0.0.0-20240616162244-4768e80dfb9a // indirect
	github.com/cockroachdb/logtags v0.0.0-20241215232642-bb51bb14a506 // indirect
	github.com/cockroachdb/redact v1.1.6 // indirect
	github.com/consensys/gnark-crypto v0.18.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/crate-crypto/go-eth-kzg v1.3.0 // indirect
	github.com/crate-crypto/go-ipa v0.0.0-20240724233137-53bbb0ceb27a // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/deckarep/golang-set/v2 v2.6.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/ethereum/c-kzg-4844/v2 v2.1.0 // indirect
	github.com/ethereum/go-verkle v0.2.2 // indirect
	github.com/getsentry/sentry-go v0.35.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/golang/snappy v0.0.5-0.20231225225746-43d5d4cd4e0e // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.23.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/supranational/blst v0.3.14 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.41.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// The replace blocks below are copied VERBATIM from protocol/go.mod, which is
// their single source of truth (deps_test.go diffs them against it on every
// `go test ./...`). Go does not apply a dependency module's
// own replace directives, so any module depending on the protocol module (a
// Cosmos chain, with its forks of cosmos-sdk, cometbft, cosmos/evm, iavl, etc.)
// MUST redeclare them or the build resolves the upstream modules and fails.
// Keep in sync with protocol/go.mod.
