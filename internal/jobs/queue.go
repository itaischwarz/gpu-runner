package jobs

import "gpu-runner/internal/executer"

type JobQueue struct {
    Queue chan *Job
    Executor *executer.Executor
}

func NewJobQueue(size int) *JobQueue {
    return &JobQueue{
        Queue: make(chan *Job, size),
    }
}

func (jq *JobQueue) Enqueue(job *Job) {
    jq.Queue <- job
}
