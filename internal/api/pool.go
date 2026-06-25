package api

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/leykun/bybit-backtester/internal/cache"
	"github.com/leykun/bybit-backtester/internal/store"
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
	jobs    chan string
	db      *store.DB
	rdb     *cache.Client
	ctx     context.Context
}

func NewPool(ctx context.Context, numWorkers, queueSize int, db *store.DB, rdb *cache.Client) *Pool {
	p := &Pool{
		jobs: make(chan string, queueSize),
		db:   db,
		rdb:  rdb,
		ctx:  ctx,
	}
	for i := 0; i < numWorkers; i++ {
		go p.worker()
	}
	return p
}

func (p *Pool) worker() {
	for id := range p.jobs {
		if err := p.db.SetStatus(p.ctx, id, string(StatusRunning), ""); err != nil {
			log.Printf("set running %s: %v", id, err)
			continue
		}

		row, err := p.db.GetRun(p.ctx, id)
		if err != nil {
			_ = p.db.SetStatus(p.ctx, id, string(StatusError), err.Error())
			continue
		}
		req := BacktestRequest{
			Symbol: row.Symbol, Interval: row.Interval, Strategy: row.Strategy,
			Days: row.Days, Params: row.Params,
		}

		res, err := runBacktest(p.ctx, p.rdb, req)
		if err != nil {
			_ = p.db.SetStatus(p.ctx, id, string(StatusError), err.Error())
			continue
		}

		if err := SaveResult(p.ctx, p.db, id, res); err != nil {
			_ = p.db.SetStatus(p.ctx, id, string(StatusError), err.Error())
			continue
		}
		if err := p.db.SetStatus(p.ctx, id, string(StatusDone), ""); err != nil {
			log.Printf("set done %s: %v", id, err)
			continue
		}

		hash, err := store.RequestHash(req.Symbol, req.Interval, req.Strategy, req.Days, req.Params)
		if err != nil {
			log.Printf("result cache hash %s: %v", id, err)
			continue
		}
		if err := p.rdb.SetResultRunID(p.ctx, hash, id); err != nil {
			log.Printf("result cache set %s: %v", id, err)
		}
	}
}

func (p *Pool) Submit(req BacktestRequest) (string, bool) {
	if err := validate(&req); err != nil {
		return "", false
	}

	hash, err := store.RequestHash(req.Symbol, req.Interval, req.Strategy, req.Days, req.Params)
	if err != nil {
		return "", false
	}

	if cachedID, ok, err := p.rdb.GetResultRunID(p.ctx, hash); err == nil && ok {
		if row, err := p.db.GetRun(p.ctx, cachedID); err == nil && row.Status == string(StatusDone) {
			return cachedID, true
		}
	}

	if existingID, ok, err := p.db.FindDoneRunByHash(p.ctx, hash); err == nil && ok {
		_ = p.rdb.SetResultRunID(p.ctx, hash, existingID)
		return existingID, true
	}

	id := uuid.NewString()
	if err := p.db.CreateRun(p.ctx, id, hash, req.Symbol, req.Interval, req.Strategy, req.Days, req.Params); err != nil {
		return "", false
	}

	select {
	case p.jobs <- id:
		return id, true
	default:
		_ = p.db.SetStatus(p.ctx, id, string(StatusError), "queue full")
		return "", false
	}
}

func (p *Pool) Get(id string) (*Job, bool) {
	row, err := p.db.GetRun(p.ctx, id)
	if err != nil {
		return nil, false
	}
	job, err := runRowToJob(p.ctx, p.db, row)
	if err != nil {
		return nil, false
	}
	return job, true
}
