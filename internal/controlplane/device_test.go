package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseHealthyGPUs(t *testing.T) {
	out := "GPU-aaa, NVIDIA A100-SXM4-80GB, 81920, 80000\n" +
		"GPU-bbb, NVIDIA GeForce RTX 4090, 24564, 1200\n"

	devices, err := parseNvidiaSMI(out)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	want := []GPUDevice{
		{UUID: "GPU-aaa", Model: "NVIDIA A100-SXM4-80GB", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true},
		{UUID: "GPU-bbb", Model: "NVIDIA GeForce RTX 4090", TotalMemoryMB: 24564, FreeMemoryMB: 1200, Healthy: true},
	}
	if len(devices) != len(want) {
		t.Fatalf("expected %d devices, got %d", len(want), len(devices))
	}
	for i := range want {
		if devices[i] != want[i] {
			t.Errorf("device %d: expected %+v, got %+v", i, want[i], devices[i])
		}
	}
}

func TestParseUnreadableMemoryIsUnhealthy(t *testing.T) {
	out := "GPU-aaa, NVIDIA A100, [N/A], [N/A]\n" +
		"GPU-bbb, NVIDIA A100, 81920, [GPU requires reset]\n"

	devices, err := parseNvidiaSMI(out)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	for _, d := range devices {
		if d.Healthy {
			t.Errorf("expected %s to be unhealthy", d.UUID)
		}
	}
}

func TestParseSkipsRowsWithoutUUIDOrWrongColumns(t *testing.T) {
	out := "GPU-aaa, NVIDIA A100, 81920, 80000\n" +
		"GPU-bbb, NVIDIA A100, 81920\n" +
		"[N/A], NVIDIA A100, 81920, 80000\n"

	devices, err := parseNvidiaSMI(out)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(devices) != 1 || devices[0].UUID != "GPU-aaa" {
		t.Fatalf("expected only GPU-aaa, got %+v", devices)
	}
}

func TestParseEmptyOutput(t *testing.T) {
	devices, err := parseNvidiaSMI("")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices, got %+v", devices)
	}
}

// fakeSMI writes an executable script that stands in for nvidia-smi.
func fakeSMI(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nvidia-smi")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake nvidia-smi: %v", err)
	}
	return path
}

func TestQueryGPUsRunsNvidiaSMI(t *testing.T) {
	smi := fakeSMI(t, `
[ "$1" = "--query-gpu=uuid,name,memory.total,memory.free" ] || { echo "bad args: $*" >&2; exit 2; }
[ "$2" = "--format=csv,noheader,nounits" ] || { echo "bad args: $*" >&2; exit 2; }
echo "GPU-aaa, NVIDIA A100, 81920, 80000"`)

	devices, err := QueryGPUs(context.Background(), smi)
	if err != nil {
		t.Fatalf("QueryGPUs failed: %v", err)
	}
	if len(devices) != 1 || devices[0].UUID != "GPU-aaa" || !devices[0].Healthy {
		t.Fatalf("unexpected devices: %+v", devices)
	}
}

func TestQueryGPUsFailsWhenNvidiaSMIFails(t *testing.T) {
	smi := fakeSMI(t, `echo "NVIDIA-SMI has failed because it couldn't communicate with the NVIDIA driver" >&2; exit 9`)

	if _, err := QueryGPUs(context.Background(), smi); err == nil {
		t.Fatal("expected an error when nvidia-smi exits non-zero")
	}
}
