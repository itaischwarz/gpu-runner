package store

import (
	"os"
	"testing"
	"time"

	"gpu-runner/internal/jobs"
)

func setupTestStore(t *testing.T) *JobStore {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-jobs-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := NewJobStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create job store: %v", err)
	}
	t.Cleanup(func() { store.DB.Close() })
	return store
}

func TestCreateJob(t *testing.T) {
	s := setupTestStore(t)

	job := &jobs.Job{
		Command:      "echo hello",
		Status:       jobs.StatusPending,
		StorageBytes: jobs.Volume10MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
	}

	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	if job.ID == "" {
		t.Fatal("expected job ID to be set after creation")
	}
	if job.ID != "1" {
		t.Errorf("expected first job ID to be '1', got %q", job.ID)
	}
	if job.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCreateMultipleJobs(t *testing.T) {
	s := setupTestStore(t)

	for i := 0; i < 5; i++ {
		job := &jobs.Job{
			Command:      "echo test",
			Status:       jobs.StatusPending,
			StorageBytes: jobs.Volume10MB,
			VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
		}
		if err := s.CreateJob(job); err != nil {
			t.Fatalf("CreateJob %d failed: %v", i, err)
		}
	}

	all, err := s.ListJobs("")
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("expected 5 jobs, got %d", len(all))
	}
}

func TestGetJob(t *testing.T) {
	s := setupTestStore(t)

	job := &jobs.Job{
		Command:      "ls -la",
		Status:       jobs.StatusPending,
		StorageBytes: jobs.Volume25MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume25MB],
	}
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	fetched, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}

	if fetched.Command != "ls -la" {
		t.Errorf("expected command 'ls -la', got %q", fetched.Command)
	}
	if fetched.Status != jobs.StatusPending {
		t.Errorf("expected status pending, got %q", fetched.Status)
	}
	if fetched.StorageBytes != jobs.Volume25MB {
		t.Errorf("expected storage %d, got %d", jobs.Volume25MB, fetched.StorageBytes)
	}
}

func TestGetJobNotFound(t *testing.T) {
	s := setupTestStore(t)

	_, err := s.GetJob("999")
	if err == nil {
		t.Fatal("expected error for non-existent job")
	}
}

func TestUpdateJob(t *testing.T) {
	s := setupTestStore(t)

	job := &jobs.Job{
		Command:      "sleep 10",
		Status:       jobs.StatusRunning,
		StorageBytes: jobs.Volume10MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
	}
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	job.Status = jobs.StatusSuccess
	job.FinishedAt = time.Now().Format(time.RFC3339)
	if err := s.UpdateJob(job); err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}

	fetched, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if fetched.Status != jobs.StatusSuccess {
		t.Errorf("expected status success, got %q", fetched.Status)
	}
}

func TestListJobsWithStatusFilter(t *testing.T) {
	s := setupTestStore(t)

	statuses := []jobs.JobStatus{jobs.StatusPending, jobs.StatusRunning, jobs.StatusPending, jobs.StatusSuccess}
	for _, st := range statuses {
		job := &jobs.Job{
			Command:      "echo test",
			Status:       st,
			StorageBytes: jobs.Volume10MB,
			VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
		}
		if err := s.CreateJob(job); err != nil {
			t.Fatalf("CreateJob failed: %v", err)
		}
	}

	pending, err := s.ListJobs("pending")
	if err != nil {
		t.Fatalf("ListJobs(pending) failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending jobs, got %d", len(pending))
	}

	running, err := s.ListJobs("running")
	if err != nil {
		t.Fatalf("ListJobs(running) failed: %v", err)
	}
	if len(running) != 1 {
		t.Errorf("expected 1 running job, got %d", len(running))
	}

	all, err := s.ListJobs("")
	if err != nil {
		t.Fatalf("ListJobs('') failed: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 total jobs, got %d", len(all))
	}
}

func TestListJobsDescendingOrder(t *testing.T) {
	s := setupTestStore(t)

	for i := 0; i < 3; i++ {
		job := &jobs.Job{
			Command:      "echo test",
			Status:       jobs.StatusPending,
			StorageBytes: jobs.Volume10MB,
			VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := s.CreateJob(job); err != nil {
			t.Fatalf("CreateJob failed: %v", err)
		}
	}

	all, err := s.ListJobs("")
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(all))
	}
	// Most recent should be first (DESC order)
	if all[0].ID != "3" {
		t.Errorf("expected first job ID '3' (most recent), got %q", all[0].ID)
	}
	if all[2].ID != "1" {
		t.Errorf("expected last job ID '1' (oldest), got %q", all[2].ID)
	}
}

func TestCancelPendingJob(t *testing.T) {
	s := setupTestStore(t)

	job := &jobs.Job{
		Command:      "echo cancel-me",
		Status:       jobs.StatusPending,
		StorageBytes: jobs.Volume10MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
	}
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	cancelled, err := s.CancelJob(job.ID)
	if err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}
	if cancelled.Status != jobs.StatusCancelled {
		t.Errorf("expected status cancelled, got %q", cancelled.Status)
	}

	fetched, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if fetched.Status != jobs.StatusCancelled {
		t.Errorf("expected persisted status cancelled, got %q", fetched.Status)
	}
}

func TestCancelRunningJob(t *testing.T) {
	s := setupTestStore(t)

	job := &jobs.Job{
		Command:      "sleep 100",
		Status:       jobs.StatusRunning,
		StorageBytes: jobs.Volume10MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
	}
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	_, err := s.CancelJob(job.ID)
	if err != nil {
		t.Fatalf("CancelJob on running job should succeed: %v", err)
	}
}

func TestCancelCompletedJobFails(t *testing.T) {
	s := setupTestStore(t)

	job := &jobs.Job{
		Command:      "echo done",
		Status:       jobs.StatusSuccess,
		StorageBytes: jobs.Volume10MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
	}
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	_, err := s.CancelJob(job.ID)
	if err == nil {
		t.Fatal("expected error when cancelling a completed job")
	}
}

func TestCancelNonExistentJobFails(t *testing.T) {
	s := setupTestStore(t)

	_, err := s.CancelJob("999")
	if err == nil {
		t.Fatal("expected error when cancelling a non-existent job")
	}
}
