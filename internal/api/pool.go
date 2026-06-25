package api

import (
	"sync"

	"github.com/google/uuid"
)

type JobStatus string

const (
	StatusQueued  JobStatus = "queued"
	StatusRunning JobStatus = "running"
	StatusDone    JobStatus = "done"
	StatusError   JobStatus = "error"
)

type Job struct {
	ID      string          `json:"id"`
	Status  JobStatus       `json:"status"`
	Request BacktestRequest `json:"request"`
	Result  *BacktestResult `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type Pool struct {
	jobs     chan string
	cacheDir string

	mu    sync.RWMutex
	store map[string]*Job
}

func NewPool(numWorkers, queueSize int, cacheDir string) *Pool {
	p := &Pool{
		jobs:     make(chan string, queueSize),
		cacheDir: cacheDir,
		store:    make(map[string]*Job),
	}
	for i := 0; i < numWorkers; i++ {
		go p.worker()
	}
	return p
}

func (p *Pool) worker() {
	for id := range p.jobs {
		p.setStatus(id, StatusRunning)

		p.mu.RLock()
		req := p.store[id].Request
		p.mu.RUnlock()

		res, err := runBacktest(p.cacheDir, req)

		p.mu.Lock()
		job := p.store[id]
		if err != nil {
			job.Status = StatusError
			job.Error = err.Error()
		} else {
			job.Status = StatusDone
			job.Result = res
		}
		p.mu.Unlock()
	}
}

func (p *Pool) Submit(req BacktestRequest) (string, bool) {
	id := uuid.NewString()
	p.mu.Lock()
	p.store[id] = &Job{ID: id, Status: StatusQueued, Request: req}
	p.mu.Unlock()

	select {
	case p.jobs <- id:
		return id, true
	default:
		p.mu.Lock()
		delete(p.store, id)
		p.mu.Unlock()
		return "", false
	}
}

func (p *Pool) Get(id string) (*Job, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	job, ok := p.store[id]
	return job, ok
}

func (p *Pool) setStatus(id string, s JobStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if job, ok := p.store[id]; ok {
		job.Status = s
	}
}
