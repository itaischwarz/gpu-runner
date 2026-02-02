package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/store"

	"github.com/gorilla/mux"
)

func setupTestHandlers(t *testing.T) *Handlers {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-api-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	jobStore, err := store.NewJobStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { jobStore.DB.Close() })

	exec := executer.NewExecutor()
	queue := jobs.NewJobQueue(10)
	queue.Executor = exec

	return &Handlers{
		Queue:    queue,
		JobStore: jobStore,
	}
}

func TestGetJobHandler(t *testing.T) {
	h := setupTestHandlers(t)

	job := &jobs.Job{
		Command:      "echo test",
		Status:       jobs.StatusPending,
		StorageBytes: jobs.Volume10MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
	}
	if err := h.JobStore.CreateJob(job); err != nil {
		t.Fatalf("failed to seed job: %v", err)
	}

	req := httptest.NewRequest("GET", "/jobs/"+job.ID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": job.ID})
	rr := httptest.NewRecorder()

	h.GetJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp jobs.Job
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Command != "echo test" {
		t.Errorf("expected command 'echo test', got %q", resp.Command)
	}
}

func TestGetJobNotFoundHandler(t *testing.T) {
	h := setupTestHandlers(t)

	req := httptest.NewRequest("GET", "/jobs/999", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "999"})
	rr := httptest.NewRecorder()

	h.GetJob(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestListJobsHandler(t *testing.T) {
	h := setupTestHandlers(t)

	for _, st := range []jobs.JobStatus{jobs.StatusPending, jobs.StatusRunning, jobs.StatusSuccess} {
		job := &jobs.Job{
			Command:      "echo test",
			Status:       st,
			StorageBytes: jobs.Volume10MB,
			VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
		}
		if err := h.JobStore.CreateJob(job); err != nil {
			t.Fatalf("failed to seed job: %v", err)
		}
	}

	// List all
	req := httptest.NewRequest("GET", "/jobs", nil)
	rr := httptest.NewRecorder()
	h.ListJobs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var all []*jobs.Job
	if err := json.NewDecoder(rr.Body).Decode(&all); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 jobs, got %d", len(all))
	}

	// List filtered
	req = httptest.NewRequest("GET", "/jobs?status=pending", nil)
	rr = httptest.NewRecorder()
	h.ListJobs(rr, req)

	var filtered []*jobs.Job
	if err := json.NewDecoder(rr.Body).Decode(&filtered); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 pending job, got %d", len(filtered))
	}
}

func TestListJobsEmptyHandler(t *testing.T) {
	h := setupTestHandlers(t)

	req := httptest.NewRequest("GET", "/jobs", nil)
	rr := httptest.NewRecorder()
	h.ListJobs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Empty list should return null or empty array
	body := rr.Body.String()
	if body != "null\n" && body != "[]\n" {
		t.Errorf("expected null or empty array, got %q", body)
	}
}

func TestCancelJobHandlerNonExistent(t *testing.T) {
	h := setupTestHandlers(t)

	cancelBody, _ := json.Marshal(map[string]string{"reason": "no longer needed"})
	req := httptest.NewRequest("POST", "/endjobs/999", bytes.NewReader(cancelBody))
	req = mux.SetURLVars(req, map[string]string{"id": "999"})
	rr := httptest.NewRecorder()

	h.CancelJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-existent job, got %d", rr.Code)
	}
}

func TestCancelCompletedJobHandler(t *testing.T) {
	h := setupTestHandlers(t)

	job := &jobs.Job{
		Command:      "echo done",
		Status:       jobs.StatusSuccess,
		StorageBytes: jobs.Volume10MB,
		VolumePath:   jobs.VolumePaths[jobs.Volume10MB],
	}
	if err := h.JobStore.CreateJob(job); err != nil {
		t.Fatalf("failed to seed job: %v", err)
	}

	req := httptest.NewRequest("POST", "/endjobs/"+job.ID, bytes.NewReader([]byte("{}")))
	req = mux.SetURLVars(req, map[string]string{"id": job.ID})
	rr := httptest.NewRecorder()

	h.CancelJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for completed job cancel, got %d", rr.Code)
	}
}

func TestRouterRegistration(t *testing.T) {
	h := setupTestHandlers(t)
	router := NewRouter(h)

	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/jobs"},
		{"GET", "/jobs"},
		{"POST", "/endjobs/123"},
		{"GET", "/jobs/123"},
		{"DELETE", "/server"},
	}

	for _, tc := range routes {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		match := &mux.RouteMatch{}
		if !router.Match(req, match) {
			t.Errorf("expected route %s %s to match", tc.method, tc.path)
		}
	}
}

func TestAuthMiddleWareValidToken(t *testing.T) {
	token := "test-secret-token"
	called := false

	handler := AuthMiddleWare(token, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("DELETE", "/server", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if !called {
		t.Error("expected inner handler to be called with valid token")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAuthMiddleWareInvalidToken(t *testing.T) {
	handler := AuthMiddleWare("correct-token", func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called with invalid token")
	})

	req := httptest.NewRequest("DELETE", "/server", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleWareMissingToken(t *testing.T) {
	handler := AuthMiddleWare("some-token", func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called with missing token")
	})

	req := httptest.NewRequest("DELETE", "/server", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestShutdownEndpointWithAuth(t *testing.T) {
	token := "shutdown-secret"
	t.Setenv("SHUTDOWN_TOKEN", token)

	quitCalled := false
	h := setupTestHandlers(t)
	h.QuitFunction = func() { quitCalled = true }

	router := NewRouter(h)

	// Valid token should trigger shutdown
	req := httptest.NewRequest("DELETE", "/server", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !quitCalled {
		t.Error("expected QuitFunction to be called")
	}

	// Invalid token should be rejected
	quitCalled = false
	req = httptest.NewRequest("DELETE", "/server", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if quitCalled {
		t.Error("QuitFunction should not be called with invalid token")
	}
}
