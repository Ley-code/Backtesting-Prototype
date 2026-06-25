# Bybit Backtester — Prototype

An event-driven backtesting engine in Go that replays **real Bybit historical
data** and reports **Sharpe ratio** and **maximum drawdown**, with a simple
single-page UI to run backtests and view the equity curve.

This is a working prototype of the full backtesting platform. It is deliberately
scoped to the prototype requirements, but the architecture is the *real* one —
nothing here is a throwaway that would be rebuilt for production.

---

## Prototype scope (as requested)

| Dimension | Values |
|---|---|
| **Products** | BTCUSDT, ETHUSDT |
| **Timeframes** | 1 min, 5 min, 15 min |
| **Metrics** | Max Drawdown, Sharpe ratio |
| **Strategies** | MA Crossover, RSI Mean-Reversion (my choice — pluggable) |
| **Data source** | Bybit public REST API (`/v5/market/kline`, spot) + on-disk cache |

---

## Quick start

```bash
# from the project root
go run ./cmd/api
# open http://localhost:8080
```

Pick a product, timeframe, and strategy, set a lookback window, and hit **Run
backtest**. First run for a given window fetches from Bybit (a few seconds);
identical reruns are served from cache in ~15ms.

Run the tests:

```bash
go test ./...
```

## Docker / EC2

Build and run locally with Docker:

```bash
docker build -t bybit-backtester .
docker run --rm -p 8080:8080 -v bybit-backtester-cache:/app/.cache bybit-backtester
```

Or use Docker Compose (recommended for EC2 because it keeps the cache volume and
auto-restarts the container):

```bash
docker compose up -d --build
```

Then open `http://<server-ip>:8080`.

The container uses these environment variables:

- `ADDR` — HTTP listen address inside the container (defaults to `:8080`)
- `CACHE_DIR` — writable directory for Bybit historical-data cache
- `WEB_DIR` — location of the bundled frontend assets

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

If you want the app reachable publicly, allow inbound TCP `8080` in the EC2
security group (or place it behind Nginx / an AWS load balancer later).

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
    │                   │ equity curve + trade log
    │                   ▼
    │            Metrics (Sharpe, Max Drawdown)
    ▼
Bybit Data Layer  ──►  on-disk JSON cache
(public REST, paginated)
```

### The five components (the mental model)

1. **Data Handler** (`internal/data`) — fetches OHLCV bars from Bybit, paginates
   the 1000-bar API limit, caches to disk, and streams bars **one at a time, in
   chronological order**. This is where look-ahead bias lives or dies.
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
| **Strategy = interface** | `Strategy` with `OnBar` | Adding a new trading system is **writing one file** in `internal/strategy`; the engine never changes. The two strategies in the UI are the proof. |
| **Data = interface** | `DataHandler` with `Next()` | The engine doesn't know data comes from Bybit. Swapping in a live websocket feed or CSV later changes nothing upstream. |
| **Realism** | Broker models fee + adverse slippage | A backtest with zero fees and perfect fills is a fantasy. Modelling both is why these results are believable (see below). |
| **Data fetch** | Live Bybit + disk cache | Real integration, but offline-safe and instant on rerun. Cache key snaps to the bar boundary so repeated runs reuse it. |
| **Job/result store** | In-memory (prototype) | Production is Postgres: `runs`, `trades`, `equity_curve`, `metrics`. Called out honestly as the next step, not hidden. |
| **Async API** | POST enqueues → GET polls | This is what actually exercises the worker pool, and it's the model that scales to long-running backtests. |

---

## About the results (read this before the demo)

Results vary by strategy, parameters, timeframe, and the market window — which is
exactly what an honest backtester should show. A few things to expect and to be
ready to explain:

- **The numbers respond to the parameters.** On BTC 15m over 30 days, a tighter
  MA (10/30) over-trades (~120 trades) and bleeds out on fees, while a wider MA
  (20/50) trades less (~66 trades) and loses less. **Fewer trades → less fee
  drag** — visible directly in the metrics. The RSI strategy, trading rarely
  (~4 trades), can come out **clearly positive** on the same window. There is no
  single "the result" — that's the point of a backtester.
- **Costs are real.** The broker charges a fee on every trade and applies
  adverse slippage, so high-frequency whipsaw strategies are correctly punished.
- **Large Sharpe magnitudes** come from **annualizing minute-bar returns**
  (×√525,600 for 1m). That's the standard formula; sign and ordering are what
  matter and they're consistent.
- **Fixing look-ahead made results more believable, not just more correct.**
  Filling at the next bar's open (instead of the close the signal was computed
  on) removes a free, unrealistic edge — so the equity curves you see are ones a
  real trader could have actually achieved.

**This is the whole point.** A backtester's job is to tell you the truth —
including when a strategy loses money after costs, and including when it doesn't.
An engine that always prints green is the one you shouldn't trust. Adding or
tuning a strategy is one file / a few inputs; the engine's value is that it
won't flatter any of them.

---

## Project layout

```
/cmd/api              → main, server + worker pool wiring
/internal/engine      → event types, time-ordered queue, the run loop + interfaces
/internal/data        → Bybit fetch (paginated) + disk cache (DataHandler)
/internal/strategy    → MA crossover + RSI (both implement Strategy)
/internal/broker      → execution simulation (fees, slippage)
/internal/portfolio   → cash, position, equity curve, trade log
/internal/metrics     → Sharpe + Max Drawdown (+ unit tests)
/internal/api         → Gin handlers, worker pool, job store, per-run wiring
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

## What production (the full project) adds

- **Postgres** for runs, trades, equity curves, and metrics (durable, queryable).
- **More metrics** (win rate, profit factor, CAGR) and more strategies.
- **Configurable date ranges** and parameter sweeps.
- **Docker** packaging and deployment.
- Golden-file tests pinning a known strategy/dataset for reproducibility.
```
