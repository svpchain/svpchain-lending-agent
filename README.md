# SVP-Chain Lending Agent

`svpchain-lending-agent` is the public A2A relay for Lendora. It does not
contain Lendora contracts, asset configuration, or a built-in MCP server.
Those belong to the private `svpchain-defi-mcp` service.

At startup the agent authenticates to DeFi MCP, calls `tools/list`, and
publishes only the `lendora_*` tools authorized for its relay token. Its own
public tools are limited to:

- `broadcast_evm_tx`
- `evm_tx_status`
- `svpchain-meta/list_tools`

`lendora_build_*` calls return unsigned EVM payloads. The caller signs those
locally and sends the signed raw transaction through `broadcast_evm_tx`.

## Configuration

The agent needs only an EVM JSON-RPC endpoint and private DeFi MCP access:

```toml
listen_addr = ":8084"
public_url = "https://lending-agent.example.com"

[dex_chain]
id = "svp-2517-1"
evm_rpc_url = "http://127.0.0.1:8545"

[defi_mcp]
url = "http://127.0.0.1:8766/"
auth_token = "lending-agent-private-secret"
timeout = "90s"

[llm]
provider = "openai"
base_url = "https://api.deepseek.com"
model = "deepseek-v4-flash"
api_key_env = "SVPCHAIN_LENDING_AGENT_LLM_API_KEY"
```

The LLM is optional. With its API key present, plain text and
`{"intent":"..."}` requests may use read-only `lendora_get_*` and
`lendora_assess_risk` tools. Structured `lendora_build_*` requests always
pass directly to DeFi MCP and never invoke the LLM.

DeFi MCP owns the allowed tool list, assets, cToken/Comptroller addresses,
and token authorization. Add a dedicated `[[trusted_agent]]` entry there for
this relay. Do not reuse another public agent's token.

## Local Run

Start `svpchain-defi-mcp` first, then set the optional LLM environment
variable and run:

```sh
SVPCHAIN_LENDING_AGENT_LLM_API_KEY="..." \
go run ./cmd/svpchain-lending-agent -config cmd/svpchain-lending-agent/agent.toml.example
```

The health endpoint is `/healthz`; the Agent Card is
`/.well-known/agent-card.json`.

## Deploy

Create the deploy config once:

```sh
./scripts/deploy.sh --config-dir ~/.config/svpchain-lending-agent-dev03 --init-config
```

Set the values shown in `scripts/config.sh.example`, especially
`SVPCHAIN_EVM_RPC`, `SVPCHAIN_DEFI_MCP_URL`, and
`SVPCHAIN_LENDING_DEFI_MCP_AUTH_TOKEN`. Inspect the rendered config without
deploying:

```sh
./scripts/deploy.sh --config-dir ~/.config/svpchain-lending-agent-dev03 --print-config
./scripts/deploy.sh --config-dir ~/.config/svpchain-lending-agent-dev03 --print-nginx
```

Deploy with:

```sh
./scripts/deploy.sh --config-dir ~/.config/svpchain-lending-agent-dev03
```

The script prints, but never installs, the Nginx location block. Add it to the
server for the configured Lending Agent host so its root path reaches
`127.0.0.1:8084`.
Any Agent Card change requires the normal on-chain agent update so its
capability hash matches the live card.

## On-Chain Registration

Lending has an independent on-chain identity. Its DID is allocated by the chain
as `did:svp:<owner>:<index>`. Its owner key is stored only in
the local dedicated keyring from `SVPCHAIN_LENDING_AGENT_KEYRING_HOME`; it is
not sent to the deployment host and must not be shared with EVM Agent.

After the service and public Agent Card are live, create the identity once:

```sh
./scripts/deploy.sh --config-dir ~/.config/svpchain-lending-agent-dev03 --gen-owner-key
```

Fund the printed `svp1...` address with the x/agent minimum bond, registration
fee, and gas. Then preview and submit the registration:

```sh
./scripts/deploy.sh --config-dir ~/.config/svpchain-lending-agent-dev03 --register --dry-run
./scripts/deploy.sh --config-dir ~/.config/svpchain-lending-agent-dev03 --register
```

The same `--register` command is idempotent: it updates an existing Lending
Agent record. It discovers the single Agent owned by the dedicated key; if the
owner controls multiple Agents, set `SVPCHAIN_LENDING_AGENT_ID` to Lending's
indexed DID. It lets `svpchaind` fetch the live public Agent Card and compute
the capability hash, so run it again whenever that card changes. `svpchaind`
must support `query agent next-agent-index`; an older binary constructs the
legacy unindexed DID and will be rejected by the current chain.

## Verification

```sh
go test ./cmd/svpchain-lending-agent ./internal/a2aserver ./internal/toolbridge \
  ./internal/config ./internal/defimcp ./internal/wire ./internal/agenttools \
  ./internal/agentrunner ./internal/llm
```
