package gpu

import (
	"context"
	"gpu-runner/internal/controlplane"
	"testing"
)

func TestProviderPollMapsDevicesToSlots(t *testing.T) {
	smi := fakeSMI(t, `echo "GPU-aaa, NVIDIA A100, 81920, 80000"`)
	p := NewProvider(smi)

	slots, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll failed: %v", err)
	}

	want := controlplane.Slot{ID: "GPU-aaa", Model: "NVIDIA A100", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true}
	if len(slots) != 1 || slots[0] != want {
		t.Fatalf("expected [%+v], got %+v", want, slots)
	}
}

func TestProviderPollReturnsQueryError(t *testing.T) {
	p := NewProvider(fakeSMI(t, "exit 1"))

	if _, err := p.Poll(context.Background()); err == nil {
		t.Fatal("expected an error when nvidia-smi fails")
	}
}

func TestProviderEnvPinsGPUByUUID(t *testing.T) {
	env := NewProvider("nvidia-smi").Env("GPU-aaa")
	if len(env) != 1 || env[0] != "CUDA_VISIBLE_DEVICES=GPU-aaa" {
		t.Fatalf("unexpected env: %v", env)
	}
}
