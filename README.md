# gpu-runner

A job runner service that executes shell commands with configurable storage volumes, retry logic, and Redis-backed queuing.

## Architecture

- **HTTP API** (gorilla/mux) for job submission, listing, status, and cancellation
- **Redis** for job queuing (pending/processing lists) and structured log streaming
- **SQLite** for persistent job storage
- **Worker pool** (3 workers) that dequeue and execute jobs with 30s timeouts

## Prerequisites

- Go 1.25+
- Docker & Docker Compose
- Redis (provided via Docker Compose)

## Getting Started

```bash
# Start the full stack (server + Redis)
make docker-up

# Or run locally (requires Redis on localhost:6379)
make build
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
