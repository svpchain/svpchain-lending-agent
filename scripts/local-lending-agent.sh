#!/usr/bin/env bash
# Run svpchain-lending-agent against protocol/scripts/local_node_agents.sh.
#
# Usage:
#   cp scripts/lendora.toml.example lendora.toml
#   $EDITOR lendora.toml
#   ./scripts/local-lending-agent.sh start
#   ./scripts/local-lending-agent.sh register [--bond 5000asvp]
#   ./scripts/local-lending-agent.sh update
#   ./scripts/local-lending-agent.sh stop|status|logs|config
#
# A Comptroller address is mandatory: the local chain fixture starts the chain
# but does not deploy Lendora. cToken market addresses are discovered from that
# Comptroller and are supplied per Lendora operation, not in this launcher.
#
# Environment equivalents: LENDING_AGENT_LOCAL_CHAIN_ID,
# LENDING_AGENT_LOCAL_GRPC, LENDING_AGENT_LOCAL_REST,
# LENDING_AGENT_LOCAL_COMET_RPC, LENDING_AGENT_LOCAL_EVM_RPC,
# LENDING_AGENT_LOCAL_INDEXER, LENDING_AGENT_LOCAL_LISTEN,
# LENDING_AGENT_LOCAL_PUBLIC_URL, LENDING_AGENT_LOCAL_LENDORA_FILE,
# LENDING_AGENT_LOCAL_OPERATOR_KEY_FILE, LENDING_AGENT_LOCAL_PROTOCOL_DIR,
# LENDING_AGENT_LOCAL_CHAIN_SCRIPT, LENDING_AGENT_LOCAL_CHAIN_HOME,
# LENDING_AGENT_LOCAL_CHAIN_BINARY, LENDING_AGENT_LOCAL_FUNDER_KEY, and
# LENDING_AGENT_LOCAL_FUND_AMOUNT, and LENDING_AGENT_LOCAL_LENDORA_FILE.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

fail() { printf 'local-lending-agent: %s\n' "$*" >&2; exit 1; }
info() { printf 'local-lending-agent: %s\n' "$*"; }

mode="start"
chain_id="${LENDING_AGENT_LOCAL_CHAIN_ID:-svp-2517-1}"
grpc_addr="${LENDING_AGENT_LOCAL_GRPC:-127.0.0.1:9090}"
rest_url="${LENDING_AGENT_LOCAL_REST:-http://127.0.0.1:1317}"
comet_rpc="${LENDING_AGENT_LOCAL_COMET_RPC:-http://127.0.0.1:26657}"
evm_rpc="${LENDING_AGENT_LOCAL_EVM_RPC:-http://127.0.0.1:8545}"
indexer="${LENDING_AGENT_LOCAL_INDEXER:-http://127.0.0.1:3002}"
listen_addr="${LENDING_AGENT_LOCAL_LISTEN:-127.0.0.1:8084}"
public_url="${LENDING_AGENT_LOCAL_PUBLIC_URL:-http://localhost:8084}"
lendora_file="${LENDING_AGENT_LOCAL_LENDORA_FILE:-${REPO_DIR}/lendora.toml}"
operator_key_file="${LENDING_AGENT_LOCAL_OPERATOR_KEY_FILE:-}"
protocol_dir="${LENDING_AGENT_LOCAL_PROTOCOL_DIR:-}"
local_chain_script="${LENDING_AGENT_LOCAL_CHAIN_SCRIPT:-}"
local_chain_home="${LENDING_AGENT_LOCAL_CHAIN_HOME:-${DYDX_HOME:-$HOME/.svpchain-agents}}"
local_chain_binary="${LENDING_AGENT_LOCAL_CHAIN_BINARY:-}"
local_chain_funder_key="${LENDING_AGENT_LOCAL_FUNDER_KEY:-localval}"
fund_amount="${LENDING_AGENT_LOCAL_FUND_AMOUNT:-20000000000000000000asvp}"
registration_bond=""
skip_build=0
listen_overridden=0

usage() {
  sed -n '2,/^set -euo pipefail/p' "${BASH_SOURCE[0]}" | sed -n '/^#/p' | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    start|stop|status|logs|config|register|update) mode="$1"; shift ;;
    --bond) registration_bond="${2:-}"; shift 2 ;;
    --chain-id) chain_id="${2:-}"; shift 2 ;;
    --grpc-addr) grpc_addr="${2:-}"; shift 2 ;;
    --rest-url) rest_url="${2:-}"; shift 2 ;;
    --comet-rpc) comet_rpc="${2:-}"; shift 2 ;;
    --evm-rpc) evm_rpc="${2:-}"; shift 2 ;;
    --indexer) indexer="${2:-}"; shift 2 ;;
    --listen) listen_addr="${2:-}"; listen_overridden=1; shift 2 ;;
    --public-url) public_url="${2:-}"; shift 2 ;;
    --lendora-file) lendora_file="${2:-}"; shift 2 ;;
    --operator-key-file) operator_key_file="${2:-}"; shift 2 ;;
    --skip-build) skip_build=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "${mode}" == register || -z "${registration_bond}" ]] || fail "--bond is only valid with register"
public_url="${public_url%/}"
state_dir="${REPO_DIR}/build/local-lending-agent"
config_path="${state_dir}/agent.toml"
binary_path="${state_dir}/svpchain-lending-agent"
localctl_path="${state_dir}/svpchain-lending-agent-localctl"
pid_path="${state_dir}/agent.pid"
log_path="${state_dir}/agent.log"

absolute_file_path() {
  [[ "$1" == /* ]] && { printf '%s' "$1"; return; }
  cd "$(dirname "$1")" && printf '%s/%s' "$PWD" "$(basename "$1")"
}

toml_string_value() {
  sed -nE "s/^[[:space:]]*$1[[:space:]]*=[[:space:]]*\"([^\"]*)\"[[:space:]]*$/\1/p" "$2" | head -n 1
}

port_from_listen() {
  [[ "${listen_addr}" == *:* ]] || fail "--listen must be host:port, got ${listen_addr}"
  printf '%s' "${listen_addr##*:}"
}

pid_alive() {
  [[ -s "${pid_path}" ]] || return 1
  local pid
  pid="$(<"${pid_path}")"
  agent_process "${pid}"
}

agent_process() {
  local pid="$1" command
  [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null || return 1
  command="$(ps -p "${pid}" -o command= 2>/dev/null || true)"
  [[ "${command}" == *"${binary_path}"* && "${command}" == *"-config ${config_path}"* ]]
}

listening_agent_pid() {
  command -v lsof >/dev/null 2>&1 || return 1
  local port pid
  port="$(port_from_listen)"
  for pid in $(lsof -t -nP -iTCP@127.0.0.1:"${port}" -sTCP:LISTEN 2>/dev/null || true); do
    if agent_process "${pid}"; then
      printf '%s' "${pid}"
      return 0
    fi
  done
  return 1
}

stop_pid() {
  local pid="$1"
  kill "${pid}" 2>/dev/null || true
  for _ in {1..20}; do kill -0 "${pid}" 2>/dev/null || return; sleep 1; done
  fail "pid ${pid} did not stop"
}

require_free_listen_port() {
  command -v lsof >/dev/null 2>&1 || return 0
  local pids
  pids="$(lsof -t -nP -iTCP@127.0.0.1:"$1" -sTCP:LISTEN 2>/dev/null || true)"
  [[ -z "${pids}" ]] || fail "127.0.0.1:$1 is already in use by pid(s) ${pids//$'\n'/, }"
}

resolve_protocol_dir() {
  if [[ -n "${protocol_dir}" ]]; then
    [[ -f "${protocol_dir}/go.mod" ]] || fail "LENDING_AGENT_LOCAL_PROTOCOL_DIR is not a protocol module: ${protocol_dir}"
    printf '%s' "${protocol_dir}"
    return
  fi
  local candidate
  for candidate in "${REPO_DIR}/../svpagent/protocol" "${REPO_DIR}/../../svpchain/protocol"; do
    [[ -f "${candidate}/go.mod" ]] && { printf '%s' "${candidate}"; return; }
  done
  fail "protocol checkout not found; set LENDING_AGENT_LOCAL_PROTOCOL_DIR"
}

resolve_local_chain_script() {
  if [[ -n "${local_chain_script}" ]]; then
    [[ -f "${local_chain_script}" ]] || fail "LENDING_AGENT_LOCAL_CHAIN_SCRIPT does not exist: ${local_chain_script}"
    printf '%s' "${local_chain_script}"
    return
  fi
  printf '%s/scripts/local_node_agents.sh' "$(resolve_protocol_dir)"
}

resolve_chain_binary() {
  if [[ -n "${local_chain_binary}" ]]; then
    [[ -x "${local_chain_binary}" ]] || fail "LENDING_AGENT_LOCAL_CHAIN_BINARY is not executable: ${local_chain_binary}"
    printf '%s' "${local_chain_binary}"
    return
  fi
  command -v svpchaind >/dev/null 2>&1 && { command -v svpchaind; return; }
  local candidate="$(go env GOPATH)/bin/svpchaind"
  [[ -x "${candidate}" ]] || fail "svpchaind is not installed; set LENDING_AGENT_LOCAL_CHAIN_BINARY"
  printf '%s' "${candidate}"
}

check_local_chain() {
  curl -fsS --max-time 3 "${comet_rpc}/status" >/dev/null || fail "local chain Comet RPC is unavailable at ${comet_rpc}"
  curl -fsS --max-time 3 "${rest_url}/cosmos/base/tendermint/v1beta1/node_info" >/dev/null || fail "local chain REST API is unavailable at ${rest_url}"
  if command -v nc >/dev/null 2>&1; then
    nc -z -w 3 "${grpc_addr%:*}" "${grpc_addr##*:}" >/dev/null 2>&1 || fail "local chain gRPC is unavailable at ${grpc_addr}"
  fi
}

render_config() {
  cat <<EOF
# Generated by scripts/local-lending-agent.sh. Do not edit by hand.
listen_addr = "${listen_addr}"
public_url = "${public_url}"

[dex_chain]
id               = "${chain_id}"
grpc_addr        = "${grpc_addr}"
comet_rpc_url    = "${comet_rpc}"
indexer_base_url = "${indexer}"
evm_rpc_url      = "${evm_rpc}"

EOF
  cat <<EOF
[fee]
dynamic        = true
gas_price      = "25000000000"
gas_adjustment = 1.25
max_gas_limit  = 2000000

EOF
  [[ -f "${lendora_file}" ]] || fail "Lendora config file not found: ${lendora_file}; copy scripts/lendora.toml.example to lendora.toml and edit it"
  cat <<EOF

# Lendora config included from ${lendora_file}.
EOF
  cat "${lendora_file}"
  if [[ -n "${operator_key_file}" ]]; then
    cat <<EOF

[operator]
key_file     = "${operator_key_file}"
capabilities = ["evm.contract_call"]
metadata     = "local development Lendora agent"
EOF
  fi
}

build_binaries() {
  GOWORK=off go build -o "${binary_path}" ./cmd/svpchain-lending-agent
  GOWORK=off go build -o "${localctl_path}" ./cmd/svpchain-lending-agent-localctl
}

fund_operator() {
  local address="$1" chain_binary result
  chain_binary="$(resolve_chain_binary)"
  info "funding operator ${address} with ${fund_amount}"
  result="$("${chain_binary}" --home "${local_chain_home}" tx bank send "${local_chain_funder_key}" "${address}" "${fund_amount}" --from "${local_chain_funder_key}" --keyring-backend test --chain-id "${chain_id}" --node "tcp://${comet_rpc#http://}" --gas auto --gas-adjustment 1.5 --fees 500000asvp --broadcast-mode sync -y -o json)" || fail "fund operator: ${result:-transaction failed}"
  info "fund transaction accepted: $(jq -r '.txhash // "unknown"' <<<"${result}")"
}

case "${mode}" in
  config) render_config; exit 0 ;;
  status)
    if pid_alive; then info "running (pid $(<"${pid_path}"), ${public_url})"; exit 0; fi
    if pid="$(listening_agent_pid)"; then
      printf '%s\n' "${pid}" > "${pid_path}"
      info "running (pid ${pid}, ${public_url})"; exit 0
    fi
    rm -f "${pid_path}"; info "stopped"; exit 1 ;;
  logs) [[ -f "${log_path}" ]] || fail "no log file at ${log_path}"; tail -n 120 -f "${log_path}" ;;
  stop)
    if pid_alive; then
      stop_pid "$(<"${pid_path}")"
    elif pid="$(listening_agent_pid)"; then
      info "recovering untracked agent pid ${pid}"
      stop_pid "${pid}"
    fi
    rm -f "${pid_path}"; info "stopped"; exit 0 ;;
  register|update)
    if [[ -z "${operator_key_file}" && -f "${config_path}" ]]; then
      operator_key_file="$(toml_string_value key_file "${config_path}")"
    fi
    if [[ "${listen_overridden}" == 0 && -f "${config_path}" ]]; then
      configured_listen="$(toml_string_value listen_addr "${config_path}")"
      [[ -z "${configured_listen}" ]] || listen_addr="${configured_listen}"
    fi
    [[ -n "${operator_key_file}" && -f "${operator_key_file}" ]] || fail "${mode} requires --operator-key-file"
    [[ -x "${localctl_path}" ]] || fail "local control binary is missing; rerun start without --skip-build"
    agent_url="http://127.0.0.1:$(port_from_listen)"
    curl -fsS --max-time 3 "${agent_url}/healthz" >/dev/null || fail "agent is unavailable at ${agent_url}; start it with --operator-key-file first"
    args=(--action "${mode}" --agent-url "${agent_url}" --key-file "$(absolute_file_path "${operator_key_file}")")
    [[ -z "${registration_bond}" ]] || args+=(--bond "${registration_bond}")
    exec "${localctl_path}" "${args[@]}" ;;
esac

pid_alive && fail "already running (pid $(<"${pid_path}")); use stop first"
port="$(port_from_listen)"
require_free_listen_port "${port}"
script="$(resolve_local_chain_script)"
[[ -f "${script}" ]] || fail "local agent-chain launcher not found: ${script}"
info "ensuring local chain is running via ${script}"
DYDX_HOME="${local_chain_home}" CHAIN_ID="${chain_id}" bash "${script}" start
check_local_chain
mkdir -p "${state_dir}"

if [[ -n "${operator_key_file}" ]]; then
  [[ -f "${operator_key_file}" ]] || fail "operator key file not found: ${operator_key_file}"
  operator_key_file="$(absolute_file_path "${operator_key_file}")"
  grep -Eq '^(0x)?[0-9a-fA-F]{64}[[:space:]]*$' "${operator_key_file}" || fail "operator key file must contain one 32-byte hex key"
fi
render_config > "${config_path}"
if [[ "${skip_build}" == 0 ]]; then
  info "building local binaries"
  build_binaries
else
  [[ -x "${binary_path}" && -x "${localctl_path}" ]] || fail "--skip-build requested but local binaries are absent"
fi
if [[ -n "${operator_key_file}" ]]; then
  operator_address="$("${localctl_path}" --action address --key-file "${operator_key_file}")" || fail "derive operator address"
  fund_operator "${operator_address}"
fi

info "starting against chain ${chain_id}, Lendora config ${lendora_file}"
nohup "${binary_path}" -config "${config_path}" >"${log_path}" 2>&1 < /dev/null &
pid=$!
printf '%s\n' "${pid}" > "${pid_path}"
for _ in {1..20}; do
  if ! kill -0 "${pid}" 2>/dev/null; then rm -f "${pid_path}"; tail -n 80 "${log_path}" >&2 || true; fail "agent exited during startup"; fi
  if curl -fsS --max-time 2 "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1 && curl -fsS --max-time 2 "http://127.0.0.1:${port}/.well-known/agent-card.json" >/dev/null 2>&1; then
    info "ready"
    info "health: http://127.0.0.1:${port}/healthz"
    info "card:   ${public_url}/.well-known/agent-card.json"
    exit 0
  fi
  sleep 1
done
kill "${pid}" 2>/dev/null || true
rm -f "${pid_path}"
tail -n 80 "${log_path}" >&2 || true
fail "agent readiness check failed after 20 seconds"
