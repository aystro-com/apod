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

set -uo pipefail

BASE="${1:-smoke.test}"; shift || true
DEFAULT_DRIVERS=(static php wordpress node laravel paymenter odoo unifi)
DRIVERS=("${@:-${DEFAULT_DRIVERS[@]}}")

RAM="${RAM:-1G}"; CPU="${CPU:-1}"; DISK="${DISK:-3G}"
WAIT_SECS="${WAIT_SECS:-180}"
KEEP="${KEEP:-0}"

pass=(); fail=()

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

echo
echo "──────────────────────────────────────────"
echo "PASS (${#pass[@]}): ${pass[*]:-none}"
echo "FAIL (${#fail[@]}): ${fail[*]:-none}"
echo "Failure logs: /tmp/smoke-<driver>.log"
[ "${#fail[@]}" -eq 0 ]
