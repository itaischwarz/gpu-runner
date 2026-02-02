# gpu-runner

A job runner service that executes shell commands with configurable storage volumes, retry logic, and Redis-backed queuing.

## Architecture

- **HTTP API** (gorilla/mux) for job submission, listing, status, and cancellation
- **Redis** for job queuing (pending/processing lists) and structured log streaming
- **SQLite** for persistent job storage
- **Worker pool** (configurable) that dequeue and execute jobs with configurable timeouts
- **Fully configurable** via environment variables

## Features

- Job submission, queuing, execution, and cancellation
- Automatic retry logic with configurable max retries
- Storage volume tiers (10MB, 25MB, 50MB)
- Real-time structured logging via Redis Streams
- Graceful shutdown handling
- Environment variable configuration for all settings
- Docker Compose deployment ready

## Prerequisites

- Go 1.25+
- Docker & Docker Compose
- Redis (provided via Docker Compose)

## Getting Started

### Quick Start with Docker Compose

```bash
# Copy environment file and customize as needed
cp .env.example .env

# Start the full stack (server + Redis)
make docker-up

# The server will be available at http://localhost:8080
```

### Local Development

```bash
# Ensure Redis is running locally
redis-server

# Build binaries
make build

# Set environment variables (optional)
export SERVER_PORT=8080
export REDIS_ADDRESS=localhost:6379
export WORKER_COUNT=5

# Run server
./bin/server
```

## CLI Usage

```bash
# Submit a job
./bin/gpucli submit --cmd "echo hello" --storage 10485760 --maxRetries 3

# List all jobs
./bin/gpucli list

# List by status
./bin/gpucli list --status pending

# Check job status
./bin/gpucli status <job-id>

# Cancel a job
./bin/gpucli cancel --id <job-id> --reason "no longer needed"
```

## API Endpoints

| Method | Path             | Description        |
|--------|------------------|--------------------|
| POST   | `/jobs`          | Create a new job   |
| GET    | `/jobs`          | List jobs          |
| GET    | `/jobs/{id}`     | Get job details    |
| POST   | `/endjobs/{id}`  | Cancel a job       |

### Create Job

```json
POST /jobs
{
  "command": "echo hello",
  "storage": 10485760,
  "max_retries": 3
}
```

Storage is auto-adjusted to the nearest tier: 10MB, 25MB, or 50MB. Default `max_retries` is 3.

## Storage Volumes

| Tier  | Bytes      | Path                              |
|-------|------------|-----------------------------------|
| 10MB  | 10485760   | `/var/lib/jobrunner/volumes/10mb` |
| 25MB  | 26214400   | `/var/lib/jobrunner/volumes/25mb` |
| 50MB  | 52428800   | `/var/lib/jobrunner/volumes/50mb` |

## Makefile Targets

```bash
make build         # Build server and CLI binaries
make test          # Run all tests
make lint          # Run golangci-lint
make fmt           # Format Go source files
make docker-build  # Build Docker images
make docker-up     # Start containers
make docker-down   # Stop containers
```

## Running Tests

```bash
make test
```

Tests cover the store layer (SQLite CRUD), executor (command execution, cancellation, timeouts), API handlers (HTTP request/response), and job model/queue logic.

## Configuration

All configuration is done via environment variables. See [.env.example](.env.example) for a complete reference.

### Key Configuration Options

#### Server Configuration
- `SERVER_ADDRESS` - Server bind address (default: `0.0.0.0`)
- `SERVER_PORT` - Server port (default: `8080`)

#### Redis Configuration
- `REDIS_ADDRESS` - Redis server address (default: `redis:6379`)
- `REDIS_PASSWORD` - Redis password (default: empty)
- `REDIS_DB` - Redis database number (default: `0`)
- `REDIS_DIAL_TIMEOUT` - Connection timeout (default: `5s`)
- `REDIS_READ_TIMEOUT` - Read timeout (default: `3s`)
- `REDIS_WRITE_TIMEOUT` - Write timeout (default: `3s`)
- `REDIS_MAX_RETRIES` - Max retry attempts (default: `3`)

#### Database Configuration
- `DATABASE_PATH` - SQLite database path (default: `/data/jobs.db`)

#### Worker Configuration
- `WORKER_COUNT` - Number of concurrent workers (default: `3`)
- `WORKER_JOB_TIMEOUT` - Job execution timeout (default: `30s`)
- `WORKER_QUEUE_CAPACITY` - Internal queue capacity (default: `10`)
- `WORKER_RESULTS_BUFFER` - Results channel buffer (default: `100`)

#### Storage Configuration
- `STORAGE_VOLUME_10MB_PATH` - Path for 10MB tier (default: `/var/lib/jobrunner/volumes/10mb`)
- `STORAGE_VOLUME_25MB_PATH` - Path for 25MB tier (default: `/var/lib/jobrunner/volumes/25mb`)
- `STORAGE_VOLUME_50MB_PATH` - Path for 50MB tier (default: `/var/lib/jobrunner/volumes/50mb`)

#### Logger Configuration
- `LOG_LEVEL` - Logging level: `debug`, `info`, `warn`, `error` (default: `info`)
- `LOG_DIR` - Log directory path (default: `~/log/gpu-runner`)
- `LOG_FILE` - Log file name (default: `server.log`)

#### CLI Configuration
- `GPU_RUNNER_SERVER` - Default server URL for CLI (default: `http://0.0.0.0:8080`)

### Configuration Examples

#### High-Throughput Setup
```bash
# .env file for high throughput
WORKER_COUNT=10
WORKER_JOB_TIMEOUT=5m
WORKER_QUEUE_CAPACITY=50
WORKER_RESULTS_BUFFER=200
LOG_LEVEL=warn
```

#### Development Setup
```bash
# .env file for development
SERVER_PORT=3000
REDIS_ADDRESS=localhost:6379
WORKER_COUNT=2
LOG_LEVEL=debug
```

#### Production Setup
```bash
# .env file for production
SERVER_ADDRESS=0.0.0.0
SERVER_PORT=8080
REDIS_ADDRESS=redis-cluster.production.svc:6379
REDIS_PASSWORD=secure_password_here
DATABASE_PATH=/mnt/persistent/jobs.db
WORKER_COUNT=8
WORKER_JOB_TIMEOUT=10m
LOG_LEVEL=info
LOG_DIR=/var/log/gpu-runner
```

## Testing Complex Jobs

For a comprehensive list of complex job examples to test the runner, see [COMPLEX_JOBS_EXAMPLES.md](COMPLEX_JOBS_EXAMPLES.md).

Example complex job submissions:

```bash
# Data processing pipeline
gpucli submit --cmd "seq 1 100 > raw.txt && awk '{print \$1*2}' raw.txt > transformed.txt && cat transformed.txt | awk '{sum+=\$1} END {print sum}'" --storage 10485760

# Concurrent processing
gpucli submit --cmd "for i in {1..10}; do (echo 'Processing '\$i && sleep 1) & done; wait && echo 'Done'" --storage 10485760

# Resource-intensive job
gpucli submit --cmd "python3 -c 'import random; m = [[random.random() for _ in range(100)] for _ in range(100)]; print(sum(sum(r) for r in m))'" --storage 10485760

# Job with retry logic
gpucli submit --cmd "if [ \$((RANDOM % 2)) -eq 0 ]; then exit 0; else exit 1; fi" --storage 10485760 --maxRetries 5
```

## Deployment

### Docker Compose Production Deployment

1. Create a `.env` file with production settings
2. Customize volume paths in `docker-compose.yml` if needed
3. Deploy:

```bash
docker-compose up -d
```

### Kubernetes Deployment

For Kubernetes deployment, you'll need to create:
- `ConfigMap` or `Secret` for environment variables
- `Deployment` for the server
- `Service` to expose the HTTP API
- `StatefulSet` for Redis (or use managed Redis)
- `PersistentVolumeClaim` for database and job volumes

Example partial manifest:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gpu-runner-config
data:
  SERVER_PORT: "8080"
  REDIS_ADDRESS: "redis-service:6379"
  WORKER_COUNT: "5"
  WORKER_JOB_TIMEOUT: "60s"
  LOG_LEVEL: "info"
```
