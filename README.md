# Bybit Backtester

An event-driven backtesting engine in Go that replays **real Bybit historical
data** and reports **Sharpe ratio** and **maximum drawdown**, with a single-page
UI to run backtests and view equity curves, trade markers, and indicator overlays.

**Live demo:** *http://13.50.126.124/* 
---

## Scope

| Dimension | Values |
|---|---|
| **Products** | BTCUSDT, ETHUSDT |
| **Timeframes** | 1 min, 5 min, 15 min |
| **Metrics** | Max Drawdown, Sharpe ratio |
| **Strategies** | MA Crossover, EMA Crossover, RSI Mean-Reversion, Bollinger Bounce, Breakout, Momentum %, Take Profit / Stop Loss |
| **Data source** | Bybit public REST API (`/v5/market/kline`, spot) + Redis bar cache |

---

## Quick start

Requires **PostgreSQL** and **Redis**. Use Docker Compose:

```bash
docker compose up -d --build
# open http://localhost
```

Pick a product, timeframe, and strategy, set a lookback window, and hit **Run
backtest**. First run for a window fetches from Bybit (a few seconds); identical
bar windows are served from Redis; identical backtest requests return instantly
from the result cache.

Run the tests:

```bash
go test ./...
```

## Docker / EC2

```bash
docker compose up -d --build
```

Then open `http://<server-ip>` (port 80 mapped to the app).

Services: **bybit-backtester**, **postgres**, **redis**.

Environment variables (set in `docker-compose.yml`):

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string (required) |
| `REDIS_URL` | Redis connection string (required) |
| `REDIS_BARS_TTL` | OHLCV bar cache TTL (default `168h`) |
| `REDIS_RESULT_TTL` | Backtest result cache TTL (default `24h`) |
| `ADDR` | HTTP listen address (default `:8080`) |
| `WEB_DIR` | Frontend assets path |

### EC2 deploy flow

On a small Ubuntu EC2 instance with Docker and the Compose plugin installed:

```bash
git clone <your-repo-url>
cd bybit-backtester
docker compose up -d --build
```

On later deploys:

```bash
git pull
docker compose up -d --build
```

If you want the app reachable publicly, allow inbound TCP `80` in the EC2
security group (or place it behind a load balancer with TLS later).

---

## Architecture

```
Browser (single-page UI)
    │  POST /api/backtests  {symbol, interval, strategy, days}
    │  GET  /api/backtests/:id   (poll for result)
    ▼
Gin REST API ──► Worker Pool (fixed N workers, bounded queue)
    │                   │  one job = one fully-isolated backtest
    │                   ▼
    │        ┌──────── BACKTEST ENGINE (per run) ────────┐
    │        │  DataHandler → Strategy → Broker →         │
    │        │       Portfolio,  all via a time-ordered   │
    │        │       EVENT QUEUE                           │
    │        └────────────────────────────────────────────┘
    │                   │ normalized result
    │                   ▼
    │            PostgreSQL (runs, trades, equity, metrics)
    │
    ├── Redis ── bar cache (OHLCV windows)
    └── Redis ── result cache (identical backtest requests)
    ▼
Bybit REST API (cache miss only)
```

### The five components (the mental model)

1. **Data Handler** (`internal/data`) — fetches OHLCV bars from Bybit on cache
   miss, stores windows in **Redis**, streams bars **one at a time, in order**.
2. **Strategy** (`internal/strategy`) — receives each bar, holds its own
   indicator state (moving averages / RSI), and emits orders. Pluggable via one
   Go interface.
3. **Broker** (`internal/broker`) — simulates fills: applies **slippage**
   (always adverse) and a **percentage fee**. No real exchange, so it *models*
   reality.
4. **Portfolio** (`internal/portfolio`) — tracks cash, the open position, and
   the **equity curve** (account value at every bar). The equity curve is the
   source of truth for every metric.
5. **Metrics** (`internal/metrics`) — consumes the equity curve to produce
   Sharpe and Max Drawdown.

### The core: a time-ordered event loop (`internal/engine`)

Everything flows through a min-heap **event queue** ordered by `(timestamp,
type)`. The engine pops the earliest event, routes it, and pushes any events the
component emits:

```
MarketEvent → strategy.OnBar() → OrderEvent → broker.Execute() → FillEvent → portfolio.OnFill()
```

**The anti-look-ahead rule:** a strategy decides on bar *T* using data up to and
including *T*'s close (the close is only knowable once the bar is over). It
therefore **cannot** trade at *T*'s close. Orders decided on bar *T* are held as
*pending* and filled at the **open of bar T+1** — the earliest price a real
trader could act on. This is enforced in the engine loop, and proven by
`TestNoLookAhead` in `internal/engine/engine_test.go`, which asserts a buy
decided on one bar fills at the *next* bar's open, not the decision bar's close.

---

## Key design decisions & tradeoffs

| Decision | Choice | Why / tradeoff |
|---|---|---|
| **Look-ahead bias** | Time-ordered queue **+ next-bar-open fills** | Two layers: the strictly time-ordered queue means a strategy at *T* can never be handed data after *T*; and orders decided on bar *T* fill at **bar T+1's open**, never at the close they were decided on. Enforced mechanically and covered by a unit test, not by discipline. |
| **Engine model** | Event-driven (not vectorized) | Slower than a vectorized pass, but correct, honest about ordering, and the same shape as a future **live-trading** path. The right call for a platform, not a one-off script. |
| **Concurrency** | Fixed-size **worker pool** | Not goroutine-per-request: under a burst, extra jobs queue on a channel instead of all fighting for CPU/memory, so the box **degrades gracefully** instead of falling over. Each run has fully isolated state. |
| **Strategy = interface** | `Strategy` with `OnBar` | Adding a new trading system is **writing one file** in `internal/strategy`; the engine never changes. Seven strategies are registered this way. |
| **Data = interface** | `DataHandler` with `Next()` | The engine doesn't know data comes from Bybit. Swapping in a live websocket feed or CSV later changes nothing upstream. |
| **Realism** | Broker models fee + adverse slippage | A backtest with zero fees and perfect fills is a fantasy. Modelling both keeps results grounded in realistic execution costs. |
| **Data fetch** | Bybit + **Redis bar cache** | Real integration; reruns on the same window skip Bybit. |
| **Job/result store** | **PostgreSQL** (normalized) | Runs, trades, equity curve, price, indicators — durable across restarts. |
| **Result cache** | **Redis** | Identical backtest requests return the cached run instantly. |
| **Async API** | POST enqueues → GET polls | This is what actually exercises the worker pool, and it's the model that scales to long-running backtests. |

---

## Project layout

```
/cmd/api              → main, server + worker pool wiring
/internal/engine      → event types, time-ordered queue, the run loop + interfaces
/internal/data        → Bybit fetch + Redis bar cache (DataHandler)
/internal/cache       → Redis client (bars + result cache)
/internal/store       → PostgreSQL job store + migrations
/internal/indicators/ → shared indicator math (SMA, EMA, RSI, Bollinger, Donchian)
/internal/strategy    → seven pluggable strategies (one file each)
/internal/broker      → execution simulation (fees, slippage)
/internal/portfolio   → cash, position, equity curve, trade log
/internal/metrics     → Sharpe + Max Drawdown (+ unit tests)
/internal/api         → Gin handlers, worker pool, persist layer
/web                  → single-page UI (chart + metrics)
```

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/options` | Products / timeframes / strategies for the UI |
| `POST` | `/api/backtests` | Enqueue a run → `{id, status}` (202) |
| `GET` | `/api/backtests/:id` | Poll job; returns metrics + equity curve when done |

Request body:

```json
{ "symbol": "BTCUSDT", "interval": "5", "strategy": "rsi_reversion", "days": 30 }
```

## Roadmap

- **More metrics** (win rate, profit factor, CAGR) and parameter sweeps.
- **Configurable date ranges** beyond rolling lookback days.
- **External job queue** (Redis Streams / NATS) for multi-instance workers.
- Golden-file tests pinning a known strategy/dataset for reproducibility.
