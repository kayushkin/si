#!/usr/bin/env bash
# Build, install and restart the live si service, then prove it still routes.
#
# si is the comms layer — Discord, matterbridge, websocket and TUI adapters —
# so a restart drops the user's chat bridges for as long as it takes to come
# back. That is why this script exists rather than a hand-run build-and-copy:
# every step that can be checked before the restart is checked before it, and
# the restart is followed by a real assertion rather than a sleep.
#
# The unit is installed FROM ./si.service, and every value this script needs
# (binary path, working directory, feed mode, NATS URL, websocket address) is
# read back OUT of that unit rather than restated here. There is deliberately no
# second copy to drift. Until 2026-08-07 the tracked si.service was a fossil of
# a different host — `User=slava`, `/home/slava/repos/si`, and `SI_FEED=inber`,
# which is not one of the three modes cmd/si/main.go accepts. Installing it
# would have crash-looped the service. It now matches what actually runs.
#
# si.service is a --user unit, so no sudo is involved.
#
# Usage: ./deploy.sh
#   SI_DEPLOY_ALLOW_DIRTY=1   deploy a tree with uncommitted changes
#   SI_DEPLOY_VERIFY_SECS=N   how long to wait for plumbing traffic (default 120)
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"
UNIT_SRC="$REPO_DIR/si.service"
UNIT_DIR="$HOME/.config/systemd/user"
UNIT_NAME="si.service"
BACKUP_DIR="$HOME/.local/share/si-deploy-backup"
VERIFY_SECS="${SI_DEPLOY_VERIFY_SECS:-120}"

# go lives behind mise shims on this host; a non-login shell (automation, an
# agent session) does not get them on PATH.
export PATH="$HOME/.local/share/mise/shims:$PATH"
# systemctl --user needs these to reach the user manager. Without them it prints
# NOTHING and exits 0 — silence that reads exactly like success.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
export DBUS_SESSION_BUS_ADDRESS="${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/bus}"

step() { printf '\n==> %s\n' "$*"; }
fail() { echo "DEPLOY FAILED: $*" >&2; exit 1; }

for bin in go curl jq systemctl; do
  command -v "$bin" >/dev/null 2>&1 || fail "required tool '$bin' not on PATH"
done
[ -f "$UNIT_SRC" ] || fail "missing unit template: $UNIT_SRC"

step "read the unit template — it is the source of truth for these values"
# Resolve systemd's %h ourselves; everything below has to agree with what
# systemd will actually exec.
unit_value() { sed -n "s|^$1=||p" "$UNIT_SRC" | tail -1 | sed "s|%h|$HOME|g"; }
unit_env()   { sed -n "s|^Environment=$1=||p" "$UNIT_SRC" | tail -1 | sed "s|%h|$HOME|g"; }

BIN_PATH="$(unit_value ExecStart)"
WORK_DIR="$(unit_value WorkingDirectory)"
FEED="$(unit_env SI_FEED)"
NATS_URL="$(unit_env NATS_URL)"
WS_ADDR="$(unit_env SI_WS_ADDR)"
[ -n "$BIN_PATH" ] || fail "unit has no ExecStart"
[ -n "$WORK_DIR" ] || fail "unit has no WorkingDirectory"
[ -n "$WS_ADDR" ]  || fail "unit declares no SI_WS_ADDR — refusing to guess which port to probe"
[ -n "$FEED" ]     || fail "unit declares no SI_FEED — refusing to guess (an unset one defaults to nats silently)"
echo "    binary:    $BIN_PATH"
echo "    feed:      $FEED"
echo "    ws addr:   $WS_ADDR"
# A bare ":8090" is a wildcard bind; probe it on loopback.
BASE="http://${WS_ADDR/#:/127.0.0.1:}"

step "preflight"
# systemd reports a missing WorkingDirectory and a missing binary with the same
# nameless 'result: resources' failure, then Restart=always loops on it forever.
# Name which, before the restart rather than after.
[ -d "$WORK_DIR" ] || fail "WorkingDirectory does not exist: $WORK_DIR"

# The one that actually bites. SI_FEED=nats makes cmd/si/main.go log.Fatalf if
# it cannot reach NATS, and the unit restarts always — so deploying while NATS
# is down does not degrade si, it removes it and spins. Check the socket first.
if [ "$FEED" = "nats" ] || [ "$FEED" = "bus" ]; then
  [ -n "$NATS_URL" ] || fail "SI_FEED=$FEED but the unit declares no NATS_URL"
  nats_hostport="${NATS_URL#nats://}"; nats_hostport="${nats_hostport%%/*}"
  nats_host="${nats_hostport%%:*}"; nats_port="${nats_hostport##*:}"
  [ "$nats_host" = "$nats_port" ] && nats_port=4222
  timeout 3 bash -c "exec 3<>/dev/tcp/$nats_host/$nats_port" 2>/dev/null \
    || fail "NATS is not accepting connections on $nats_host:$nats_port — si would log.Fatalf on boot and Restart=always would loop it. Start NATS first."
  echo "    NATS reachable at $nats_host:$nats_port"
fi

cd "$REPO_DIR"
if [ -n "$(git status --porcelain 2>/dev/null)" ] && [ "${SI_DEPLOY_ALLOW_DIRTY:-}" != "1" ]; then
  git status --short >&2
  fail "working tree is dirty — the installed binary would match no commit. Commit, or re-run with SI_DEPLOY_ALLOW_DIRTY=1"
fi
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo '(no git)')"
echo "    deploying $COMMIT from a clean tree"

step "build, vet, test"
# No gofmt gate: adapter/openclaw/openclaw.go and message.go are gofmt-dirty at
# HEAD and have been deliberately left that way. A gate here would block every
# deploy on a condition this script is not the right place to fix.
go build ./... || fail "go build failed"
go vet ./...   || fail "go vet failed"
go test ./...  || fail "go test failed"

STAGED="$REPO_DIR/build/si"
mkdir -p "$REPO_DIR/build"
# Explicit -o, and ./cmd/si not `.`. `go build -o si .` — which the justfile
# still does — silently produces a package ARCHIVE, not an executable, because
# the root package is a library.
go build -o "$STAGED" ./cmd/si || fail "building ./cmd/si failed"
[ -x "$STAGED" ] || fail "no executable produced at $STAGED"
echo "    built: $(ls -lh "$STAGED" | awk '{print $5}')"

step "boot-and-answer smoke, before touching the live service"
# `go build` passing says the tree compiles, not that the binary is alive, and
# si's headline failure mode is silent: router.Run only LOGS an adapter that
# stopped, so a si that cannot bind its port keeps running with no listener
# forever. The smoke boots it on a temp port with the in-process echo feed and
# round-trips a real message. It never touches NATS — see that script's header
# for why pointing it at the live bus would destroy traffic rather than just
# make noise.
./scripts/e2e-smoke.sh >/dev/null || fail "e2e smoke failed — not installing. Run ./scripts/e2e-smoke.sh to see why."
echo "    smoke passed (si boots, listens, round-trips)"

step "baseline the log store, before the restart"
# Taken now so the post-restart check can tell entries the NEW binary wrote from
# entries the old one already had on disk.
LOG_BASELINE="$(./scripts/verify-log-typing.sh baseline)"
echo "    $LOG_BASELINE"

step "install"
mkdir -p "$BACKUP_DIR" "$(dirname "$BIN_PATH")" "$UNIT_DIR"
if [ -f "$BIN_PATH" ]; then
  cp -p "$BIN_PATH" "$BACKUP_DIR/si.prev"
  echo "    previous binary backed up to $BACKUP_DIR/si.prev"
fi
# cp over a running binary fails ETXTBSY. Stage alongside, then rename — which
# is atomic, so there is no window where $BIN_PATH is half-written.
cp "$STAGED" "$BIN_PATH.new"
chmod +x "$BIN_PATH.new"
mv -f "$BIN_PATH.new" "$BIN_PATH"

if ! diff -q "$UNIT_SRC" "$UNIT_DIR/$UNIT_NAME" >/dev/null 2>&1; then
  echo "    unit differs from the installed one:"
  diff -u "$UNIT_DIR/$UNIT_NAME" "$UNIT_SRC" 2>/dev/null | sed 's/^/      /' || true
fi
install -m 0644 "$UNIT_SRC" "$UNIT_DIR/$UNIT_NAME"
systemctl --user daemon-reload
printf '%s deployed %s\n' "$(date -u +%FT%TZ)" "$COMMIT" >>"$BACKUP_DIR/deployed.log"
echo "    installed $BIN_PATH and $UNIT_DIR/$UNIT_NAME"

step "restart $UNIT_NAME (chat bridges drop until it answers again)"
systemctl --user restart "$UNIT_NAME"

step "wait for it to answer on $BASE/api/status"
# Poll, never sleep-and-hope, and never infer health from the unit being active:
# an active si with no listener is the exact failure this waits out.
READY=0
for _ in $(seq 1 80); do
  if ! systemctl --user is-active --quiet "$UNIT_NAME"; then
    systemctl --user status "$UNIT_NAME" --no-pager -l | tail -20 >&2
    fail "$UNIT_NAME is not active — it died on startup (see status above)"
  fi
  if curl -fsS -o /dev/null --max-time 2 "$BASE/api/status" 2>/dev/null; then READY=1; break; fi
  sleep 0.25
done
[ "$READY" = "1" ] || fail "$UNIT_NAME is running but never answered $BASE/api/status within ~20s — the websocket adapter failed to bind and si did NOT exit (router.go only logs it)"

step "verify the live service"
STATUS="$(curl -fsS --max-time 5 "$BASE/api/status")"
echo "    /api/status: $STATUS"
jq -e '.adapters | index("websocket")' <<<"$STATUS" >/dev/null \
  || fail "the websocket adapter is not registered: $STATUS"
echo "    websocket adapter registered"

step "verify the fix this deploy carries — plumbing must be typed 'routing'"
# The whole reason ~/bin/si needed replacing. Three outcomes and INCONCLUSIVE is
# a real one: plumbing arrives on its own schedule, so a quiet window proves
# nothing and must not be reported as a pass.
set +e
./scripts/verify-log-typing.sh check "$LOG_BASELINE" "$VERIFY_SECS"
LOG_RC=$?
set -e
case "$LOG_RC" in
  0) ;;
  1) fail "si is live but still mistyping plumbing — roll back: cp $BACKUP_DIR/si.prev $BIN_PATH && systemctl --user restart $UNIT_NAME" ;;
  3) echo "    NOT PROVEN YET — si is healthy, but no plumbing arrived to check. Re-run:"
     echo "      ./scripts/verify-log-typing.sh check '$LOG_BASELINE' 1800" ;;
  *) fail "verify-log-typing.sh errored (exit $LOG_RC)" ;;
esac

printf '\n==> DEPLOYED — si %s is live on %s\n' "$COMMIT" "$BASE"
echo "    rollback: cp $BACKUP_DIR/si.prev $BIN_PATH && systemctl --user restart $UNIT_NAME"
