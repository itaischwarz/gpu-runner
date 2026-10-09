# Real-GPU test plan

Goal: prove gpu-runner works on real GPUs, not just the fake `nvidia-smi`.

**Where things run:** the server runs **on the GPU box**, because it needs that
box's `nvidia-smi` and GPU. Your Mac just SSHes in. The `-L ...` part
of the SSH command lets your Mac reach the box's server at
`http://localhost:18080` (box A) or `http://localhost:18081` (box B).

Boxes (vast.ai, one GPU each, billing while on):

| Box | GPU | SSH |
|---|---|---|
| A | RTX 4090, 48 GB | `ssh -p 32869 root@23.158.136.85 -L 18080:localhost:18080` |
| B | RTX 5090, 32 GB | `ssh -p 59015 root@47.186.21.5 -L 18081:localhost:18080` |

---

## Step 1: push the scripts (on your Mac, once)

The boxes download the code from GitHub, so the scripts must be pushed first.

```bash
cd ~/Projects/gpu-runner
git add scripts
git commit -m "test: load test and real-GPU test scripts"
git push -u origin itai/gpu-test
```

(Or tell Claude "push it".)

## Step 2: run on box A

```bash
ssh -p 32869 root@23.158.136.85 -L 18080:localhost:18080
```

Then, on the box:

```bash
git clone -b itai/gpu-test https://github.com/itaischwarz/gpu-runner
cd gpu-runner && bash scripts/gputest/run.sh
```

This takes about 2–3 minutes (installing Go and Redis, then the checks). Copy
everything it prints.

## Step 3: run on box B

Open a new Mac terminal tab (box A can stay open; each box uses its own Mac port).

```bash
ssh -p 59015 root@47.186.21.5 -L 18081:localhost:18080
```

Same two commands:

```bash
git clone -b itai/gpu-test https://github.com/itaischwarz/gpu-runner
cd gpu-runner && bash scripts/gputest/run.sh
```

## Step 4: paste both outputs to Claude

## Step 5: stop both boxes in the vast.ai console

They charge while they're running.

---

## What the script checks

| # | Check | Pass means |
|---|---|---|
| 1 | GPU discovery | The server found the real GPU through `nvidia-smi` |
| 2 | Oversized job | A job bigger than the GPU gets HTTP 400 |
| 3 | Pinning | A GPU job sees only its assigned GPU (`CUDA_VISIBLE_DEVICES`) |
| 4 | Process visibility | `nvidia-smi` can see how much memory each job uses |
| 5 | Under request | A job using 3 GB with 8000 MB requested succeeds |
| 6 | Over request | A job using 6 GB with 2000 MB requested gets killed with "GPU memory exceeded" |
| 7 | Load test | 30 jobs run on the real GPU, with none lost |

**Expected:** checks 4 and 6 may fail. vast.ai boxes are Docker containers, and
containers usually hide process IDs from `nvidia-smi`. If that happens, the
memory kill can't see any job's usage. That's a limit of the environment, not a
bug in the code: enforcement then needs a VM, or a container that shares the
host's process IDs. Everything else should pass.

## Optional, while the server is running

From your Mac, with the SSH session still open:

- `http://localhost:18080/metrics` (18081 for box B): live GPU free memory, health and busy state.
- `curl -X POST localhost:18080/jobs -d '{"command":"nvidia-smi","memory_mb":1000}'`: submit a job yourself.

To stop the server on the box: `kill $(cat /root/gputest/server.pid)`.

## If something breaks

- **Errors about torch / PyTorch downloads**: you're running an old copy of
  the script. It no longer needs PyTorch (`gpu.py` talks to the NVIDIA driver
  directly). Run `git pull`, then re-run.
- **"Could not request local forwarding"** when you SSH in: another session
  already uses that Mac port. Harmless for the test; to reach the server from
  your Mac too, pick another Mac port, e.g. `-L 18082:localhost:18080`.
- **"cannot reach the GPU through the NVIDIA driver"**: run `nvidia-smi` on
  the box; if that fails too, the box itself is broken, so rent another.
- **"server did not start"**: the script prints the last log lines; paste them.
- **Re-running**: just run `bash scripts/gputest/run.sh` again. It restarts the
  server and skips anything already installed.
