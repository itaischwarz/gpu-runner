package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"

	_ "github.com/mattn/go-sqlite3"

)

type JobStore struct {
	DB *sql.DB
}

var serverLogger *slog.Logger

func init() {
	serverLogger = logger.Server
}

func NewJobStore(path string) (*JobStore, error) {

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		serverLogger.Error("Unable to Create Sqlite DB", "error", err)
		return nil, err
	}
	js := &JobStore{DB: db}

	if err := js.initSchema(); err != nil {
		serverLogger.Error("DB Schema resulted in an error", "error", err)
	}
	log := fmt.Sprintf("Succesfully Created DB in %s", path)
	serverLogger.Info(log)
	return js, nil

}

func (s *JobStore) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL,
    status TEXT NOT NULL,
    log TEXT,
    storage_bytes INTEGER,
    volume_path TEXT,
    created_at DATETIME,
    started_at DATETIME,
    finished_at DATETIME,
    exit_code INTEGER
);`
	_, err := s.DB.Exec(schema)
	return err

}

func (s *JobStore) CreateJob(j *jobs.Job) error {

	// if j.ID == "" || j.Command == "" {
	// 	return fmt.Errorf("job missing required fields")

	// }
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now()
	}

	result, err := s.DB.Exec(
		`INSERT INTO jobs 
			(command, status, log, storage_bytes, volume_path, created_at, started_at, finished_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		j.Command,
		string(j.Status),
		j.Log,
		j.StorageBytes,
		j.VolumePath,
		j.CreatedAt,
		j.StartedAt,
		j.FinishedAt,
	)

	if err != nil {
		serverLogger.Error("unable to create job", "Error", err)
	}
	lastId, err := result.LastInsertId()
	if err != nil {
		serverLogger.Error("unable to fetch id", "Error", err)
	}
	j.ID = fmt.Sprintf("%d", lastId)

	return err

}

func (s *JobStore) GetJob(id string) (*jobs.Job, error) {

	row := s.DB.QueryRow(
		`SELECT id, command, status, log, storage_bytes, volume_path, created_at, started_at, finished_at
         FROM jobs WHERE id = ?`, id)

	var j jobs.Job
	var status string
	err := row.Scan(
		&j.ID,
		&j.Command,
		&status,
		&j.Log,
		&j.StorageBytes,
		&j.VolumePath,
		&j.CreatedAt,
		&j.StartedAt,
		&j.FinishedAt,
	)
	if err != nil {

		return nil, err
	}
	j.Status = jobs.JobStatus(status)
	return &j, nil

}

func (s *JobStore) CancelJob(id string) (*jobs.Job, error) {
	job, err := s.GetJob(id)
	if err != nil {
		return nil, err
	}
	if job.Status != jobs.StatusPending && job.Status != jobs.StatusRunning {
		return nil, fmt.Errorf("job cannot be cancelled: status is %s", job.Status)
	}
	job.Status = jobs.StatusCancelled
	_, err = s.DB.Exec(`UPDATE jobs SET status = ?, finished_at = ? WHERE id = ?`, string(job.Status), time.Now(), id)
	if err != nil {
		return nil, err
	}
	return job, nil
}
