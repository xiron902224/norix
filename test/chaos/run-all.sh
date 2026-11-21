#!/usr/bin/env bash
# Run all chaos scenarios against one shared stack (#41).
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=lib.sh
source "$DIR/lib.sh"

chaos_setup 2
trap chaos_teardown EXIT

export CHAOS_SESSION
SESSION="$(chaos_session)"
export CHAOS_SESSION="$SESSION"

# Order matters: restart/DB first (agents still up), partition, then crash last.
"$DIR/control-plane-restart.sh"
"$DIR/db-restart.sh"
"$DIR/network-partition.sh"
"$DIR/agent-crash.sh"

chaos_log "OK all chaos scenarios"
