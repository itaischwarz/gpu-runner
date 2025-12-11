package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gpu-runner/internal/jobs"

	_ "github.com/mattn/go-sqlite3"
)

type JobStore struct { 
		DB *sql.DB
}
var logger *slog.Logger

func init(){
	logger = slog.New(slog.NewTextHandler(
		os.Stdout,
		&slog.HandlerOptions{Level : slog.LevelDebug},
	))
}




func NewJobStore(path string) (*JobStore, error) {


		db, err := sql.Open("sqlite3", path)
		if err != nil {
			logger.Error("Unable to Create Sqlite DB", "error", err)
			return nil, err
		}
		js := &JobStore{DB: db}
		
		if err := js.initSchema(); err == nil{
			logger.Error("DB Schema resulted in an error", "error", err)
		}
		logger.Info("Succesfully Created DB")
		return js, nil


}

func (s *JobStore) initSchema() error {
    schema := `
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
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

		if j.ID == "" || j.Command == "" {
			return fmt.Errorf("job missing required fields")

		}
		if j.CreatedAt.IsZero() {
        j.CreatedAt = time.Now()
    }

		result, err := s.DB.Exec(
			`INSERT INTO jobs 
			(id, command, status, log, storage_bytes, volume_path, created_at, started_at, finished_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			j.ID,
			j.Command,
			string(j.Status),
			j.Log,	
			j.StorageBytes,
			j.VolumePath,
			j.CreatedAt,
			j.StartedAt,
			j.FinishedAt,
	)
	lastId, err := result.LastInsertId()
	j.ID = fmt.Sprintf("%d", lastId)
	if err != nil {
		logger.Error("unable to create job", "Error", err)
	} else {
		logger.Info("Succesfully Created Job!")
	}

	

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
			 logger.Error("unable to get job", "Error", err)

       return nil, err
    }
    j.Status = jobs.JobStatus(status)
    return &j, nil

}