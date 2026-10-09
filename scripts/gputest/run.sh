#!/usr/bin/env bash
# Tests gpu-runner on a real GPU machine (e.g. a vast.ai box). Run it ON the
# GPU machine, from a clone of the repo:
#
#   git clone -b itai/gpu-test https://github.com/itaischwarz/gpu-runner
#   cd gpu-runner && bash scripts/gputest/run.sh
#
# It installs Go, Redis and PyTorch if missing, starts the server against the
# real GPU, and checks: GPU discovery, oversized-job rejection, GPU pinning,
# per-process memory visibility, a job under its memory request, a job over
# its memory request (should be killed), and a short load test.
#
# The server is left running on 127.0.0.1:$PORT afterwards so you can poke it
# (e.g. through `ssh -L 8080:localhost:8080`). Re-running the script restarts it.
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
WORK=${WORK:-/root/gputest}
PORT=${PORT:-8080}
REDIS_DB=${REDIS_DB:-15}
URL=http://127.0.0.1:$PORT
export PATH=/usr/local/go/bin:$PATH

PASS=0
FAIL=0
pass() { echo "  PASS  $*"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL  $*"; FAIL=$((FAIL + 1)); }
step() { echo; echo "==> $*"; }

mkdir -p "$WORK/logs" "$WORK/v10" "$WORK/v25" "$WORK/v50"

# ---------------------------------------------------------------- install --
step "Installing dependencies (first run takes a few minutes for PyTorch)"
if ! command -v redis-server >/dev/null || ! command -v jq >/dev/null || ! command -v gcc >/dev/null; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq && apt-get install -y -qq redis-server jq gcc curl >/dev/null || { echo "apt install failed"; exit 1; }
fi
if ! command -v go >/dev/null; then
  curl -sSL https://go.dev/dl/go1.25.5.linux-amd64.tar.gz | tar xz -C /usr/local || { echo "Go install failed"; exit 1; }
fi
if [ -x /venv/main/bin/python ]; then
  PY=/venv/main/bin/python
else
  [ -x "$WORK/venv/bin/python" ] || python3 -m venv "$WORK/venv"
  PY=$WORK/venv/bin/python
fi
if ! "$PY" -c 'import torch' 2>/dev/null; then
  if command -v uv >/dev/null; then uv pip install -q --python "$PY" torch; else "$PY" -m pip install -q torch; fi
fi
"$PY" -c 'import torch; assert torch.cuda.is_available(); print("  torch", torch.__version__, "on", torch.cuda.get_device_name(0))' \
  || { echo "PyTorch cannot see the GPU"; exit 1; }
go version | sed 's/^/  /'

redis-cli ping >/dev/null 2>&1 || redis-server --daemonize yes --save '' --appendonly no >/dev/null
redis() { redis-cli -n "$REDIS_DB" "$@"; }

step "Building"
(cd "$ROOT" && go build -o "$WORK/server" ./cmd/server && go build -o "$WORK/loadtest" ./scripts/loadtest) || { echo "build failed"; exit 1; }
echo "  ok"

# ----------------------------------------------------------------- server --
step "Starting server on $URL"
if [ -f "$WORK/server.pid" ]; then kill "$(cat "$WORK/server.pid")" 2>/dev/null; sleep 1; fi
redis FLUSHDB >/dev/null # test DB only; jobs from a previous run would be replayed
rm -f "$WORK/jobs.db"
GPU_ENABLED=true GPU_POLL_INTERVAL=2s WORKER_JOB_TIMEOUT=120s WORKER_QUEUE_CAPACITY=50 \
REDIS_ADDRESS=127.0.0.1:6379 REDIS_DB="$REDIS_DB" SERVER_ADDRESS=127.0.0.1 SERVER_PORT="$PORT" \
DATABASE_PATH="$WORK/jobs.db" LOG_DIR="$WORK/logs" \
STORAGE_VOLUME_10MB_PATH="$WORK/v10" STORAGE_VOLUME_25MB_PATH="$WORK/v25" STORAGE_VOLUME_50MB_PATH="$WORK/v50" \
  nohup "$WORK/server" >"$WORK/server.out" 2>&1 &
echo $! >"$WORK/server.pid"
for _ in $(seq 50); do curl -sf "$URL/metrics" >/dev/null && break; sleep 0.2; done
curl -sf "$URL/metrics" >/dev/null || { echo "server did not start:"; tail -20 "$WORK/server.out"; exit 1; }
echo "  pid $(cat "$WORK/server.pid"), logs in $WORK/logs/server.log"

# A small PyTorch program that holds N GB of GPU memory for S seconds.
cat >"$WORK/alloc.py" <<'EOF'
import sys, time, torch
x = torch.empty(int(float(sys.argv[1]) * 1024**3), dtype=torch.uint8, device="cuda")
torch.cuda.synchronize()
time.sleep(float(sys.argv[2]))
EOF

# submit COMMAND MEMORY_MB -> prints the job ID
submit() {
  curl -s -X POST "$URL/jobs" -H 'Content-Type: application/json' \
    -d "$(jq -n --arg c "$1" --argjson m "$2" '{command: $c, memory_mb: $m, max_retries: 0}')" | jq -r '.id // empty'
}
# wait_job ID [TIMEOUT_S] -> prints the final status
wait_job() {
  local s
  for _ in $(seq $((${2:-120} * 2))); do
    s=$(curl -s "$URL/jobs/$1" | jq -r .status)
    case $s in success | failed | cancelled) echo "$s"; return ;; esac
    sleep 0.5
  done
  echo "timeout"
}
job_error() { curl -s "$URL/jobs/$1" | jq -r '.error // ""'; }

# ----------------------------------------------------------------- checks --
step "1. GPU discovery"
SMI_GPUS=$(nvidia-smi --query-gpu=uuid --format=csv,noheader | wc -l)
SEEN=$(curl -s "$URL/metrics" | grep -c '^gpu_runner_gpu_memory_total_mb')
curl -s "$URL/metrics" | grep -E '^gpu_runner_gpu_(memory_total_mb|memory_free_mb|healthy)' | sed 's/^/        /'
[ "$SEEN" = "$SMI_GPUS" ] && [ "$SEEN" -gt 0 ] && pass "server sees all $SEEN GPU(s)" || fail "nvidia-smi lists $SMI_GPUS GPU(s), server reports $SEEN"

step "2. Job bigger than the largest GPU is rejected"
MAX_MB=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | sort -n | tail -1 | tr -d ' ')
CODE=$(curl -s -o "$WORK/reject.json" -w '%{http_code}' -X POST "$URL/jobs" -H 'Content-Type: application/json' \
  -d "{\"command\":\"true\",\"memory_mb\":$((MAX_MB + 1000))}")
[ "$CODE" = 400 ] && pass "HTTP 400: $(cat "$WORK/reject.json")" || fail "expected HTTP 400, got $CODE"

step "3. GPU pinning (job sees only its own GPU)"
rm -f "$WORK/pin.txt"
ID=$(submit "$PY -c \"import os,torch; print(os.environ.get('CUDA_VISIBLE_DEVICES'), torch.cuda.device_count(), torch.cuda.get_device_name(0))\" > $WORK/pin.txt" 2000)
ST=$(wait_job "$ID" 120)
echo "        job saw: $(cat "$WORK/pin.txt" 2>/dev/null)"
read -r PIN_ENV PIN_COUNT _ <"$WORK/pin.txt" 2>/dev/null || true
if [ "$ST" = success ] && [[ ${PIN_ENV:-} == GPU-* ]] && [ "${PIN_COUNT:-}" = 1 ]; then
  pass "CUDA_VISIBLE_DEVICES=$PIN_ENV, job sees exactly 1 GPU"
else
  fail "status=$ST error=$(job_error "$ID")"
fi

step "4. Can nvidia-smi see per-process GPU memory here? (memory kill depends on it)"
ID=$(submit "$PY $WORK/alloc.py 3 15" 8000)
sleep 8
APPS=$(nvidia-smi --query-compute-apps=pid,used_memory --format=csv,noheader,nounits)
echo "        nvidia-smi compute-apps: ${APPS:-<empty>}"
VISIBLE=0
while IFS=, read -r pid _; do
  pid=${pid// /}
  [ -n "$pid" ] && ps -p "$pid" -o args= 2>/dev/null | grep -q alloc.py && VISIBLE=1
done <<<"$APPS"
if [ "$VISIBLE" = 1 ]; then
  pass "job's process and its GPU memory are visible"
else
  fail "nvidia-smi does not report this container's processes (PID namespace); memory enforcement cannot work here"
fi

step "5. Job under its memory request succeeds (3 GB used, 8000 MB requested)"
ST=$(wait_job "$ID" 60)
[ "$ST" = success ] && pass "succeeded" || fail "status=$ST error=$(job_error "$ID")"

step "6. Job over its memory request is killed (6 GB used, 2000 MB requested)"
START=$(date +%s)
ID=$(submit "$PY $WORK/alloc.py 6 40" 2000)
ST=$(wait_job "$ID" 90)
ERR=$(job_error "$ID")
echo "        status=$ST after $(($(date +%s) - START))s, error: ${ERR:-<none>}"
[ "$ST" = failed ] && [[ $ERR == *"GPU memory exceeded"* ]] && pass "killed for exceeding memory" || fail "expected failed with 'GPU memory exceeded'"

step "7. Load test (30 jobs, 4-20 GB requests)"
if "$WORK/loadtest" -server "$URL" -jobs 30 -large 0 -min-run 1s -max-run 3s; then
  pass "load test finished"
else
  fail "load test failed"
fi
echo "        Redis after: pending=$(redis LLEN gpu-runner:jobs:pending) processing=$(redis LLEN gpu-runner:jobs:processing)"

# ---------------------------------------------------------------- summary --
step "Summary: $PASS passed, $FAIL failed"
echo "  GPU:    $(nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader)"
echo "  Server still running (pid $(cat "$WORK/server.pid")) on $URL"
echo "  Stop it with: kill \$(cat $WORK/server.pid)"
[ "$FAIL" = 0 ]
