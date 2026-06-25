package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var migrationSQL string

type DB struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	db := &DB{pool: pool}
	if err := db.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() {
	db.pool.Close()
}

func (db *DB) migrate(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, migrationSQL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

type RunRow struct {
	ID            string
	Status        string
	RequestHash   string
	Symbol        string
	Interval      string
	Strategy      string
	Days          int
	Params        map[string]int
	ErrorMessage  string
	Bars          *int
	BuildMS       *int64
	FromTime      *time.Time
	ToTime        *time.Time
	RSIOversold   *float64
	RSIOverbought *float64
}

func (db *DB) CreateRun(ctx context.Context, id, requestHash string, symbol, interval, strategy string, days int, params map[string]int) error {
	if params == nil {
		params = map[string]int{}
	}
	pb, err := json.Marshal(params)
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, `
		INSERT INTO runs (id, status, request_hash, symbol, interval, strategy, days, params)
		VALUES ($1, 'queued', $2, $3, $4, $5, $6, $7)`,
		id, requestHash, symbol, interval, strategy, days, pb)
	return err
}

func (db *DB) FindDoneRunByHash(ctx context.Context, requestHash string) (string, bool, error) {
	var id string
	err := db.pool.QueryRow(ctx, `
		SELECT id FROM runs
		WHERE request_hash = $1 AND status = 'done'
		ORDER BY completed_at DESC NULLS LAST
		LIMIT 1`, requestHash).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

func (db *DB) SetStatus(ctx context.Context, id, status string, errMsg string) error {
	now := time.Now().UTC()
	switch status {
	case "running":
		_, err := db.pool.Exec(ctx, `
			UPDATE runs SET status = $2, updated_at = $3, started_at = COALESCE(started_at, $3)
			WHERE id = $1`, id, status, now)
		return err
	case "done":
		_, err := db.pool.Exec(ctx, `
			UPDATE runs SET status = $2, updated_at = $3, completed_at = $3, error_message = NULL
			WHERE id = $1`, id, status, now)
		return err
	case "error":
		_, err := db.pool.Exec(ctx, `
			UPDATE runs SET status = $2, updated_at = $3, completed_at = $3, error_message = $4
			WHERE id = $1`, id, status, now, errMsg)
		return err
	default:
		_, err := db.pool.Exec(ctx, `
			UPDATE runs SET status = $2, updated_at = $3 WHERE id = $1`, id, status, now)
		return err
	}
}

func (db *DB) GetRun(ctx context.Context, id string) (*RunRow, error) {
	row := &RunRow{ID: id}
	var paramsJSON []byte
	var errMsg *string
	err := db.pool.QueryRow(ctx, `
		SELECT status, request_hash, symbol, interval, strategy, days, params,
		       error_message, bars, build_ms, from_time, to_time, rsi_oversold, rsi_overbought
		FROM runs WHERE id = $1`, id).Scan(
		&row.Status, &row.RequestHash, &row.Symbol, &row.Interval, &row.Strategy,
		&row.Days, &paramsJSON, &errMsg, &row.Bars, &row.BuildMS,
		&row.FromTime, &row.ToTime, &row.RSIOversold, &row.RSIOverbought,
	)
	if err != nil {
		return nil, err
	}
	if errMsg != nil {
		row.ErrorMessage = *errMsg
	}
	row.Params = map[string]int{}
	if len(paramsJSON) > 0 {
		_ = json.Unmarshal(paramsJSON, &row.Params)
	}
	return row, nil
}

func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}
