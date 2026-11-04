package box

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gpu-runner/internal/controlplane"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
)

// pollTimeout bounds one GET /gpus. The box's own nvidia-smi call times out
// at 10s, so this leaves room for the network.
const pollTimeout = 15 * time.Second

// Client talks to one GPU box. It is the central server's view of that box:
// a provider for its GPUs and a way to run jobs on them.
type Client struct {
	url  string
	http *http.Client // no overall timeout: a run lasts as long as its job
}

var _ controlplane.Provider = (*Client)(nil)

// New returns a client for the box at url, e.g. http://127.0.0.1:19000.
func New(url string) *Client {
	return &Client{url: strings.TrimRight(url, "/"), http: &http.Client{}}
}

// URL is the box's address.
func (c *Client) URL() string {
	return c.url
}

func (c *Client) Resource() jobs.JobResource {
	return jobs.ResourceGPU
}

// Poll asks the box for a fresh reading of its GPUs.
func (c *Client) Poll(ctx context.Context) ([]controlplane.Slot, error) {
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/gpus", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("box %s unreachable: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("box %s: GET /gpus: %s", c.url, readError(resp))
	}

	var slots []controlplane.Slot
	if err := json.NewDecoder(resp.Body).Decode(&slots); err != nil {
		return nil, fmt.Errorf("box %s: decode /gpus: %w", c.url, err)
	}
	return slots, nil
}

// Env pins a job to one GPU on the box. The box applies it when it runs the
// job.
func (c *Client) Env(slotID string) []string {
	return []string{"CUDA_VISIBLE_DEVICES=" + slotID}
}

// RunJobWithEnv runs a job on the box and waits for it to end. It has the
// local executor's signature, so a worker can run jobs either way.
//
// Cancelling ctx (user cancel or job timeout) closes the request, and the box
// kills the job. volumePath is ignored: storage volumes live on the central
// server, so the job runs in the box's own work directory.
func (c *Client) RunJobWithEnv(command, jobID, volumePath string, ctx context.Context, jobLogger logger.JobLogger, extraEnv []string, memoryLimitMB int) (string, error) {
	body, err := json.Marshal(RunRequest{JobID: jobID, Command: command, Env: extraEnv, MemoryMB: memoryLimitMB})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/run", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("job stopped: %w", ctx.Err())
		}
		// The box crashed or the network dropped. The job may have died with
		// it, so report a plain failure: it can be retried elsewhere.
		jobLogger.Error("GPU box unreachable", logger.Item("box", c.url), logger.Item("error", err.Error()))
		return "", fmt.Errorf("box %s unreachable: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("box %s: POST /run: %s", c.url, readError(resp))
	}

	var result RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("job stopped: %w", ctx.Err())
		}
		return "", fmt.Errorf("box %s: decode /run: %w", c.url, err)
	}

	switch {
	case result.MemoryExceeded != nil:
		memErr := &executer.MemoryExceededError{UsedMB: result.MemoryExceeded.UsedMB, LimitMB: result.MemoryExceeded.LimitMB}
		jobLogger.Error(memErr.Error(),
			logger.Item("used_mb", memErr.UsedMB),
			logger.Item("requested_mb", memErr.LimitMB),
		)
		return result.Output, memErr
	case result.Error != "":
		jobLogger.Error("Command execution failed", logger.Item("box", c.url), logger.Item("error", result.Error))
		return result.Output, errors.New(result.Error)
	}
	jobLogger.Info("Successfully executed command", logger.Item("box", c.url), logger.Item("output_length", len(result.Output)))
	return result.Output, nil
}

// readError returns a short description of a non-200 response.
func readError(resp *http.Response) string {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
}
