CREATE TABLE IF NOT EXISTS runs (
    id UUID PRIMARY KEY,
    status TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    symbol TEXT NOT NULL,
    interval TEXT NOT NULL,
    strategy TEXT NOT NULL,
    days INT NOT NULL,
    params JSONB NOT NULL DEFAULT '{}',
    error_message TEXT,
    bars INT,
    build_ms BIGINT,
    from_time TIMESTAMPTZ,
    to_time TIMESTAMPTZ,
    rsi_oversold DOUBLE PRECISION,
    rsi_overbought DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_runs_request_hash ON runs (request_hash);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs (status);

CREATE TABLE IF NOT EXISTS run_metrics (
    run_id UUID PRIMARY KEY REFERENCES runs (id) ON DELETE CASCADE,
    sharpe_ratio DOUBLE PRECISION NOT NULL,
    max_drawdown DOUBLE PRECISION NOT NULL,
    total_return DOUBLE PRECISION NOT NULL,
    final_equity DOUBLE PRECISION NOT NULL,
    initial_equity DOUBLE PRECISION NOT NULL,
    num_trades INT NOT NULL
);

CREATE TABLE IF NOT EXISTS equity_points (
    run_id UUID NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    time TIMESTAMPTZ NOT NULL,
    equity DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (run_id, time)
);

CREATE TABLE IF NOT EXISTS trades (
    run_id UUID NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    time TIMESTAMPTZ NOT NULL,
    side TEXT NOT NULL,
    qty DOUBLE PRECISION NOT NULL,
    price DOUBLE PRECISION NOT NULL,
    fee DOUBLE PRECISION NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trades_run_id ON trades (run_id);

CREATE TABLE IF NOT EXISTS price_points (
    run_id UUID NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    time TIMESTAMPTZ NOT NULL,
    close DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (run_id, time)
);

CREATE TABLE IF NOT EXISTS indicator_points (
    run_id UUID NOT NULL REFERENCES runs (id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    time TIMESTAMPTZ NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (run_id, label, time)
);
