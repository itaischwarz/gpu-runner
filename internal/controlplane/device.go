package controlplane

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GPUDevice is a single nvidia-smi reading for one GPU.
type GPUDevice struct {
	UUID          string `json:"uuid"`
	Model         string `json:"model"`
	TotalMemoryMB int    `json:"total_memory_mb"`
	FreeMemoryMB  int    `json:"free_memory_mb"`
	Healthy       bool   `json:"healthy"`
}

// queryFields is the column order requested from nvidia-smi and expected by parseNvidiaSMI.
var queryFields = []string{"uuid", "name", "memory.total", "memory.free"}

// queryTimeout bounds a single nvidia-smi call. nvidia-smi can hang when a GPU
// falls off the bus, which is exactly when we need the poll to come back.
const queryTimeout = 10 * time.Second

// QueryGPUs runs nvidia-smi once and returns a reading for every GPU it reports.
func QueryGPUs(ctx context.Context, smiPath string) ([]GPUDevice, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, smiPath,
		"--query-gpu="+strings.Join(queryFields, ","),
		"--format=csv,noheader,nounits",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("nvidia-smi failed: %w: %s", err, strings.TrimSpace(stderr.String()+stdout.String()))
	}
	return parseNvidiaSMI(stdout.String())
}

// parseNvidiaSMI turns nvidia-smi CSV output into devices. A row whose memory
// readings don't parse (e.g. "[N/A]" or "[GPU requires reset]") yields an
// unhealthy device. A row with the wrong number of columns or no UUID is
// skipped, since it can't be attributed to a GPU.
func parseNvidiaSMI(out string) ([]GPUDevice, error) {
	r := csv.NewReader(strings.NewReader(out))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1

	var devices []GPUDevice
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse nvidia-smi output: %w", err)
		}
		if len(rec) != len(queryFields) {
			controlPlaneLogger.Warn("Skipping malformed nvidia-smi row", "row", strings.Join(rec, ","))
			continue
		}

		uuid := strings.TrimSpace(rec[0])
		if uuid == "" || strings.HasPrefix(uuid, "[") {
			controlPlaneLogger.Warn("Skipping nvidia-smi row without a UUID", "row", strings.Join(rec, ","))
			continue
		}

		total, totalOK := parseMB(rec[2])
		free, freeOK := parseMB(rec[3])
		devices = append(devices, GPUDevice{
			UUID:          uuid,
			Model:         strings.TrimSpace(rec[1]),
			TotalMemoryMB: total,
			FreeMemoryMB:  free,
			Healthy:       totalOK && freeOK,
		})
	}
	return devices, nil
}

func parseMB(raw string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return v, true
}
