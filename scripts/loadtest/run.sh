#!/usr/bin/env bash
# Load-tests gpu-runner against 4 fake GPUs and a local Redis.
#
#   scripts/loadtest/run.sh          # normal run
#   scripts/loadtest/run.sh crash    # kill -9 the server mid-run, restart it,
#                                    # and check no job is lost
#
# Env: JOBS (default 200), REDIS_ADDR (localhost:6379), REDIS_DB (15),
# PORT (18080). The Redis DB must be empty; it is flushed afterwards.
set -euo pipefail

MODE=${1:-normal}
JOBS=${JOBS:-200}
REDIS_ADDR=${REDIS_ADDR:-localhost:6379}
REDIS_DB=${REDIS_DB:-15}
PORT=${PORT:-18080}
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
WORK=$(mktemp -d)
SERVER_PID=""

redis() { redis-cli -u "redis://$REDIS_ADDR/$REDIS_DB" "$@"; }

if [ "$(redis DBSIZE)" != "0" ]; then
  echo "Redis DB $REDIS_DB is not empty; refusing to touch it." >&2
  exit 1
fi

cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
  redis FLUSHDB >/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

# 4 fake GPUs: two 80 GB, two 24 GB. No per-process memory is reported, so
# memory enforcement never kills a job here.
cat > "$WORK/nvidia-smi" <<'EOF'
#!/bin/sh
case "$1" in
  --query-compute-apps=*) ;;
  *)
    echo "GPU-a100-0, NVIDIA A100-SXM4-80GB, 81920, 80000"
    echo "GPU-a100-1, NVIDIA A100-SXM4-80GB, 81920, 80000"
    echo "GPU-4090-0, NVIDIA GeForce RTX 4090, 24564, 24000"
    echo "GPU-4090-1, NVIDIA GeForce RTX 4090, 24564, 24000" ;;
esac
EOF
chmod +x "$WORK/nvidia-smi"
mkdir -p "$WORK/logs" "$WORK/v10" "$WORK/v25" "$WORK/v50"

echo "Building..."
(cd "$ROOT" && go build -o "$WORK/server" ./cmd/server && go build -o "$WORK/loadtest" ./scripts/loadtest)

start_server() {
  GPU_ENABLED=true NVIDIA_SMI_PATH="$WORK/nvidia-smi" GPU_POLL_INTERVAL=1s \
  REDIS_ADDRESS="$REDIS_ADDR" REDIS_DB="$REDIS_DB" SERVER_ADDRESS=127.0.0.1 SERVER_PORT="$PORT" \
  WORKER_QUEUE_CAPACITY=50 DATABASE_PATH="$WORK/jobs.db" LOG_DIR="$WORK/logs" \
  STORAGE_VOLUME_10MB_PATH="$WORK/v10" STORAGE_VOLUME_25MB_PATH="$WORK/v25" STORAGE_VOLUME_50MB_PATH="$WORK/v50" \
    "$WORK/server" >>"$WORK/server.out" 2>&1 &
  SERVER_PID=$!
  for _ in $(seq 50); do
    curl -sf "http://127.0.0.1:$PORT/metrics" >/dev/null && return
    sleep 0.2
  done
  echo "server did not start; see $WORK/server.out" >&2
  exit 1
}

start_server

if [ "$MODE" = "crash" ]; then
  "$WORK/loadtest" -server "http://127.0.0.1:$PORT" -jobs "$JOBS" -ready-file "$WORK/ready" &
  LT_PID=$!
  while [ ! -f "$WORK/ready" ]; do sleep 0.2; done
  sleep 3
  echo ">>> kill -9 server (pid $SERVER_PID) with jobs in flight: processing=$(redis LLEN gpu-runner:jobs:processing) pending=$(redis LLEN gpu-runner:jobs:pending)"
  kill -9 "$SERVER_PID"; SERVER_PID=""
  pkill -f "^gpurunner-loadtest " 2>/dev/null || true # jobs die with the machine
  sleep 1
  start_server
  echo ">>> restarted: $(grep -o 'Requeued jobs left running by a previous crash.*' "$WORK/logs/server.log" | tail -1)"
  wait "$LT_PID"
else
  "$WORK/loadtest" -server "http://127.0.0.1:$PORT" -jobs "$JOBS"
fi

echo "Redis after test: pending=$(redis LLEN gpu-runner:jobs:pending) processing=$(redis LLEN gpu-runner:jobs:processing)"
echo "Server errors logged: $(grep -c 'level=ERROR' "$WORK/logs/server.log" || true) (storage-dir errors are expected on macOS)"
