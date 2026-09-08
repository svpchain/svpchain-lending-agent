#!/usr/bin/env bash
# Deploy the public Lending Agent. Lendora contracts and tools stay in the
# private DeFi MCP; this process only relays its token-authorized catalog and
# lands caller-signed EVM transactions.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/common.sh"

fail() { printf "  ${C_RED}x${C_RESET} %s\n" "$*" >&2; exit 1; }

readonly AGENT_NAME="svpchain-lending-agent"
readonly AGENT_PORT="8084"
readonly IMAGE_REPO="ghcr.io/svpchain/svpchain-lending-agent"

# Registration is local-only. The owner key lives in this agent's dedicated
# svpchaind keyring and is never rendered into runtime files or sent to dev03.

config_dir="${SVPCHAIN_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/${AGENT_NAME}}"
use_config=1
for ((i = 1; i <= $#; i++)); do
  case "${!i}" in
    --config-dir) j=$((i + 1)); config_dir="${!j:-}" ;;
    --no-config) use_config=0 ;;
  esac
done
unset i j

readonly CONFIG_VARS=(
  SVPCHAIN_DEPLOY_HOST SVPCHAIN_CHAIN_ID SVPCHAIN_EVM_RPC
  SVPCHAIN_AGENT_PUBLIC_URL SVPCHAIN_DEFI_MCP_URL
  SVPCHAIN_LENDING_DEFI_MCP_AUTH_TOKEN
  SVPCHAIN_LENDING_AGENT_LLM_PROVIDER SVPCHAIN_LENDING_AGENT_LLM_BASE_URL
  SVPCHAIN_LENDING_AGENT_LLM_MODEL SVPCHAIN_LENDING_AGENT_LLM_API_KEY
  SVPCHAIN_BIN SVPCHAIN_LENDING_AGENT_OWNER_KEY_NAME
  SVPCHAIN_LENDING_AGENT_KEYRING_HOME SVPCHAIN_LENDING_AGENT_KEYRING_BACKEND
  SVPCHAIN_LENDING_AGENT_ID
  SVPCHAIN_REGISTER_RPC SVPCHAIN_AGENT_CAPABILITIES
  SVPCHAIN_AGENT_PRICING_AMOUNT SVPCHAIN_AGENT_PRICING_UNIT
  SVPCHAIN_REGISTER_FEES
  SVPCHAIN_INSTALL_DIR
)

ENV_PRESET=" "
for v in "${CONFIG_VARS[@]}"; do
  [[ -n "${!v:-}" ]] && ENV_PRESET+="${v} "
done
unset v
was_preset() { [[ "$ENV_PRESET" == *" $1 "* ]]; }

if [[ "$use_config" == 1 && -f "${config_dir}/config.sh" ]]; then
  [[ -z "$(find "${config_dir}/config.sh" -perm -g+w -o -perm -o+w 2>/dev/null)" ]] || \
    fail "refusing to source a group- or world-writable config file: ${config_dir}/config.sh"
  preset=()
  for v in "${CONFIG_VARS[@]}"; do
    [[ -n "${!v:-}" ]] && preset+=("${v}=${!v}")
  done
  # shellcheck disable=SC1090
  source "${config_dir}/config.sh" || fail "config file failed to load: ${config_dir}/config.sh"
  for pair in "${preset[@]:-}"; do
    [[ -n "$pair" ]] && printf -v "${pair%%=*}" '%s' "${pair#*=}"
  done
  unset preset pair v
fi

mode=install
dry_run=0
skip_build=0
host="${SVPCHAIN_DEPLOY_HOST:-}"
chain_id="${SVPCHAIN_CHAIN_ID:-svp-2517-1}"
evm_rpc="${SVPCHAIN_EVM_RPC:-http://127.0.0.1:8545}"
public_url="${SVPCHAIN_AGENT_PUBLIC_URL:-https://agents.svpchain.org}"
defi_mcp_url="${SVPCHAIN_DEFI_MCP_URL:-http://127.0.0.1:8766/}"
defi_mcp_auth_token="${SVPCHAIN_LENDING_DEFI_MCP_AUTH_TOKEN:-}"
llm_provider="${SVPCHAIN_LENDING_AGENT_LLM_PROVIDER:-openai}"
llm_base_url="${SVPCHAIN_LENDING_AGENT_LLM_BASE_URL:-https://api.deepseek.com}"
llm_model="${SVPCHAIN_LENDING_AGENT_LLM_MODEL:-deepseek-v4-flash}"
install_dir="${SVPCHAIN_INSTALL_DIR:-~/svpchain-lending-agent}"
chain_bin="${SVPCHAIN_BIN:-svpchaind}"
owner_key_name="${SVPCHAIN_LENDING_AGENT_OWNER_KEY_NAME:-lending-agent-owner}"
keyring_home="${SVPCHAIN_LENDING_AGENT_KEYRING_HOME:-${config_dir}/keyring}"
keyring_backend="${SVPCHAIN_LENDING_AGENT_KEYRING_BACKEND:-file}"
registered_agent_id="${SVPCHAIN_LENDING_AGENT_ID:-}"
register_rpc="${SVPCHAIN_REGISTER_RPC:-}"
agent_capabilities="${SVPCHAIN_AGENT_CAPABILITIES:-evm.lending,lendora}"
pricing_amount="${SVPCHAIN_AGENT_PRICING_AMOUNT:-1000000}"
pricing_unit="${SVPCHAIN_AGENT_PRICING_UNIT:-call}"
register_fees="${SVPCHAIN_REGISTER_FEES:-}"
register_bond=""
image_tag=""
platform="linux/amd64"
FLAG_SET=" "
mark_flag() { FLAG_SET+="$1 "; }
was_flag() { [[ "$FLAG_SET" == *" $1 "* ]]; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host) host="$2"; mark_flag SVPCHAIN_DEPLOY_HOST; shift 2 ;;
    --chain-id) chain_id="$2"; mark_flag SVPCHAIN_CHAIN_ID; shift 2 ;;
    --evm-rpc) evm_rpc="$2"; mark_flag SVPCHAIN_EVM_RPC; shift 2 ;;
    --public-url) public_url="$2"; mark_flag SVPCHAIN_AGENT_PUBLIC_URL; shift 2 ;;
    --defi-mcp-url) defi_mcp_url="$2"; mark_flag SVPCHAIN_DEFI_MCP_URL; shift 2 ;;
    --llm-provider) llm_provider="$2"; mark_flag SVPCHAIN_LENDING_AGENT_LLM_PROVIDER; shift 2 ;;
    --llm-base-url) llm_base_url="$2"; mark_flag SVPCHAIN_LENDING_AGENT_LLM_BASE_URL; shift 2 ;;
    --llm-model) llm_model="$2"; mark_flag SVPCHAIN_LENDING_AGENT_LLM_MODEL; shift 2 ;;
    --owner-key-name) owner_key_name="$2"; mark_flag SVPCHAIN_LENDING_AGENT_OWNER_KEY_NAME; shift 2 ;;
    --keyring-home) keyring_home="$2"; mark_flag SVPCHAIN_LENDING_AGENT_KEYRING_HOME; shift 2 ;;
    --keyring-backend) keyring_backend="$2"; mark_flag SVPCHAIN_LENDING_AGENT_KEYRING_BACKEND; shift 2 ;;
    --agent-id) registered_agent_id="$2"; mark_flag SVPCHAIN_LENDING_AGENT_ID; shift 2 ;;
    --register-rpc) register_rpc="$2"; mark_flag SVPCHAIN_REGISTER_RPC; shift 2 ;;
    --capabilities) agent_capabilities="$2"; mark_flag SVPCHAIN_AGENT_CAPABILITIES; shift 2 ;;
    --pricing-amount) pricing_amount="$2"; mark_flag SVPCHAIN_AGENT_PRICING_AMOUNT; shift 2 ;;
    --pricing-unit) pricing_unit="$2"; mark_flag SVPCHAIN_AGENT_PRICING_UNIT; shift 2 ;;
    --fees) register_fees="$2"; mark_flag SVPCHAIN_REGISTER_FEES; shift 2 ;;
    --bond) register_bond="$2"; shift 2 ;;
    --install-dir) install_dir="$2"; mark_flag SVPCHAIN_INSTALL_DIR; shift 2 ;;
    --image-tag) image_tag="$2"; shift 2 ;;
    --platform) platform="$2"; shift 2 ;;
    --config-dir) mark_flag SVPCHAIN_CONFIG_DIR; shift 2 ;;
    --no-config) shift ;;
    --init-config) mode=init-config; shift ;;
    --gen-owner-key) mode=gen-owner-key; shift ;;
    --register) mode=register; shift ;;
    --print-env) mode=print-env; shift ;;
    --print-config) mode=print-config; shift ;;
    --print-compose) mode=print-compose; shift ;;
    --print-nginx) mode=print-nginx; shift ;;
    --dry-run) dry_run=1; shift ;;
    --skip-build) skip_build=1; shift ;;
    --uninstall) mode=uninstall; shift ;;
    -h|--help)
      sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed -n '/^#/p' | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) fail "unknown flag: $1" ;;
  esac
done

public_url="${public_url%/}"
defi_mcp_url="${defi_mcp_url%/}/"
agent_public_url="${public_url}"

render_agent_toml() {
  cat <<EOF
# Auto-generated by scripts/deploy.sh -- do not edit by hand.
listen_addr = "0.0.0.0:${AGENT_PORT}"
public_url = "${agent_public_url}"

[dex_chain]
id = "${chain_id}"
evm_rpc_url = "${evm_rpc}"

[defi_mcp]
url = "${defi_mcp_url}"
auth_token = "${defi_mcp_auth_token}"
timeout = "90s"

[llm]
provider = "${llm_provider}"
base_url = "${llm_base_url}"
model = "${llm_model}"
api_key_env = "SVPCHAIN_LENDING_AGENT_LLM_API_KEY"
EOF
}

render_compose_yaml() {
  cat <<EOF
# Auto-generated by scripts/deploy.sh -- do not edit by hand.
services:
  ${AGENT_NAME}:
    image: ${image_ref}
    container_name: ${AGENT_NAME}
    restart: unless-stopped
    command: ["-config", "/etc/${AGENT_NAME}/agent.toml"]
    network_mode: host
    environment:
      SVPCHAIN_LENDING_AGENT_LLM_API_KEY: "${SVPCHAIN_LENDING_AGENT_LLM_API_KEY:-}"
    volumes:
      - ${install_dir}/agent.toml:/etc/${AGENT_NAME}/agent.toml:ro
EOF
}

render_nginx_conf() {
  cat <<EOF
# ${AGENT_NAME} -- generated by scripts/deploy.sh --print-nginx
location / {
    proxy_pass http://127.0.0.1:${AGENT_PORT}/;
    proxy_set_header Host              \$host;
    proxy_set_header X-Real-IP         \$remote_addr;
    proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 300s;
}
EOF
}

print_env() {
  local names=(
    SVPCHAIN_CONFIG_DIR SVPCHAIN_DEPLOY_HOST SVPCHAIN_CHAIN_ID SVPCHAIN_EVM_RPC
    SVPCHAIN_AGENT_PUBLIC_URL SVPCHAIN_DEFI_MCP_URL SVPCHAIN_LENDING_DEFI_MCP_AUTH_TOKEN
    SVPCHAIN_LENDING_AGENT_LLM_PROVIDER SVPCHAIN_LENDING_AGENT_LLM_BASE_URL
    SVPCHAIN_LENDING_AGENT_LLM_MODEL SVPCHAIN_LENDING_AGENT_LLM_API_KEY
    SVPCHAIN_BIN SVPCHAIN_LENDING_AGENT_OWNER_KEY_NAME
    SVPCHAIN_LENDING_AGENT_KEYRING_HOME SVPCHAIN_LENDING_AGENT_KEYRING_BACKEND
    SVPCHAIN_LENDING_AGENT_ID
    SVPCHAIN_REGISTER_RPC SVPCHAIN_AGENT_CAPABILITIES
    SVPCHAIN_AGENT_PRICING_AMOUNT SVPCHAIN_AGENT_PRICING_UNIT SVPCHAIN_REGISTER_FEES
    SVPCHAIN_INSTALL_DIR
  )
  local values=(
    "$config_dir" "$host" "$chain_id" "$evm_rpc" "$public_url" "$defi_mcp_url" "$defi_mcp_auth_token"
    "$llm_provider" "$llm_base_url" "$llm_model" "${SVPCHAIN_LENDING_AGENT_LLM_API_KEY:-}"
    "$chain_bin" "$owner_key_name" "$keyring_home" "$keyring_backend" "$registered_agent_id"
    "$register_rpc" "$agent_capabilities" "$pricing_amount" "$pricing_unit" "$register_fees" "$install_dir"
  )
  for i in "${!names[@]}"; do
    local name="${names[$i]}" value="${values[$i]}" origin=default
    if was_flag "$name"; then origin=flag
    elif was_preset "$name"; then origin=environment
    elif [[ -n "${!name:-}" ]]; then origin="config file"
    fi
    if [[ "$name" == *AUTH_TOKEN || "$name" == *API_KEY ]]; then
      [[ -n "$value" ]] && value="set (${#value} chars)" || value=unset
    elif [[ -z "$value" ]]; then value="(empty)"
    fi
    printf '%-42s %-14s %s\n' "$name" "$origin" "$value"
  done
}

resolve_keyring_home() {
  case "$keyring_home" in
    "~") keyring_home="$HOME" ;;
    "~/"*) keyring_home="$HOME/${keyring_home#\~/}" ;;
  esac
}

require_chain_bin() {
  if [[ "$chain_bin" == */* ]]; then
    [[ -x "$chain_bin" ]] || fail "SVPCHAIN_BIN is not executable: ${chain_bin}"
  else
    command -v "$chain_bin" >/dev/null 2>&1 || fail "svpchaind was not found; set SVPCHAIN_BIN to its absolute path"
    chain_bin="$(command -v "$chain_bin")"
  fi
}

require_indexed_agent_cli() {
  "$chain_bin" query agent next-agent-index --help >/dev/null 2>&1 || fail \
    "${chain_bin} is too old for indexed Agent IDs; rebuild svpchaind from the current protocol source before registering"
}

owner_address() {
  "$chain_bin" keys show "$owner_key_name" -a \
    --home "$keyring_home" --keyring-backend "$keyring_backend"
}

resolve_min_bond() {
  require_cmd jq
  local params amount denom
  params="$("$chain_bin" query agent params --node "$register_rpc" --output json)" || fail "could not query agent module params from ${register_rpc}"
  amount="$(jq -r '.params.min_bond.amount // empty' <<<"$params")"
  denom="$(jq -r '.params.min_bond.denom // empty' <<<"$params")"
  [[ -n "$amount" && -n "$denom" ]] || fail "agent module params did not contain params.min_bond"
  printf '%s%s' "$amount" "$denom"
}

register_agent() {
  [[ -n "$register_rpc" ]] || fail "SVPCHAIN_REGISTER_RPC is required for --register"
  [[ "$public_url" == https://* || "$public_url" == http://* ]] || fail "SVPCHAIN_AGENT_PUBLIC_URL must be an http(s) URL"
  [[ -n "$agent_capabilities" ]] || fail "SVPCHAIN_AGENT_CAPABILITIES is required for --register"
  [[ -n "$pricing_amount" && -n "$pricing_unit" ]] || fail "pricing amount and unit are required for --register"
  [[ -n "$register_fees" ]] || fail "SVPCHAIN_REGISTER_FEES is required for --register"
  require_chain_bin
  require_indexed_agent_cli
  require_cmd curl
  require_cmd jq
  resolve_keyring_home
  [[ -d "$keyring_home" ]] || fail "owner keyring does not exist: ${keyring_home}; run --gen-owner-key first"

  curl -fsS "${public_url}/.well-known/agent-card.json" >/dev/null || \
    fail "public Agent Card is unavailable: ${public_url}/.well-known/agent-card.json"

  local owner agent_id agents_json agent_count bond action
  owner="$(owner_address)" || fail "owner key ${owner_key_name} was not found in ${keyring_home}; run --gen-owner-key first"
  agents_json="$("$chain_bin" query agent agents-by-owner "$owner" --node "$register_rpc" --output json)" || \
    fail "could not query existing agents for owner ${owner}"
  agent_count="$(jq -r '.agents | length' <<<"$agents_json")"
  [[ "$agent_count" =~ ^[0-9]+$ ]] || fail "agents-by-owner returned an invalid response"
  if [[ "$agent_count" == 0 ]]; then
    action=register
    agent_id="did:svp:${owner}:<allocated-by-chain>"
  elif [[ -n "$registered_agent_id" ]]; then
    jq -e --arg id "$registered_agent_id" '.agents[] | select(.agent_id == $id)' >/dev/null <<<"$agents_json" || \
      fail "SVPCHAIN_LENDING_AGENT_ID is not controlled by ${owner}: ${registered_agent_id}"
    action=update
    agent_id="$registered_agent_id"
  elif [[ "$agent_count" == 1 ]]; then
    action=update
    agent_id="$(jq -r '.agents[0].agent_id // empty' <<<"$agents_json")"
    [[ -n "$agent_id" ]] || fail "agents-by-owner response did not contain agent_id"
  else
    fail "owner ${owner} controls ${agent_count} agents; set SVPCHAIN_LENDING_AGENT_ID to the Lending Agent DID"
  fi

  bond="$register_bond"
  if [[ "$action" == register && -z "$bond" ]]; then bond="$(resolve_min_bond)"; fi
  if [[ "$dry_run" == 1 ]]; then
    step "[dry-run] ${action} ${agent_id} at ${public_url}"
    printf '  capabilities: %s\n  price: %s %s\n' "$agent_capabilities" "$pricing_amount" "$pricing_unit"
    [[ "$action" == register ]] && printf '  initial bond: %s\n' "$bond"
    exit 0
  fi

  local tx_args=(--endpoint "$public_url" --capabilities "$agent_capabilities"
    --price-amount "$pricing_amount" --price-unit "$pricing_unit"
    --from "$owner_key_name" --home "$keyring_home" --keyring-backend "$keyring_backend"
    --chain-id "$chain_id" --node "$register_rpc" --gas auto --gas-adjustment 1.5
    --fees "$register_fees" -y)
  if [[ "$action" == register ]]; then
    "$chain_bin" tx agent register-agent "$bond" "${tx_args[@]}"
  else
    "$chain_bin" tx agent update-agent "$agent_id" "${tx_args[@]}"
  fi
  step "Submitted ${action} for ${agent_id}; svpchaind derives the capability hash from the live public Agent Card"
}

if [[ "$mode" == init-config ]]; then
  dst="${config_dir}/config.sh"
  [[ ! -e "$dst" ]] || fail "refusing to overwrite ${dst}"
  mkdir -p "$config_dir"
  install -m 600 "${SCRIPT_DIR}/config.sh.example" "$dst"
  step "Wrote ${dst} (mode 600)"
  exit 0
fi
if [[ "$mode" == print-env ]]; then print_env; exit 0; fi
if [[ "$mode" == print-config ]]; then render_agent_toml; exit 0; fi
if [[ "$mode" == print-compose ]]; then image_ref="${IMAGE_REPO}:${image_tag:-<tag>}"; render_compose_yaml; exit 0; fi
if [[ "$mode" == print-nginx ]]; then render_nginx_conf; exit 0; fi
if [[ "$mode" == gen-owner-key ]]; then
  require_chain_bin
  resolve_keyring_home
  if [[ "$dry_run" == 1 ]]; then
    step "[dry-run] would create ${owner_key_name} in ${keyring_home} using the ${keyring_backend} keyring"
    exit 0
  fi
  mkdir -p "$keyring_home"
  chmod 700 "$keyring_home"
  if "$chain_bin" keys show "$owner_key_name" --home "$keyring_home" --keyring-backend "$keyring_backend" >/dev/null 2>&1; then
    fail "owner key ${owner_key_name} already exists in ${keyring_home}; it is this agent's on-chain identity"
  fi
  "$chain_bin" keys add "$owner_key_name" --home "$keyring_home" \
    --keyring-backend "$keyring_backend" --key-type eth_secp256k1 --no-backup --output json >/dev/null
  owner="$(owner_address)" || fail "created owner key could not be read"
  step "Created Lending Agent owner key. Fund ${owner} with at least the minimum bond, registration fee, and gas."
  exit 0
fi
if [[ "$mode" == register ]]; then register_agent; exit 0; fi

[[ -n "$host" ]] || fail "--host is required (or set SVPCHAIN_DEPLOY_HOST)"
if [[ "$mode" == install ]]; then
  [[ -n "$defi_mcp_auth_token" ]] || fail "SVPCHAIN_LENDING_DEFI_MCP_AUTH_TOKEN is required"
  [[ -n "$evm_rpc" ]] || fail "SVPCHAIN_EVM_RPC is required"
fi

resolve_install_dir() {
  case "$install_dir" in
    "~"|"~/"*)
      [[ "$dry_run" == 1 ]] && return
      local remote_home
      remote_home="$(ssh -o BatchMode=yes "$host" 'printf %s "$HOME"')" || fail "could not resolve remote HOME"
      install_dir="${remote_home}${install_dir#\~}"
      ;;
  esac
}

if [[ "$mode" == uninstall ]]; then
  resolve_install_dir
  ssh -o BatchMode=yes "$host" "docker compose -f '$install_dir/docker-compose.yml' down 2>/dev/null || true; docker rm -f '$AGENT_NAME' 2>/dev/null || true; rm -rf '$install_dir'"
  step "Removed ${AGENT_NAME} from ${host}"
  exit 0
fi

require_cmd docker
require_cmd rsync
require_cmd ssh
require_cmd go
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_DIR"
if [[ -z "$image_tag" ]]; then image_tag="$(git rev-parse --short HEAD 2>/dev/null || echo dev)"; fi
image_ref="${IMAGE_REPO}:${image_tag}"
resolve_install_dir

if [[ "$skip_build" != 1 ]]; then
  go mod vendor
  docker build --platform "$platform" -t "$image_ref" -f "cmd/${AGENT_NAME}/Dockerfile" .
fi

image_tar="${REPO_DIR}/build/${AGENT_NAME}.image.tar"
mkdir -p "${REPO_DIR}/build"
docker save -o "$image_tar" "$image_ref"
stage_dir="$(mktemp -d -t "${AGENT_NAME}.stage.XXXXXX")"
trap 'rm -rf "$stage_dir"' EXIT
render_agent_toml > "$stage_dir/agent.toml"
render_compose_yaml > "$stage_dir/docker-compose.yml"
chmod 600 "$stage_dir"/*

if [[ "$dry_run" == 1 ]]; then
  info "[dry-run] would install $image_ref on $host at $install_dir"
  exit 0
fi
ssh -o BatchMode=yes "$host" "docker version --format '{{.Server.Version}}' >/dev/null && docker compose version >/dev/null"
rsync -avz "$stage_dir/" "$host:$install_dir/"
rsync -avz "$image_tar" "$host:$install_dir/${AGENT_NAME}.image.tar"
# docker load can replace the image behind an unchanged tag. Compose's default
# `up -d` then keeps the existing container, so code updates silently do not
# take effect. Always recreate this single-agent container after loading.
ssh -o BatchMode=yes "$host" "docker load < '$install_dir/${AGENT_NAME}.image.tar' && docker compose -f '$install_dir/docker-compose.yml' up -d --force-recreate"

for _ in 1 2 3 4 5 6 7 8 9 10; do
  code="$(ssh -o BatchMode=yes "$host" "curl -sS -o /dev/null -w '%{http_code}' --max-time 5 http://127.0.0.1:${AGENT_PORT}/healthz" 2>/dev/null || true)"
  [[ "$code" == 200 ]] && break
  sleep 2
done
[[ "${code:-}" == 200 ]] || fail "health check failed; inspect: ssh $host 'docker logs $AGENT_NAME --tail=80'"
step "${AGENT_NAME} is healthy at ${agent_public_url}"
