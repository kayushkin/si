#!/usr/bin/env bash
# Boot-and-answer smoke test for si.
#
# Builds cmd/si from THIS source tree, boots it on a temp port with the in-process
# echo feed, and drives the ONE thing si exists to do: route a message from an
# adapter, through the feed, and back out to that adapter's clients.
#
# Why this exists: `go build` proves the tree compiles, not that the binary is
# alive. Go 1.22+ http.ServeMux panics on a conflicting route pattern at
# REGISTRATION time — the compiler never sees it, `go vet` never sees it, and the
# process dies on its first breath. Tier 1 of the repo guard calls such a tree
# green. This is tier 2: it boots the thing and makes it answer.
#
# 🔴 SI FAILS TO LISTEN *SILENTLY*, AND THAT IS THE HEADLINE FAILURE MODE HERE.
# router.Run starts each adapter in a goroutine and merely LOGS if one stops
# (router.go:111-115). If the WebSocket adapter cannot bind its port, si does not
# exit — it keeps running, with no listener, forever. So "the process is still
# alive" proves NOTHING about si, and any check built on that alone is decorative.
# This script therefore polls the PORT for readiness and treats "never answered"
# as a failure, independently of whether the process survived.
#
# WHAT MAKES THIS HERMETIC — SI_FEED=echo, AND WHY IT IS NOT NEGOTIABLE:
# si's default feed is NATS (cmd/si/main.go:20), and pointing this smoke at the
# live bus would be far worse than merely noisy:
#   * feed/nats.go:121 opens a JetStream push consumer with the DURABLE NAME
#     "si" — the same durable the live si uses. A second binder either fails
#     ("consumer is already bound to a subscription") or, if prod si is briefly
#     down, TAKES OVER that durable and ACKs its backlog, permanently advancing
#     the live consumer's position. That is not stolen traffic; that is destroyed
#     traffic.
#   * feed/nats.go:180-195 publishes every message a WS client sends to
#     chat.inbound.<orchestrator> — so a smoke's test message would be injected
#     into the live orchestrators as a real user prompt.
# SI_FEED=echo (main.go:28-29) bypasses NATS entirely: feed.NewEcho() is an
# in-process goroutine with no sockets at all. NATS_URL is never even read.
#
# LOGSTACK_URL is pointed at a CLOSED port, not left at its default: NewRouter
# does a synchronous health GET with a 2s timeout (logstack.go:44), so the default
# would either reach the live logstack or stall boot for two seconds. A closed
# port gives an instant ECONNREFUSED and si disables logging and moves on.
#
# si has NO /health route. It serves exactly /ws and /api/status
# (adapter/websocket/websocket.go:86-87). /api/status is the readiness probe.
#
# Exits 0 on success, non-zero on the FIRST failing assertion, dumping the
# server log to stderr.
#
# Env:
#   E2E_PORT   — si websocket/status port (default 19130)
#   E2E_KEEP=1 — leave $TMP_DIR in place after the run

set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${E2E_PORT:-19130}"
BASE="http://127.0.0.1:$PORT"

for bin in go curl jq; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "ERROR: required tool '$bin' not found on PATH" >&2
    exit 2
  fi
done

TMP_DIR="$(mktemp -d -t si-e2e.XXXXXX)"
BIN_DIR="$TMP_DIR/bin"
FAKE_HOME="$TMP_DIR/home"
SERVER_LOG="$TMP_DIR/server.log"
mkdir -p "$BIN_DIR" "$FAKE_HOME"

SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [ "${E2E_KEEP:-}" = "1" ]; then
    echo "[e2e] keeping $TMP_DIR"
  else
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT INT TERM

step() { printf '\n==> %s\n' "$*"; }
dump_logs() {
  echo "----- server.log -----" >&2
  cat "$SERVER_LOG" >&2 2>/dev/null || true
}
fail() { echo "FAIL: $*" >&2; dump_logs; exit 1; }

step "build cmd/si and the ws probe from $REPO_DIR"
cd "$REPO_DIR"
# Explicit -o for both. A bare `go build ./cmd/si` writes ./si into the CWD, and
# `go build -o si .` (what the justfile still does) silently produces a package
# ARCHIVE rather than an executable, because the root package is a library.
go build -o "$BIN_DIR/si" ./cmd/si
go build -o "$BIN_DIR/wsprobe" ./scripts/wsprobe
echo "    si:      $(ls -lh "$BIN_DIR/si" | awk '{print $5}')"
echo "    wsprobe: $(ls -lh "$BIN_DIR/wsprobe" | awk '{print $5}')"

step "launch si on :$PORT with the in-process echo feed (no NATS — see header)"
env -i \
  PATH="$PATH" \
  HOME="$FAKE_HOME" \
  SI_FEED=echo \
  SI_WS_ADDR="127.0.0.1:$PORT" \
  LOGSTACK_URL="http://127.0.0.1:1" \
  "$BIN_DIR/si" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
echo "    pid: $SERVER_PID"

# Poll the PORT — never sleep-and-hope, and never infer health from the pid.
# A dead pid is one failure (route panic); a live pid that never listens is the
# OTHER, si-specific failure, and it is the silent one.
status=""
for _ in $(seq 1 50); do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    fail "si exited during startup (route registration panic? feed fatal?)"
  fi
  if status=$(curl -fsS --max-time 2 "$BASE/api/status" 2>/dev/null); then break; fi
  status=""
  sleep 0.2
done
[ -n "$status" ] || fail "si is running but never answered $BASE/api/status within 10s — the websocket adapter failed to bind and si did NOT exit (router.go:111-115 only logs it)"
echo "    status OK: $status"

step "GET /api/status — assert the parsed adapter set, not just a 200"
[ "$(jq -r '.adapters | length' <<<"$status")" = "1" ] || fail "expected exactly 1 adapter: $status"
[ "$(jq -r '.adapters[0]' <<<"$status")" = "websocket" ] || fail "adapter is not 'websocket': $status"
[ "$(jq -r '.ws_clients' <<<"$status")" = "0" ] || fail "a freshly booted si should have 0 ws_clients: $status"
echo "    adapters=[websocket] ws_clients=0"

step "WS round trip — the whole point of si: adapter -> router -> feed -> adapter"
# This is the write-then-read-back. It drives the real socket, the real router,
# and the real (echo) feed. The echo feed answers "ping" with "pong"
# (feed/echo.go:30-69) after a 200ms delay, authored as "inber".
REPLY=$("$BIN_DIR/wsprobe" -url "ws://127.0.0.1:$PORT/ws" -text ping -timeout 10s) \
  || fail "ws round trip failed — si accepted the connection but no outbound reply came back through the feed"
echo "    reply: $REPLY"

[ "$(jq -r '.text'   <<<"$REPLY")" = "pong"  ] || fail "echo feed did not answer ping with pong: $REPLY"
[ "$(jq -r '.author' <<<"$REPLY")" = "inber" ] || fail "reply author is not 'inber': $REPLY"
# The server stamps every message it accepts (websocket.go:232-241). An empty id
# or channel means the message went out without ever passing through that path.
[ -n "$(jq -r '.id // empty' <<<"$REPLY")" ] || fail "reply carries no id — si did not stamp it: $REPLY"
echo "    ping -> pong (author=inber), router+feed pipeline is live"

step "GET /api/status — si survived the socket and still answers"
status=$(curl -fsS --max-time 10 "$BASE/api/status") || fail "si stopped answering /api/status after the ws round trip"
[ "$(jq -r '.ws_clients' <<<"$status")" = "0" ] || fail "ws_clients did not return to 0 after the probe disconnected: $status"
echo "    ws_clients back to 0 (client cleanly unregistered)"

step "process still alive after serving every route"
kill -0 "$SERVER_PID" 2>/dev/null || fail "si died while serving"

printf '\nsmoke test OK (si boots, listens, and round-trips a message on :%s)\n' "$PORT"
