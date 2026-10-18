# gpu-runner

A job runner that executes shell commands on CPU workers or on NVIDIA GPUs. Jobs are queued in Redis, stored in SQLite, retried on failure, and, in GPU mode, scheduled onto GPUs by free memory, with a hard limit on how much GPU memory each job may use.

## How it works

```
gpucli / HTTP ─► API ─► Redis (pending → processing) ─► Redis adapter ─┐
                  │                                                     │
                  └─ SQLite job store ◄── acknowledger ◄── results ◄────┤
                                                                        │
  CPU mode (GPU_ENABLED=false):  WORKER_COUNT workers share one in-memory queue
  GPU mode (GPU_ENABLED=true):   queue (FIFO) ─► dispatcher ─► one worker per GPU
                                                     ▲
                                 control plane ── polls nvidia-smi
```

- **API** (`internal/api`): gorilla/mux HTTP server for submitting, listing, inspecting, and cancelling jobs, plus `/health` and Prometheus `/metrics`.
- **Redis** (`internal/redis`): the `gpu-runner:jobs:pending` and `gpu-runner:jobs:processing` lists, and one log stream per job (`gpu-runner:logs:<id>`).
- **SQLite** (`internal/store`): persistent job records.
- **Workers and executor** (`internal/jobs`, `internal/executer`): run each job as `bash -c` in its own process group, inside its storage volume, with a timeout.
- **Acknowledger**: writes each result to SQLite, re-enqueues failed jobs until `max_retries`, and removes finished jobs from the processing list.
- **Crash recovery**: at startup, jobs left in the processing list by a previous run are moved back to pending.

### GPU mode

When `GPU_ENABLED=true`:

- **Control plane** (`internal/controlplane`) discovers GPUs with `nvidia-smi` at startup (startup fails if it can't), then polls every `GPU_POLL_INTERVAL` for health and free memory. A GPU whose row is missing or doesn't parse is marked unhealthy.
- **Queue** (`internal/queue`) holds waiting jobs and tries them in FIFO order. A job that doesn't fit anywhere yet is skipped, so smaller jobs behind it can still run.
- **Dispatcher** offers idle, healthy GPUs with at least `memory_mb` free. Among those, the smallest GPU is tried first, so large GPUs stay free for large jobs.
- **One worker per GPU.** Each job sees only its GPU, through `CUDA_VISIBLE_DEVICES=<gpu uuid>`.
- **Fail fast:** a job asking for more memory than the largest GPU is rejected with a 400 when submitted.
- **Memory enforcement:** every `GPU_MEMORY_CHECK_INTERVAL`, a running job's GPU memory is measured per process group (`nvidia-smi --query-compute-apps`). If it stays above `memory_mb` for `GPU_MEMORY_GRACE_CHECKS` checks in a row, the whole process group is killed, and the job fails with an `exceeded memory request` error and is **not retried**.

Frameworks that reserve GPU memory up front (JAX takes ~75% by default, and PyTorch's caching allocator holds on to freed memory) must be capped (for example with `XLA_PYTHON_CLIENT_MEM_FRACTION`) or given a large enough `memory_mb`. Otherwise they will be killed.

See [PLAN.md](PLAN.md) for the design and the roadmap (priorities, runtime estimates, multi-node).

## Prerequisites

- Go 1.25+
- Docker and Docker Compose
- Redis (included in Docker Compose)
- For GPU mode: NVIDIA drivers and `nvidia-smi` on the host

## Getting started

### Docker Compose

```bash
cp .env.example .env   # adjust as needed
make docker-up         # server + Redis, at http://localhost:8080
```

### Local development

```bash
redis-server &          # or: docker compose up -d redis
make build              # builds bin/server and bin/gpucli

export REDIS_ADDRESS=localhost:6379
export DATABASE_PATH=./jobs.db
export GPU_ENABLED=true # optional, needs nvidia-smi
./bin/server
```

## CLI

The CLI talks to `GPU_RUNNER_SERVER` (default `http://0.0.0.0:8080`), or to `--server`.

```bash
# Submit a job (--cmd and --memory are required)
gpucli submit --cmd "python train.py" --memory 8000 --storage 10485760 --maxRetries 3

# List jobs, optionally by status (pending, running, success, failed, cancelled)
gpucli list
gpucli list --status running

# Show a job, including its logs
gpucli status <job-id>

# Cancel a job
gpucli cancel --id <job-id> --reason "no longer needed"

# Shut the server down (sends SHUTDOWN_TOKEN as a bearer token)
gpucli shutdown --reason "maintenance"

# Menu-driven mode
gpucli interactive
```

## API

| Method | Path            | Description                                                 |
|--------|-----------------|-------------------------------------------------------------|
| POST   | `/jobs`         | Create a job                                                |
| GET    | `/jobs`         | List jobs (`?status=` to filter)                            |
| GET    | `/jobs/{id}`    | Get a job, with its logs                                    |
| POST   | `/endjobs/{id}` | Cancel a job (body: `{"reason": "..."}`)                    |
| GET    | `/health`       | Server and Redis status                                     |
| GET    | `/metrics`      | Prometheus metrics                                          |
| DELETE | `/server`       | Graceful shutdown (`Authorization: Bearer $SHUTDOWN_TOKEN`) |

### Create a job

```json
POST /jobs
{
  "command": "python train.py",
  "memory_mb": 8000,
  "storage": 10485760,
  "max_retries": 3
}
```

| Field         | Required | Notes                                                                          |
|---------------|----------|--------------------------------------------------------------------------------|
| `command`     | yes      | Run with `bash -c`                                                             |
| `memory_mb`   | yes      | Must be positive. In GPU mode this is both a placement requirement and an enforced limit. |
| `storage`     | no       | Bytes, rounded up to a storage tier (10, 25, or 50 MB). More than 50 MB is rejected. |
| `max_retries` | no       | Default `3`                                                                    |

A failed job's reason is returned in its `error` field.

## Storage volumes

Each job runs with its working directory set to its tier's volume.

| Tier  | Bytes    | Default path                      |
|-------|----------|-----------------------------------|
| 10 MB | 10485760 | `/var/lib/jobrunner/volumes/10mb` |
| 25 MB | 26214400 | `/var/lib/jobrunner/volumes/25mb` |
| 50 MB | 52428800 | `/var/lib/jobrunner/volumes/50mb` |

## Metrics

Besides the Go and process metrics, GPU mode exports:

| Metric                                      | Labels         | Meaning                                |
|---------------------------------------------|----------------|----------------------------------------|
| `gpu_runner_gpu_memory_free_mb`             | `gpu`, `model` | Free memory as of the last poll        |
| `gpu_runner_gpu_memory_total_mb`            | `gpu`, `model` | Total memory                           |
| `gpu_runner_gpu_healthy`                    | `gpu`          | 1 if healthy at the last poll          |
| `gpu_runner_gpu_busy`                       | `gpu`          | 1 while running a job                  |
| `gpu_runner_gpu_jobs_placed_total`          | `gpu`          | Jobs placed on each GPU                |
| `gpu_runner_nvidia_smi_poll_seconds`        |                | Duration of each health poll           |
| `gpu_runner_nvidia_smi_poll_failures_total` |                | Health polls that failed or timed out  |

## Configuration

All settings are environment variables. See [.env.example](.env.example) for the full list.

| Variable                    | Default                    | Description                                         |
|-----------------------------|----------------------------|-----------------------------------------------------|
| `SERVER_ADDRESS`            | `0.0.0.0`                  | Bind address                                        |
| `SERVER_PORT`               | `8080`                     | Port                                                |
| `SHUTDOWN_TOKEN`            |                            | Bearer token for `DELETE /server`                   |
| `REDIS_ADDRESS`             | `redis:6379`               | Redis host:port                                     |
| `REDIS_PASSWORD`            |                            | Redis password                                      |
| `REDIS_DB`                  | `0`                        | Redis database                                      |
| `REDIS_DIAL_TIMEOUT`        | `5s`                       | Connect timeout                                     |
| `REDIS_READ_TIMEOUT`        | `3s`                       | Read timeout                                        |
| `REDIS_WRITE_TIMEOUT`       | `3s`                       | Write timeout                                       |
| `REDIS_MAX_RETRIES`         | `3`                        | Redis command retries                               |
| `DATABASE_PATH`             | `/data/jobs.db`            | SQLite file                                         |
| `WORKER_COUNT`              | `3`                        | CPU workers (CPU mode only)                         |
| `WORKER_JOB_TIMEOUT`        | `30s`                      | Per-job timeout (both modes)                        |
| `WORKER_QUEUE_CAPACITY`     | `10`                       | In-memory queue capacity                            |
| `WORKER_RESULTS_BUFFER`     | `100`                      | Results channel buffer                              |
| `STORAGE_VOLUME_{10,25,50}MB_PATH` | see above           | Storage tier paths                                  |
| `GPU_ENABLED`               | `false`                    | Turn on GPU scheduling                              |
| `NVIDIA_SMI_PATH`           | `nvidia-smi`               | Path to `nvidia-smi`                                |
| `GPU_POLL_INTERVAL`         | `10s`                      | GPU health and free-memory poll (min `1s`)          |
| `GPU_MEMORY_CHECK_INTERVAL` | `2s`                       | Per-job memory check (min `100ms`)                  |
| `GPU_MEMORY_GRACE_CHECKS`   | `2`                        | Consecutive over-limit checks before a kill (min 1) |
| `LOG_LEVEL`                 | `info`                     | `debug`, `info`, `warn`, `error`                    |
| `LOG_DIR`                   | `~/log/gpu-runner`         | Server log directory                                |
| `LOG_FILE`                  | `server.log`               | Server log file                                     |
| `GPU_RUNNER_SERVER`         | `http://0.0.0.0:8080`      | Server URL used by the CLI                          |

## Development

```bash
make build         # build bin/server and bin/gpucli
make test          # go test ./...
make lint          # golangci-lint
make fmt           # gofmt
make docker-build  # build images
make docker-up     # start containers
make docker-down   # stop containers
```

Tests never call the real `nvidia-smi`; the control plane, dispatcher, and memory enforcement are tested with fakes. CI (`.github/workflows/ci.yaml`) runs lint, tests, and a Docker build on every push and pull request to `main`.

More example jobs are in [COMPLEX_JOBS_EXAMPLES.md](COMPLEX_JOBS_EXAMPLES.md).
