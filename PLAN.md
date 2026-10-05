# Control Plane & GPU Layer: Draft Proposal

## Goal

Add GPU execution as a **separate infrastructure** next to the existing CPU pipeline.

**Separate infrastructure.** CPU and GPU are two independent pools. Each has its own Redis lists, workers, and slots, and a job runs on exactly one of them.

**Shared logic.** The scheduler, worker, handler, retry, and acknowledgement code is identical for both pools. The infrastructure is a selection choice: a job's `Resource` field picks a pool, and each pool is the same control plane with a different **provider** plugged in. Everything specific to CPU or GPU lives in the provider.

- **v1 (this plan):** one control plane per pool and one worker per slot. A slot is a GPU, or a CPU worker slot. Jobs go to the slot expected to start them soonest, and each slot's queue is ordered by `JobPriority`. `FreeMemoryMB` decides whether a job can start.
- **v2 (later):** workers are no longer tied to a slot, and several jobs can share one slot.

The GPU's hardware state comes from `nvidia-smi`. The control plane keeps only the scheduling state it needs: queues, the running job, and runtime estimates. Job and worker metrics stay in `Worker`.

## Decisions so far

| Question | Decision |
|---|---|
| Do CPU and GPU jobs share infrastructure? | No. They have separate pools, Redis lists, and workers. A job is either `cpu` or `gpu`. |
| Do CPU and GPU have different logic? | No. One control plane implementation, one worker, and one handler path. Only the `Provider` differs. |
| Who polls for health? | One shared poller per pool, calling `Provider.Poll`. For GPU, that is one `nvidia-smi` call for every GPU. |
| How many workers per GPU? | Exactly one in v1 (one per slot). v2 removes the binding. |
| Does `FreeMemoryMB` affect scheduling? | Yes. A job starts on a slot only if `FreeMemoryMB >= job.MemoryMB`. |
| What happens when a large job can't run yet? | It waits in the queue of the slot expected to fit it soonest. Smaller jobs keep flowing to the other slots. |
| Is priority used? | Yes. `JobPriority` orders each slot's queue, with aging so low-priority jobs don't starve. |
| Persist the new fields in SQLite? | Yes: `resource`, `priority`, `memory_mb`, `estimated_duration_s`, and `assigned_slot`. |
| Best fit or most free memory? | Not decided yet. A research spike comes first (see below). |
| Is `memory_mb` enforced? | Yes, in v1, even though each GPU runs only one job. Leaving it unset means the job gets a whole GPU and is never killed. Setting it is a contract: sustained use above it kills the job. See "Memory requests and enforcement". |

## Architecture

```
                         ┌─ resource=cpu ─► redis jobs:cpu:pending ─► Pool[cpu] ─┐
POST /jobs ─► handler ───┤                                                       │   same code for both pools:
                         └─ resource=gpu ─► redis jobs:gpu:pending ─► Pool[gpu] ─┤   ControlPlane + Workers
                                                                                 │
  Pool = ControlPlane{ Provider } ─► per-slot queues ─► worker[slot] ─► Executor (env = Provider.Env(slot))
                │
                └─ poller ─► Provider.Poll()   cpu: CPUProvider (WORKER_COUNT slots)
                                               gpu: GPUProvider (nvidia-smi, one slot per GPU)
```

| Package | Responsibility | Depends on the infrastructure? |
|---|---|---|
| `internal/controlplane` (new) | `Slot`, `Provider`, per-slot queues, placement, priority policy, runtime estimates, poller, work stealing | No |
| `internal/controlplane/gpu` (new) | `GPUDevice`, the `nvidia-smi` query and parser, and `GPUProvider` | **Yes**, GPU only |
| `internal/controlplane/cpu` (new) | `CPUProvider` | **Yes**, CPU only |
| `internal/jobs` | `Worker` runs whatever arrives on its inbox | No |
| `internal/redis` | Queue, acknowledgement, and retry, with keys built from `Resource` | No |
| `internal/api` | One `CreateJob` path. It checks that the requested pool exists. | No |

## Provider interface (the only infrastructure-specific code)

```go
package controlplane

type Slot struct {
    ID            string `json:"id"`              // GPU UUID, or "cpu-1"
    Model         string `json:"model"`
    TotalMemoryMB int    `json:"total_memory_mb"`
    FreeMemoryMB  int    `json:"free_memory_mb"`
    Healthy       bool   `json:"healthy"`
}

type Provider interface {
    Resource() jobs.JobResource                 // "cpu" | "gpu"
    Poll(ctx context.Context) ([]Slot, error)   // current state of every slot
    Env(slotID string) []string                 // extra env so a job only sees its slot
}
```

The scheduler treats both providers' slots the same way. A CPU slot behaves like a GPU that is always healthy and has unlimited memory, so every rule below works for both.

### GPUProvider (`internal/controlplane/gpu`)

```go
type GPUDevice struct {
    UUID          string `json:"uuid"`
    Model         string `json:"model"`
    TotalMemoryMB int    `json:"total_memory_mb"`
    FreeMemoryMB  int    `json:"free_memory_mb"`
    Healthy       bool   `json:"healthy"`
}
```

Every field comes from a single call:

```
nvidia-smi --query-gpu=uuid,name,memory.total,memory.free --format=csv,noheader,nounits
```

| Field | nvidia-smi field |
|---|---|
| `UUID` | `uuid` |
| `Model` | `name` |
| `TotalMemoryMB` | `memory.total` |
| `FreeMemoryMB` | `memory.free` |
| `Healthy` | derived (see below) |

- **Poll** runs the query with a timeout and maps each `GPUDevice` to a `Slot`, using the UUID as the slot ID.
- **Healthy** means the GPU's row is present and every field in it parses. A value like `[GPU requires reset]` marks that GPU unhealthy. If `nvidia-smi` fails or times out, every GPU is marked unhealthy. If a GPU found at startup is missing from a later poll, it is marked unhealthy.
- **Env** returns `CUDA_VISIBLE_DEVICES=<uuid>`. The UUID is used rather than the index because indices can change order between driver reloads.

### CPUProvider (`internal/controlplane/cpu`)

- **Poll** returns `WORKER_COUNT` slots, `cpu-1` to `cpu-N`. They are always healthy, and their memory is effectively unlimited (`math.MaxInt`).
- **Env** returns `nil`.
- Later, this provider could report real host memory or CPU load without any change to the scheduler.

## Job changes (`internal/jobs/job.go`)

```go
Resource          JobResource `json:"resource"`             // "cpu" (default) | "gpu"
Priority          JobPriority `json:"priority"`             // type already exists in values.go, but not on Job yet
MemoryMB          int         `json:"memory_mb"`            // 0 = whole GPU, not enforced; >0 = enforced limitEstimatedDuration int         `json:"estimated_duration_s"` // optional user hint
AssignedSlot      string      `json:"assigned_slot"`        // set by the control plane
```

The fields mean the same thing for both pools, so the handler validates them all in one way:
- A `resource` with no running pool (for example `gpu` when `GPU_ENABLED=false`) is rejected with a 400.
- `priority` defaults to `medium`.
- `memory_mb` is checked against the pool's largest slot (see "Fail fast" below).

The CLI gets `--resource`, `--memory`, `--priority`, and `--estimate` flags. The interactive priority prompt already exists in `ui.PromptForPriority`.

## Memory requests and enforcement (v1)

**Why enforce in v1** even though each GPU runs one job: a request that isn't enforced is only a guess. If v1 records guesses, v2 can't rely on them to share GPUs. Enforcing from the start means every `memory_mb` in the system is a limit that held.

### Two kinds of job

| `memory_mb` | Meaning | Placement | Enforced? |
|---|---|---|---|
| Not set (0) | **Exclusive.** The whole GPU is yours. | Any healthy, idle GPU | No; there's no limit to exceed |
| Set | **Contract.** We reserve exactly this much. | A healthy, idle GPU with `FreeMemoryMB >= memory_mb` | Yes: sustained use above it kills the job |

Users who don't know their number leave it unset, which is always safe. Memory settings are always user-defined; the system doesn't record observed usage or suggest values.

### Measuring per job, not per GPU

The health poll reports total GPU usage, which includes any process on the card that isn't ours. Killing a job based on that number could kill a job that did nothing wrong. So enforcement uses per-process numbers:

```
nvidia-smi --query-compute-apps=pid,gpu_uuid,used_memory --format=csv,noheader,nounits
```

- **Process group per job:** the executor starts each job in its own process group (`SysProcAttr{Setpgid: true}`). The process actually using the GPU is often a child of `bash -c`, so matching on the bash PID alone would miss it.
- **Attribution:** a compute process belongs to the job if `getpgid(pid)` equals the job's process group. Job usage is the sum of `used_memory` across its processes on its assigned GPU.
### Enforcement rule

- **Check frequency:** every `MEMORY_CHECK_INTERVAL` (default `2s`), only for running jobs with `memory_mb > 0`. This is faster than the 10s health poll, so a breach is noticed quickly.
- **Kill condition:** usage above `memory_mb` for `MEMORY_GRACE_CHECKS` consecutive checks (default `2`). A single momentary spike doesn't kill the job.
- **How the kill works:** cancel the job's context, and signal the whole process group (`kill(-pgid, SIGKILL)`), so the job's child processes die too and the GPU memory is actually freed.
- **Outcome:** status `failed` with reason `exceeded memory request: used X MB, requested Y MB`. The job is marked **non-retryable**, because retrying with the same request would fail the same way.

### Who does what

| Piece | Responsibility |
|---|---|
| `controlplane/device.go` | `QueryComputeApps`: the per-process `nvidia-smi` query and its parser |
| Executor | Starts each job in its own process group, exposes the job's PGID, and can kill the whole group |
| Dispatcher | Knows which job runs on which GPU, so it runs the check loop and decides when to kill |

### Known limits

- **Polling can't stop a fast spike.** A job can briefly go over its limit between checks, and a spike can still cause an out-of-memory error for the job itself. In v1 that only affects the job that spiked, since it has the GPU to itself. Hard isolation needs MIG (future work).
- **Frameworks that reserve memory up front:** JAX takes about 75% of the GPU at startup by default, and PyTorch's caching allocator keeps freed memory. A job using them must cap its framework (for example, `XLA_PYTHON_CLIENT_MEM_FRACTION` for JAX) or request enough memory. The docs and CLI help need to say this.
- **Containers:** `nvidia-smi` inside a container may show host PIDs, or no PIDs at all, which would break attribution. This must be checked on the target deployment. If PIDs aren't visible, an exclusive job still works, and an enforced job is rejected at startup with a clear error rather than run unenforced.

## Scheduling algorithm (shared by both pools)

Each slot in a control plane holds:

```go
type slotState struct {
    Slot    Slot           // latest Provider.Poll reading
    Running *running       // job, startedAt, estimate; nil when idle
    Queue   []*jobs.Job    // waiting jobs, kept in priority order
    Inbox   chan *jobs.Job // to the bound worker
}
```

### 1. Runtime estimate

`est(job)` is the first available of:
1. The job's `EstimatedDuration`.
2. A moving average of observed runtimes in this pool.
3. The pool's job timeout (cold start, the most conservative value).

The result is always capped at the pool's job timeout, because the job is killed at that point anyway.

### 2. Priority order inside a slot queue

```
effective(job) = weight(job.Priority) + floor(waited / PRIORITY_AGING)
weight: highest=4, high=3, medium=2, low=1
```

Jobs are sorted by `effective` (highest first). Ties go to the earlier `CreatedAt`. Aging means a `low` job that has waited 3 × `PRIORITY_AGING` ranks like a `highest` job, so nothing starves. v1 has no preemption: a running job is never interrupted.

### 3. Placement: choosing a slot queue for a new job

```
candidates = slots where Healthy && TotalMemoryMB >= job.MemoryMB

ETA(s, job) = remaining(s.Running)                                  // max(est - elapsed, 0)
            + Σ est(k) for k in s.Queue where effective(k) >= effective(job)
              (only the jobs that would be ahead of this one)

place job on the candidate with the lowest ETA
```

- **Tie-break:** pluggable. It defaults to most free memory until the research spike decides.
- **Fail fast:** if no healthy slot is large enough, the job fails with `reason: exceeds pool capacity` and is marked **non-retryable**, so the acknowledger doesn't re-enqueue it.
- Because ETA only counts jobs ranked ahead of the new job, a `highest` job sees almost empty queues everywhere and goes to whichever slot frees up first.

### 4. Start rule: when the head of a queue runs

A job runs when the slot is healthy and idle, and its `FreeMemoryMB` is at least the job's `MemoryMB`. Otherwise the job waits. The control plane re-checks after every poll and every job completion.

### 5. Work stealing: fixing estimates that turned out wrong

When a slot goes idle with an empty queue, it takes the highest-ranked waiting job from any other slot's queue, provided that job fits its current `FreeMemoryMB`. This corrects placements that were based on bad runtime estimates.

### 6. Failures and completion

- **A slot turns unhealthy:** its running job continues. Its queued jobs are placed again using step 3.
- **A job finishes:** the worker sends the result to `Results` and calls `release()`. The control plane records the runtime for the moving average, triggers an immediate `Provider.Poll` so the freed memory shows up, and then runs the start rule.

### Worked example (GPU pool)

Three GPUs: A has 80 GB, and B and C have 24 GB each. All start idle.

| Time | Event | Result |
|---|---|---|
| t0 | `L` (60 GB, est 30m) arrives | Only A fits, ETA 0. It runs on A. |
| t1 | `L2` (60 GB, est 30m) arrives | Only A fits, ETA about 30m. It waits in A's queue. |
| t2 | `S1`, `S2` (8 GB) arrive | ETA is 0 on B and C, and about 60m on A. They run on B and C. |
| t3 | `S3` (8 GB, est 5m) arrives | B has about 4m left, C about 5m, A about 60m. It waits in B's queue. |
| t4 | `U` (8 GB, `highest`) arrives | It ranks ahead of everything queued and goes to whichever GPU frees first. |
| t5 | C finishes, and its queue is empty | C takes `S3` from B's queue. |

The CPU pool runs the same steps. Since CPU slots never run out of memory, it behaves like a priority queue spread across `WORKER_COUNT` slots.

## Redis (`internal/redis`)

The queue code is shared, and the keys are built from `Resource`:

| | CPU | GPU |
|---|---|---|
| Pending | `gpu-runner:jobs:pending` (existing key, kept as-is) | `gpu-runner:jobs:gpu:pending` |
| Processing | `gpu-runner:jobs:processing` (existing key, kept as-is) | `gpu-runner:jobs:gpu:processing` |
| Adapter | `StartRedisAdapter(ctx, ResourceCPU, pool.Intake)` | `StartRedisAdapter(ctx, ResourceGPU, pool.Intake)` |

The CPU keys keep their current names, so CPU jobs already pending in Redis are not orphaned. `Enqueue`, `Acknowledge`, and retry choose keys from `job.Resource`, which means the acknowledger has no CPU- or GPU-specific branches.

## Worker changes (`internal/jobs/worker.go`)

```go
type Worker struct {
    ID       int
    JobQueue *JobQueue   // still used for the Executor
    Results  chan *Job
    Inbox    <-chan *Job // the slot's inbox from the control plane
    Env      []string    // Provider.Env(slot)
    OnIdle   func()      // release()
}
```

Every worker is wired the same way, whichever pool it belongs to. The worker does no health checks or scheduling. To pass the env through, the executor gets `RunJobWithEnv(..., extraEnv []string)`, and `RunJob` calls it with `nil`, so existing tests are unchanged. `jobs` does not import `controlplane`.

## Startup (`cmd/server/main.go`)

```go
providers := []controlplane.Provider{cpu.NewProvider(cfg.Worker.Count)}
if cfg.GPU.Enabled {
    providers = append(providers, gpu.NewProvider(cfg.GPU.NvidiaSMIPath)) // fatal if discovery fails
}
for _, p := range providers {
    pool := controlplane.New(p, cfg.Scheduler, results)
    slots, err := pool.Discover(ctx)
    for i, s := range slots {
        inbox, release := pool.Register(s.ID)
        w := jobs.NewWorker(i+1, jobQueue, results)
        w.Inbox, w.Env, w.OnIdle = inbox, p.Env(s.ID), release
        w.Start(ctx)
    }
    client.StartRedisAdapter(ctx, p.Resource(), pool.Intake())
    pool.Start(ctx)
    pools[p.Resource()] = pool // handed to the handlers
}
```

Both pools run at the same time, and adding another infrastructure later means adding another provider.

## Persistence (`internal/store`)

New columns: `resource`, `priority`, `memory_mb`, `estimated_duration_s`, `assigned_slot`.

`CREATE TABLE IF NOT EXISTS` doesn't add columns to an existing database. So `NewJobStore` runs a small migration: it reads `PRAGMA table_info(jobs)` and runs `ALTER TABLE jobs ADD COLUMN ...` for each missing column, with defaults (`resource='cpu'`, `priority='medium'`, and `0` or `''` for the rest). Existing rows stay valid.

## ⚠ Existing issue that blocks this

`Client.Acknowledge` removes a job from the processing list with `LRem(..., json.Marshal(job))`. That only works if the job's JSON is **byte-for-byte identical** to what was pushed.

The worker already changes `Status` before the job is acknowledged, so acknowledgement probably fails today and the processing list keeps growing. Setting `AssignedSlot` would break it for certain.

Proposed fix, as its own small PR first: track processing entries by job ID. One option is to store the raw payload string on dequeue and `LRem` that exact string. A cleaner option is a Redis hash `processing:{id}`.

## Config (`internal/config`)

| Env var | Default | Scope |
|---|---|---|
| `WORKER_COUNT` | `3` | CPU pool slots (existing) |
| `GPU_ENABLED` | `false` | Turns the GPU pool on |
| `NVIDIA_SMI_PATH` | `nvidia-smi` | GPU provider |
| `POLL_INTERVAL` | `10s` | All pools |
| `PRIORITY_AGING` | `10m` | All pools |
| `WORKER_JOB_TIMEOUT` / `GPU_JOB_TIMEOUT` | `30s` / open question | Per pool |

## `/health`

The response gets one entry per pool. Each entry lists its slots: the `Slot` reading plus queue length, running job ID, and ETA. The status is `degraded` if any running pool has no healthy slot.

## Research spike: best fit vs. most free, and validating the algorithm

The tie-break, and later the whole v2 scoring function, should be chosen with data, not intuition. The plan:

1. **Prior art to review:**
   - Bin-packing heuristics: best fit vs. worst fit.
   - Kubernetes scheduler scoring: `MostAllocated` vs. `LeastAllocated`.
   - Slurm's multifactor priority and EASY backfilling.
   - Gandiva and Tiresias, two research schedulers for deep-learning GPU clusters.
2. **Simulation harness:** run the real `controlplane` package with a fake `Provider` and a fake clock. Feed it synthetic workloads: a mix of large and small jobs, mixed priorities, bursts, and runtime estimates that are deliberately wrong. Because the logic doesn't depend on the infrastructure, one harness covers both pools.
3. **Metrics:**
   - Mean and p95 wait time per priority.
   - Slot utilization.
   - Total time to finish the whole workload.
   - The longest wait of any large job (the starvation check).

## Testing

- **`controlplane`:** all tests use a fake `Provider` and a fake clock, and run once for both pools.
  - Placement picks the lowest ETA, and ETA only counts jobs ranked ahead.
  - Aging moves a `low` job up the queue over time.
  - The start rule waits until `FreeMemoryMB` is high enough.
  - An idle slot takes waiting work from another slot's queue.
  - Jobs queued on a slot that turns unhealthy are placed again.
  - A job too large for every slot fails as non-retryable.
- **`controlplane/gpu`:** parse tests for normal multi-GPU output, a malformed row, and an `[N/A]` / `[GPU requires reset]` row.
- **`controlplane/cpu`:** returns `WORKER_COUNT` healthy slots.
- **Store:** the migration adds missing columns to an existing database.
- **API:** a job for a disabled pool is rejected, and priority defaults to `medium`.
- **Executor:** `RunJobWithEnv` passes `CUDA_VISIBLE_DEVICES` through to the job.
- CI never calls the real `nvidia-smi`.

## Delivery order

1. Fix acknowledgement so it works by job ID (prerequisite).
2. New job fields, store migration, and API/CLI flags.
3. The `controlplane` package and the CPU provider. The CPU pipeline moves onto the control plane, and its behaviour should stay the same.
4. The GPU provider, the GPU Redis keys, and the `GPU_ENABLED` wiring.
5. The dispatcher, plus worker inboxes and `CUDA_VISIBLE_DEVICES`.
6. Memory enforcement: `QueryComputeApps`, process groups in the executor, and the dispatcher's check loop and kill.
7. Per-pool details in `/health`.
8. The research spike and simulation harness, then fix the tie-break.

## v2: workers not tied to slots

| v1 | v2 |
|---|---|
| Per-slot queue, one bound worker each | Per-slot queues stay, but any idle worker in the pool takes the next startable job and runs it with `Provider.Env(job.AssignedSlot)`. |
| One running job per slot | Several jobs per slot. Each slot tracks `ReservedMB`, and the start rule becomes `FreeMemoryMB - ReservedMB >= MemoryMB`. |
| No preemption | Optional: a `highest` job can checkpoint and requeue a `low` job. |
| One runtime average per pool | Runtime averages per command or user. |

All of these changes are in the shared control plane, so both pools get them at the same time.

## Future work

- Per-slot access policies (which users or jobs a slot accepts)
- Temperature, utilization, and ECC-error health thresholds (GPU provider only)
- Backfill on a slot while its queue's head waits for memory
- Requeuing a running job when its slot fails
- Hot-plugged GPUs

## Open questions

1. **Priority weights and aging:** are `4/3/2/1` and a 10-minute aging step reasonable starting points? Should `highest` get its own lane?
2. **The cold-start estimate:** falling back to the job timeout makes the scheduler pessimistic until it has observed some jobs. Should `estimated_duration_s` be required for GPU jobs?
3. **The GPU job timeout:** GPU jobs usually run much longer than the 30s default. Should there be a separate `GPU_JOB_TIMEOUT`?
