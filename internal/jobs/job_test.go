package jobs

import (
	"testing"
)

func TestJobStatusConstants(t *testing.T) {
	tests := []struct {
		status   JobStatus
		expected string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusSuccess, "success"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
	}

	for _, tc := range tests {
		if string(tc.status) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, tc.status)
		}
	}
}

func TestVolumeConstants(t *testing.T) {
	if Volume10MB != 10*1024*1024 {
		t.Errorf("Volume10MB = %d, want %d", Volume10MB, 10*1024*1024)
	}
	if Volume25MB != 25*1024*1024 {
		t.Errorf("Volume25MB = %d, want %d", Volume25MB, 25*1024*1024)
	}
	if Volume50MB != 50*1024*1024 {
		t.Errorf("Volume50MB = %d, want %d", Volume50MB, 50*1024*1024)
	}
}

func TestVolumePathsMapping(t *testing.T) {
	expected := map[JobStorage]string{
		Volume10MB: "/var/lib/jobrunner/volumes/10mb",
		Volume25MB: "/var/lib/jobrunner/volumes/25mb",
		Volume50MB: "/var/lib/jobrunner/volumes/50mb",
	}

	for vol, path := range expected {
		got, ok := VolumePaths[vol]
		if !ok {
			t.Errorf("missing volume path for %d", vol)
			continue
		}
		if got != path {
			t.Errorf("VolumePaths[%d] = %q, want %q", vol, got, path)
		}
	}
}

func TestJobQueueEnqueue(t *testing.T) {
	q := NewJobQueue(5)

	job := &Job{
		ID:      "1",
		Command: "echo test",
		Status:  StatusPending,
	}

	go q.Enqueue(job)

	received := <-q.Queue
	if received.ID != "1" {
		t.Errorf("expected job ID '1', got %q", received.ID)
	}
	if received.Command != "echo test" {
		t.Errorf("expected command 'echo test', got %q", received.Command)
	}
}

func TestJobQueueBufferSize(t *testing.T) {
	q := NewJobQueue(3)

	for i := range 3 {
		q.Enqueue(&Job{ID: string(rune('0' + i)), Command: "test"})
	}

	if len(q.Queue) != 3 {
		t.Errorf("expected queue length 3, got %d", len(q.Queue))
	}
}
