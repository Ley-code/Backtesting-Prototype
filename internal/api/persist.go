package api

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/leykun/bybit-backtester/internal/metrics"
	"github.com/leykun/bybit-backtester/internal/portfolio"
	"github.com/leykun/bybit-backtester/internal/store"
	"github.com/leykun/bybit-backtester/internal/strategy"
)

func SaveResult(ctx context.Context, db *store.DB, runID string, result *BacktestResult) error {
	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var rsiOversold, rsiOverbought *float64
	if result.RSIBands != nil {
		rsiOversold = &result.RSIBands.Oversold
		rsiOverbought = &result.RSIBands.Overbought
	}

	_, err = tx.Exec(ctx, `
		UPDATE runs SET bars = $2, build_ms = $3, from_time = $4, to_time = $5,
			rsi_oversold = $6, rsi_overbought = $7, updated_at = NOW()
		WHERE id = $1`,
		runID, result.Bars, result.BuildMS, result.From, result.To, rsiOversold, rsiOverbought)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM run_metrics WHERE run_id = $1`, runID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO run_metrics (run_id, sharpe_ratio, max_drawdown, total_return, final_equity, initial_equity, num_trades)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		runID, result.Metrics.SharpeRatio, result.Metrics.MaxDrawdown, result.Metrics.TotalReturn,
		result.Metrics.FinalEquity, result.Metrics.InitialEquity, result.Metrics.NumTrades)
	if err != nil {
		return err
	}

	if err := deleteChildRows(ctx, tx, runID); err != nil {
		return err
	}

	if err := insertEquity(ctx, tx, runID, result.Equity); err != nil {
		return err
	}
	if err := insertTrades(ctx, tx, runID, result.Trades); err != nil {
		return err
	}
	if err := insertPrice(ctx, tx, runID, result.Price); err != nil {
		return err
	}
	if err := insertIndicators(ctx, tx, runID, result.Indicators); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func deleteChildRows(ctx context.Context, tx pgx.Tx, runID string) error {
	tables := []string{"equity_points", "trades", "price_points", "indicator_points"}
	for _, t := range tables {
		if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE run_id = $1", t), runID); err != nil {
			return err
		}
	}
	return nil
}

func insertEquity(ctx context.Context, tx pgx.Tx, runID string, points []portfolio.EquityPoint) error {
	if len(points) == 0 {
		return nil
	}
	rows := make([][]any, len(points))
	for i, p := range points {
		rows[i] = []any{runID, p.Time, p.Equity}
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"equity_points"},
		[]string{"run_id", "time", "equity"},
		pgx.CopyFromRows(rows))
	return err
}

func insertTrades(ctx context.Context, tx pgx.Tx, runID string, trades []portfolio.TradeLog) error {
	if len(trades) == 0 {
		return nil
	}
	rows := make([][]any, len(trades))
	for i, t := range trades {
		rows[i] = []any{runID, t.Time, t.Side, t.Qty, t.Price, t.Fee}
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"trades"},
		[]string{"run_id", "time", "side", "qty", "price", "fee"},
		pgx.CopyFromRows(rows))
	return err
}

func insertPrice(ctx context.Context, tx pgx.Tx, runID string, price []pricePoint) error {
	if len(price) == 0 {
		return nil
	}
	rows := make([][]any, len(price))
	for i, p := range price {
		rows[i] = []any{runID, p.Time, p.Close}
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"price_points"},
		[]string{"run_id", "time", "close"},
		pgx.CopyFromRows(rows))
	return err
}

func insertIndicators(ctx context.Context, tx pgx.Tx, runID string, indicators map[string][]strategy.IndicatorPoint) error {
	if len(indicators) == 0 {
		return nil
	}
	var rows [][]any
	for label, series := range indicators {
		for _, p := range series {
			rows = append(rows, []any{runID, label, p.Time, p.Value})
		}
	}
	if len(rows) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"indicator_points"},
		[]string{"run_id", "label", "time", "value"},
		pgx.CopyFromRows(rows))
	return err
}

func LoadResult(ctx context.Context, db *store.DB, runID string) (*BacktestResult, error) {
	row, err := db.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	result := &BacktestResult{
		Request: BacktestRequest{
			Symbol: row.Symbol, Interval: row.Interval, Strategy: row.Strategy,
			Days: row.Days, Params: row.Params,
		},
	}
	if row.Bars != nil {
		result.Bars = *row.Bars
	}
	if row.BuildMS != nil {
		result.BuildMS = *row.BuildMS
	}
	if row.FromTime != nil {
		result.From = *row.FromTime
	}
	if row.ToTime != nil {
		result.To = *row.ToTime
	}
	if row.RSIOversold != nil && row.RSIOverbought != nil {
		result.RSIBands = &rsiBands{Oversold: *row.RSIOversold, Overbought: *row.RSIOverbought}
	}

	m, err := loadMetrics(ctx, db, runID)
	if err != nil {
		return nil, err
	}
	result.Metrics = *m

	result.Equity, err = loadEquity(ctx, db, runID)
	if err != nil {
		return nil, err
	}
	result.Trades, err = loadTrades(ctx, db, runID)
	if err != nil {
		return nil, err
	}
	result.Price, err = loadPrice(ctx, db, runID)
	if err != nil {
		return nil, err
	}
	result.Indicators, err = loadIndicators(ctx, db, runID)
	if err != nil {
		return nil, err
	}
	if result.Trades == nil {
		result.Trades = []portfolio.TradeLog{}
	}
	return result, nil
}

func loadMetrics(ctx context.Context, db *store.DB, runID string) (*metrics.Result, error) {
	var m metrics.Result
	err := db.Pool().QueryRow(ctx, `
		SELECT sharpe_ratio, max_drawdown, total_return, final_equity, initial_equity, num_trades
		FROM run_metrics WHERE run_id = $1`, runID).Scan(
		&m.SharpeRatio, &m.MaxDrawdown, &m.TotalReturn, &m.FinalEquity, &m.InitialEquity, &m.NumTrades)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func loadEquity(ctx context.Context, db *store.DB, runID string) ([]portfolio.EquityPoint, error) {
	rows, err := db.Pool().Query(ctx, `
		SELECT time, equity FROM equity_points WHERE run_id = $1 ORDER BY time`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []portfolio.EquityPoint
	for rows.Next() {
		var p portfolio.EquityPoint
		if err := rows.Scan(&p.Time, &p.Equity); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadTrades(ctx context.Context, db *store.DB, runID string) ([]portfolio.TradeLog, error) {
	rows, err := db.Pool().Query(ctx, `
		SELECT time, side, qty, price, fee FROM trades WHERE run_id = $1 ORDER BY time`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []portfolio.TradeLog
	for rows.Next() {
		var t portfolio.TradeLog
		if err := rows.Scan(&t.Time, &t.Side, &t.Qty, &t.Price, &t.Fee); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadPrice(ctx context.Context, db *store.DB, runID string) ([]pricePoint, error) {
	rows, err := db.Pool().Query(ctx, `
		SELECT time, close FROM price_points WHERE run_id = $1 ORDER BY time`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pricePoint
	for rows.Next() {
		var p pricePoint
		if err := rows.Scan(&p.Time, &p.Close); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func loadIndicators(ctx context.Context, db *store.DB, runID string) (map[string][]strategy.IndicatorPoint, error) {
	rows, err := db.Pool().Query(ctx, `
		SELECT label, time, value FROM indicator_points WHERE run_id = $1 ORDER BY label, time`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]strategy.IndicatorPoint{}
	for rows.Next() {
		var label string
		var p strategy.IndicatorPoint
		if err := rows.Scan(&label, &p.Time, &p.Value); err != nil {
			return nil, err
		}
		out[label] = append(out[label], p)
	}
	return out, rows.Err()
}

func runRowToJob(ctx context.Context, db *store.DB, row *store.RunRow) (*Job, error) {
	job := &Job{
		ID:     row.ID,
		Status: JobStatus(row.Status),
		Request: BacktestRequest{
			Symbol: row.Symbol, Interval: row.Interval, Strategy: row.Strategy,
			Days: row.Days, Params: row.Params,
		},
	}
	if row.ErrorMessage != "" {
		job.Error = row.ErrorMessage
	}
	if row.Status == string(StatusDone) {
		res, err := LoadResult(ctx, db, row.ID)
		if err != nil {
			return nil, err
		}
		job.Result = res
	}
	return job, nil
}
