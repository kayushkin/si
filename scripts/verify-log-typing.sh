#!/usr/bin/env bash
# Prove the DEPLOYED si files plumbing under logstack's routing bucket, not its
# billable-turn bucket.
#
# This is the curative check for si `6a949b7`. si's EventType is a routing
# direction — EventOutbound means "feed → adapters", whatever the message was.
# logstack's models.TypeOutbound means "a completed agent turn", and it is the
# only type its usage readers select. si used to copy its own string straight
# into logstack's field, so every republished healthcheck status change — no
# agent, no author, no orchestrator — was filed as a billable turn. That is
# about 95% of si's whole outbound bucket. The count behind that share is dated
# and stated once, in logstack.go's entryType — restating it here is how the two
# copies drifted apart before.
#
# `go test ./...` already pins entryType()'s mapping. What it cannot see is
# WHICH BINARY IS LIVE, and that is the only thing in question here: the fix was
# committed 2026-08-01 and ~/bin/si was 19 days stale behind it. So this script
# deliberately reads the live store rather than the source tree.
#
# THE ASSERTION: among entries appended after the baseline, one that names no
# speaker must be typed `routing`. A single `outbound` with an empty agent means
# the running binary predates the fix.
#
# It reports three outcomes, and INCONCLUSIVE is a real one — plumbing arrives
# on its own schedule (~1 entry every 18 minutes here, not the once-a-minute the
# original measurement saw), so a short window can legitimately catch nothing.
# Calling that a pass would be the lie this whole fix is about.
#
# Usage:
#   scripts/verify-log-typing.sh baseline            # print a baseline token
#   scripts/verify-log-typing.sh check <token> [sec] # classify what landed since
#   scripts/verify-log-typing.sh watch [sec]         # baseline + check, standalone
#
# Exit: 0 PASS · 1 FAIL (old binary is live) · 3 INCONCLUSIVE (no plumbing yet)

set -euo pipefail

LOGSTACK_UNIT="${LOGSTACK_UNIT:-$HOME/.config/systemd/user/logstack.service}"
DEFAULT_WAIT=90

fail() { echo "ERROR: $*" >&2; exit 2; }

# The data directory is logstack's, so read it out of logstack's unit rather
# than restating it here. si has no say in where these files land, and a second
# copy of the path is a second thing to drift.
data_dir() {
  local dir
  [ -f "$LOGSTACK_UNIT" ] || fail "logstack unit not found at $LOGSTACK_UNIT — cannot locate the log store (override with LOGSTACK_UNIT=)"
  dir="$(sed -n 's|^Environment=LOGSTACK_DATA_DIR=||p' "$LOGSTACK_UNIT" | tail -1 | sed "s|%h|$HOME|g")"
  [ -n "$dir" ] || fail "$LOGSTACK_UNIT declares no LOGSTACK_DATA_DIR — refusing to guess where the store is"
  printf '%s' "$dir"
}

# Plumbing carries no orchestrator, and logstack names the file after the
# orchestrator — so it lands in unknown.jsonl. Per UTC day, which is why the
# check below re-resolves this on every poll instead of caching it.
store_file() { printf '%s/%s/unknown.jsonl' "$(data_dir)" "$(date -u +%F)"; }

size_of() { [ -f "$1" ] && wc -c <"$1" | tr -d ' ' || echo 0; }

cmd_baseline() {
  local f; f="$(store_file)"
  printf '%s:%s\n' "$f" "$(size_of "$f")"
}

cmd_check() {
  local token="${1:-}" wait_secs="${2:-$DEFAULT_WAIT}"
  [ -n "$token" ] || fail "check needs a baseline token from \`$0 baseline\`"
  command -v jq >/dev/null 2>&1 || fail "required tool 'jq' not on PATH"

  local base_file="${token%:*}" base_off="${token##*:}"
  [[ "$base_off" =~ ^[0-9]+$ ]] || fail "malformed baseline token: $token"

  echo "==> watching for plumbing entries (up to ${wait_secs}s)"
  echo "    baseline: $base_file @ ${base_off} bytes"

  local deadline=$((SECONDS + wait_secs))
  while :; do
    # Re-resolve every poll: a UTC day boundary crossed mid-wait starts a new
    # file, and entries would otherwise land somewhere nobody is looking.
    local now_file; now_file="$(store_file)"
    local off=0
    [ "$now_file" = "$base_file" ] && off="$base_off"

    if [ -f "$now_file" ] && [ "$(size_of "$now_file")" -gt "$off" ]; then
      # tail -c +N is 1-indexed, so the byte AFTER the baseline is +N+1.
      local fresh; fresh="$(tail -c "+$((off + 1))" "$now_file")"

      # An entry that names no speaker is plumbing. `// empty` collapses a null
      # agent and an absent one onto the same answer as "".
      local bad good
      bad="$(jq -rs '[.[] | select((.agent // "") == "") | select(.type == "outbound")] | length' <<<"$fresh" 2>/dev/null || echo 0)"
      good="$(jq -rs '[.[] | select((.agent // "") == "") | select(.type == "routing")] | length' <<<"$fresh" 2>/dev/null || echo 0)"

      if [ "${bad:-0}" -gt 0 ]; then
        echo "    $bad new speakerless entrie(s) typed 'outbound' in $now_file" >&2
        jq -rs '[.[] | select((.agent // "") == "") | select(.type == "outbound")] | .[0]' <<<"$fresh" >&2 || true
        echo
        echo "FAIL: the running si still files plumbing as a billable turn — the deployed binary predates 6a949b7." >&2
        exit 1
      fi
      if [ "${good:-0}" -gt 0 ]; then
        echo "    $good new speakerless entrie(s), all typed 'routing'"
        echo
        echo "PASS: the running si files plumbing under logstack's routing bucket."
        exit 0
      fi
    fi

    [ "$SECONDS" -lt "$deadline" ] || break
    sleep 2
  done

  echo
  echo "INCONCLUSIVE: no speakerless entry arrived within ${wait_secs}s, so nothing was proved either way." >&2
  echo "  Plumbing lands roughly every 18 minutes. Re-run when some has:" >&2
  echo "    $0 check '$token' 1800" >&2
  exit 3
}

case "${1:-watch}" in
  baseline) cmd_baseline ;;
  check)    shift; cmd_check "$@" ;;
  watch)    shift; cmd_check "$(cmd_baseline)" "${1:-$DEFAULT_WAIT}" ;;
  *)        fail "unknown subcommand '${1}' (use: baseline | check | watch)" ;;
esac
