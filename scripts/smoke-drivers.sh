#!/usr/bin/env bash
#
# smoke-drivers.sh — create each built-in driver, check it actually comes up,
# then tear it down. Run this ON an apod server (it needs the `apod` CLI and
# Docker). It's the real "does every driver work?" check that can't be done
# without a Docker host.
#
#   ./smoke-drivers.sh [base-domain] [driver ...]
#
#   base-domain  domains are <driver>.<base-domain>   (default: smoke.test)
#   driver ...   limit to specific drivers (default: the self-provisioning set)
#
# Per driver it: destroys any leftover, creates the site, waits up to 3 min for
# the app container to be running AND stable (not restarting), then records
# PASS/FAIL. Logs for failures are saved to /tmp/smoke-<driver>.log. Sites are
# destroyed at the end unless KEEP=1.
#
# whmcs (needs WHMCS files + a license) and supabase (heavy; clones upstream)
# are excluded by default — pass them explicitly to include.
#
# After the driver matrix it runs a streaming/activity phase: it provisions one
# extra site and drives clone, backup, restore and destroy through it, asserting
# that /activity reports the held lock and that /events carries a terminal
# progress event for each. Set SKIP_STREAM=1 to skip that phase.

set -uo pipefail

BASE="${1:-smoke.test}"; shift || true
DEFAULT_DRIVERS=(static php wordpress node laravel paymenter odoo unifi)
DRIVERS=("${@:-${DEFAULT_DRIVERS[@]}}")

RAM="${RAM:-1G}"; CPU="${CPU:-1}"; DISK="${DISK:-3G}"
WAIT_SECS="${WAIT_SECS:-180}"
KEEP="${KEEP:-0}"

# Streaming / activity checks: exercise the live progress stream (/events) and
# the lock endpoint (/activity) against a real site for clone, backup, restore
# and destroy. They talk to the control socket as the local (admin) peer, so no
# API key is needed. Set SKIP_STREAM=1 to skip; STREAM_DRIVER picks the driver
# (php has a database, so backup/restore covers the DB dump/restore path).
SOCK="${APOD_SOCK:-/run/apod/apod.sock}"
STREAM_DRIVER="${STREAM_DRIVER:-php}"
SKIP_STREAM="${SKIP_STREAM:-0}"

pass=(); fail=()
sresult=()   # streaming-check outcomes: "<op> PASS" / "<op> FAIL: <reason>"

stable_running() {
  # Container exists, is running, and its RestartCount isn't climbing.
  local name="$1"
  local state restarts1 restarts2
  state=$(docker inspect -f '{{.State.Status}}' "$name" 2>/dev/null) || return 1
  [ "$state" = "running" ] || return 1
  restarts1=$(docker inspect -f '{{.RestartCount}}' "$name" 2>/dev/null || echo 0)
  sleep 5
  restarts2=$(docker inspect -f '{{.RestartCount}}' "$name" 2>/dev/null || echo 0)
  [ "$restarts1" = "$restarts2" ]   # not crash-looping
}

# api/sse: talk to the control socket as the local admin peer.
api() { curl -fsS --unix-socket "$SOCK" "http://apod$1" 2>/dev/null; }
sse() { curl -fsS --max-time "${2:-20}" --unix-socket "$SOCK" "http://apod$1" 2>/dev/null; }

# run_op_watch <op-label> <watch-domain> <require-held 0|1> -- <command...>
# Runs an operation in the background while polling /activity for its lock, then
# replays /events and verifies a terminal "done" arrived. The operation's own
# stdout/stderr is captured to /tmp/smoke-stream-op.log for the caller to parse.
run_op_watch() {
  local label="$1" wdom="$2" need_held="$3"; shift 3
  [ "${1:-}" = "--" ] && shift
  : >/tmp/smoke-stream-op.log
  ( "$@" ) >/tmp/smoke-stream-op.log 2>&1 &
  local pid=$! saw_held=0
  while kill -0 "$pid" 2>/dev/null; do
    if api "/api/v1/sites/$wdom/activity" | grep -q "\"operation\":\"$label\""; then
      saw_held=1
    fi
    sleep 0.2
  done
  wait "$pid"; local rc=$?

  # The just-finished run stays buffered (2-minute retention), so a post-hoc
  # subscribe replays its events and returns at the terminal one.
  local ev; ev=$(sse "/api/v1/sites/$wdom/events" 20)

  if [ "$rc" != 0 ]; then
    sresult+=("$label FAIL: command exited $rc (see /tmp/smoke-stream-op.log)"); return 1
  fi
  if ! grep -q '"status":"done"' <<<"$ev"; then
    sresult+=("$label FAIL: no terminal done event in /events stream"); return 1
  fi
  if [ "$need_held" = 1 ] && [ "$saw_held" != 1 ]; then
    sresult+=("$label FAIL: /activity never reported held with this operation"); return 1
  fi
  local note=""; [ "$saw_held" = 1 ] || note=" (held not observed — op too fast to catch)"
  sresult+=("$label PASS$note"); return 0
}

stream_phase() {
  [ "$SKIP_STREAM" = 1 ] && { echo; echo "Streaming checks skipped (SKIP_STREAM=1)"; return; }
  command -v curl >/dev/null 2>&1 || { echo; echo "curl not found — skipping streaming checks"; return; }
  [ -S "$SOCK" ] || { echo; echo "control socket $SOCK not found — skipping streaming checks"; return; }

  local dom="stream.${BASE}" clone="clone-stream.${BASE}"
  echo
  echo "=== streaming/activity checks  (${STREAM_DRIVER}, ${dom}) ==="
  apod destroy "$dom"   --purge >/dev/null 2>&1
  apod destroy "$clone" --purge >/dev/null 2>&1

  if ! apod create "$dom" --driver "$STREAM_DRIVER" --ram "$RAM" --cpu "$CPU" --storage "$DISK" \
        >/tmp/smoke-stream.log 2>&1; then
    echo "  ✗ could not provision streaming site — see /tmp/smoke-stream.log"
    sresult+=("provision FAIL")
    return
  fi
  for ((i=0; i<WAIT_SECS; i+=10)); do stable_running "apod-${dom}-app" && break; sleep 5; done

  # clone is the heaviest op (copies files + DB) → require /activity to report it.
  run_op_watch cloning "$dom" 1 -- apod clone "$dom" "$clone"
  apod destroy "$clone" --purge >/dev/null 2>&1

  # backup, then restore from the backup we just made (held is best-effort here).
  run_op_watch "backing up" "$dom" 0 -- apod backup create "$dom"
  local bid; bid=$(grep -oE 'ID: [0-9]+' /tmp/smoke-stream-op.log | grep -oE '[0-9]+' | head -1)
  if [ -n "$bid" ]; then
    run_op_watch "restoring backup" "$dom" 0 -- apod backup restore "$dom" "$bid"
  else
    sresult+=("restoring backup SKIP: could not parse backup id")
  fi

  # destroy is itself one of the streamed operations.
  run_op_watch destroying "$dom" 0 -- apod destroy "$dom" --purge
}

for d in "${DRIVERS[@]}"; do
  dom="${d}.${BASE}"
  app="apod-${dom}-app"
  echo "=== ${d}  (${dom}) ==="

  apod destroy "$dom" --purge >/dev/null 2>&1

  if ! apod create "$dom" --driver "$d" --ram "$RAM" --cpu "$CPU" --storage "$DISK" \
        >"/tmp/smoke-${d}.log" 2>&1; then
    echo "  ✗ create failed — see /tmp/smoke-${d}.log"
    fail+=("$d"); continue
  fi

  ok=0
  for ((i=0; i<WAIT_SECS; i+=10)); do
    if stable_running "$app"; then ok=1; break; fi
    sleep 5
  done

  if [ "$ok" = 1 ]; then
    echo "  ✓ app container running and stable"
    pass+=("$d")
  else
    echo "  ✗ app not running/stable — last 20 log lines:"
    docker logs --tail 20 "$app" 2>&1 | sed 's/^/      /' || true
    docker logs --tail 50 "$app" >"/tmp/smoke-${d}.log" 2>&1 || true
    fail+=("$d")
  fi

  [ "$KEEP" = 1 ] || apod destroy "$dom" --purge >/dev/null 2>&1
done

stream_phase

# Did any streaming check fail? (guard the expansion for empty/skipped runs)
stream_fail=0
for r in ${sresult[@]+"${sresult[@]}"}; do [[ "$r" == *FAIL* ]] && stream_fail=1; done

echo
echo "──────────────────────────────────────────"
echo "PASS (${#pass[@]}): ${pass[*]:-none}"
echo "FAIL (${#fail[@]}): ${fail[*]:-none}"
echo "Failure logs: /tmp/smoke-<driver>.log"
if [ "${#sresult[@]}" -gt 0 ]; then
  echo "Streaming/activity checks:"
  for r in "${sresult[@]}"; do echo "  - $r"; done
fi
[ "${#fail[@]}" -eq 0 ] && [ "$stream_fail" -eq 0 ]
