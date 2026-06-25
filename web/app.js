const $ = (id) => document.getElementById(id);

let OPTIONS = null;
let LAST = null;
let view = "equity";
let charts = { main: null, rsi: null, dd: null };

async function loadOptions() {
  const res = await fetch("/api/options");
  OPTIONS = await res.json();

  for (const s of OPTIONS.symbols) $("symbol").add(new Option(s, s));
  for (const i of OPTIONS.intervals) $("interval").add(new Option(i.label, i.value));
  for (const s of OPTIONS.strategies) $("strategy").add(new Option(s.label, s.value));

  $("interval").value = "15";
  $("strategy").value = "ma_crossover";
  $("days").value = 10;

  renderParams();
  $("strategy").addEventListener("change", renderParams);
}

function renderParams() {
  const stratVal = $("strategy").value;
  const strat = OPTIONS.strategies.find((s) => s.value === stratVal);
  const box = $("params");
  box.innerHTML = "";
  if (!strat || !strat.params) return;

  for (const p of strat.params) {
    const field = document.createElement("div");
    field.className = "field";

    const label = document.createElement("label");
    label.textContent = p.label;
    label.htmlFor = "param_" + p.key;

    const input = document.createElement("input");
    input.type = "number";
    input.id = "param_" + p.key;
    input.dataset.key = p.key;
    input.value = p.default;
    input.min = p.min;
    input.max = p.max;

    field.appendChild(label);
    field.appendChild(input);
    box.appendChild(field);
  }
}

function collectParams() {
  const params = {};
  for (const input of $("params").querySelectorAll("input[data-key]")) {
    const v = parseInt(input.value, 10);
    if (!Number.isNaN(v)) params[input.dataset.key] = v;
  }
  return params;
}

function setStatus(msg, isError = false) {
  const el = $("status");
  el.textContent = msg;
  el.classList.toggle("error", isError);
}

async function runBacktest() {
  const body = {
    symbol: $("symbol").value,
    interval: $("interval").value,
    strategy: $("strategy").value,
    days: parseInt($("days").value, 10) || 10,
    params: collectParams(),
  };

  $("run").disabled = true;
  $("results").classList.add("hidden");
  setStatus("Submitting job…");

  try {
    const sub = await fetch("/api/backtests", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    const subData = await sub.json();
    if (!sub.ok) throw new Error(subData.error || "submit failed");

    const job = await poll(subData.id);
    if (job.status === "error") throw new Error(job.error || "backtest failed");

    LAST = job.result;
    render(LAST);
    setStatus("");
  } catch (err) {
    setStatus(err.message, true);
  } finally {
    $("run").disabled = false;
  }
}

async function poll(id) {
  for (let attempt = 0; attempt < 200; attempt++) {
    const res = await fetch(`/api/backtests/${id}`);
    const job = await res.json();
    if (job.status === "done" || job.status === "error") return job;
    setStatus(job.status === "running" ? "Running backtest…" : "Queued…");
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error("timed out waiting for backtest");
}

const fmtPct = (x) => (x * 100).toFixed(2) + "%";

function render(result) {
  const m = result.metrics;

  $("sharpe").textContent = m.sharpe_ratio.toFixed(2);
  $("maxdd").textContent = fmtPct(m.max_drawdown);

  const tr = $("totalreturn");
  tr.textContent = fmtPct(m.total_return);
  tr.className = "metric-value " + (m.total_return >= 0 ? "pos" : "neg");

  $("numtrades").textContent = m.num_trades;
  $("bars").textContent = result.bars;

  const paramStr = Object.entries(result.request.params || {})
    .map(([k, v]) => `${k}=${v}`)
    .join(" ");
  $("runMeta").textContent =
    `${result.request.symbol} · ${result.request.interval}m · ${result.request.strategy}` +
    (paramStr ? ` (${paramStr})` : "") + ` · ` +
    `${result.from.slice(0, 10)} → ${result.to.slice(0, 10)} · ` +
    `initial $${m.initial_equity.toLocaleString()} → final $${m.final_equity.toFixed(2)} · ` +
    `computed in ${result.build_ms} ms`;

  drawMain(result);
  drawRSI(result);
  drawDrawdown(result);

  $("results").classList.remove("hidden");
}

const COLORS = {
  equity: "#f7a600",
  price: "#5b9dff",
  buy: "#16c784",
  sell: "#ea3943",
  ind: ["#f7a600", "#c77dff", "#16c784", "#ff9f43"],
  dd: "#ea3943",
  grid: "#232b36",
  tick: "#8b96a5",
};

function tradeMarkers(result, yLookup) {
  const buys = [], sells = [];
  for (const t of result.trades ?? []) {
    const y = yLookup(t);
    if (y === undefined || y === null) continue;
    (t.side === "BUY" ? buys : sells).push({ x: t.time, y });
  }
  return { buys, sells };
}

function baseScales() {
  return {
    x: {
      type: "time",
      time: { tooltipFormat: "MMM d, HH:mm" },
      ticks: { color: COLORS.tick, maxTicksLimit: 8 },
      grid: { color: COLORS.grid },
    },
    y: { ticks: { color: COLORS.tick }, grid: { color: COLORS.grid } },
  };
}

function zoomPlugin() {
  return {
    zoom: {
      wheel: { enabled: true },
      pinch: { enabled: true },
      drag: { enabled: false },
      mode: "x",
    },
    pan: { enabled: true, mode: "x" },
  };
}

function drawMain(result) {
  const ctx = $("mainChart").getContext("2d");
  if (charts.main) charts.main.destroy();

  const showInd = $("showIndicators").checked;
  const datasets = [];

  if (view === "equity") {
    const equity = result.equity_curve.map((p) => ({ x: p.time, y: p.equity }));
    datasets.push({
      label: "Equity ($)",
      data: equity,
      borderColor: COLORS.equity,
      backgroundColor: "rgba(247,166,0,0.08)",
      borderWidth: 2, pointRadius: 0, fill: true, tension: 0.05,
    });
    const eqByTime = new Map(result.equity_curve.map((p) => [p.time, p.equity]));
    const { buys, sells } = tradeMarkers(result, (t) => eqByTime.get(t.time));
    pushMarkers(datasets, buys, sells);
  } else {
    const price = result.price.map((p) => ({ x: p.time, y: p.close }));
    datasets.push({
      label: "Close",
      data: price,
      borderColor: COLORS.price,
      borderWidth: 1.5, pointRadius: 0, fill: false, tension: 0,
    });

    if (showInd && result.indicators) {
      let ci = 0;
      for (const [label, series] of Object.entries(result.indicators)) {
        if (label.startsWith("RSI")) continue;
        datasets.push({
          label,
          data: series.map((p) => ({ x: p.time, y: p.value })),
          borderColor: COLORS.ind[ci % COLORS.ind.length],
          borderWidth: 1.5, pointRadius: 0, fill: false, tension: 0,
        });
        ci++;
      }
    }

    const pxByTime = new Map(result.price.map((p) => [p.time, p.close]));
    const { buys, sells } = tradeMarkers(result, (t) => pxByTime.get(t.time));
    pushMarkers(datasets, buys, sells);
  }

  charts.main = new Chart(ctx, {
    type: "line",
    data: { datasets },
    options: {
      responsive: true, maintainAspectRatio: false,
      interaction: { mode: "nearest", intersect: false },
      scales: baseScales(),
      plugins: {
        legend: { labels: { color: "#e6edf3", usePointStyle: true } },
        zoom: zoomPlugin(),
      },
    },
  });
}

function pushMarkers(datasets, buys, sells) {
  datasets.push({
    label: "Buy", data: buys, showLine: false,
    pointStyle: "triangle", pointRadius: 7,
    pointBackgroundColor: COLORS.buy, pointBorderColor: COLORS.buy,
  });
  datasets.push({
    label: "Sell", data: sells, showLine: false,
    pointStyle: "triangle", rotation: 180, pointRadius: 7,
    pointBackgroundColor: COLORS.sell, pointBorderColor: COLORS.sell,
  });
}

function drawRSI(result) {
  const wrap = $("rsiWrap");
  const rsiEntry = result.indicators
    ? Object.entries(result.indicators).find(([label]) => label.startsWith("RSI"))
    : null;

  if (!rsiEntry) {
    wrap.classList.add("hidden");
    if (charts.rsi) { charts.rsi.destroy(); charts.rsi = null; }
    return;
  }
  wrap.classList.remove("hidden");

  const [label, series] = rsiEntry;
  const data = series.map((p) => ({ x: p.time, y: p.value }));
  const bands = result.rsi_bands || { oversold: 30, overbought: 70 };
  const xs = data.length ? [data[0].x, data[data.length - 1].x] : [];

  const ctx = $("rsiChart").getContext("2d");
  if (charts.rsi) charts.rsi.destroy();
  charts.rsi = new Chart(ctx, {
    type: "line",
    data: {
      datasets: [
        { label, data, borderColor: "#c77dff", borderWidth: 1.5, pointRadius: 0 },
        { label: `Overbought (${bands.overbought})`, data: xs.map((x) => ({ x, y: bands.overbought })), borderColor: COLORS.sell, borderWidth: 1, borderDash: [5, 4], pointRadius: 0 },
        { label: `Oversold (${bands.oversold})`, data: xs.map((x) => ({ x, y: bands.oversold })), borderColor: COLORS.buy, borderWidth: 1, borderDash: [5, 4], pointRadius: 0 },
      ],
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      scales: {
        x: { type: "time", ticks: { color: COLORS.tick, maxTicksLimit: 8 }, grid: { color: COLORS.grid } },
        y: { min: 0, max: 100, ticks: { color: COLORS.tick, stepSize: 25 }, grid: { color: COLORS.grid } },
      },
      plugins: { legend: { labels: { color: "#e6edf3", usePointStyle: true } }, zoom: zoomPlugin() },
    },
  });
}

function drawDrawdown(result) {
  let peak = -Infinity;
  const data = result.equity_curve.map((p) => {
    if (p.equity > peak) peak = p.equity;
    const dd = peak > 0 ? (p.equity - peak) / peak : 0;
    return { x: p.time, y: dd * 100 };
  });

  const ctx = $("ddChart").getContext("2d");
  if (charts.dd) charts.dd.destroy();
  charts.dd = new Chart(ctx, {
    type: "line",
    data: {
      datasets: [{
        label: "Drawdown (%)",
        data,
        borderColor: COLORS.dd,
        backgroundColor: "rgba(234,57,67,0.15)",
        borderWidth: 1.5, pointRadius: 0, fill: true, tension: 0.05,
      }],
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      scales: {
        x: { type: "time", ticks: { color: COLORS.tick, maxTicksLimit: 8 }, grid: { color: COLORS.grid } },
        y: { ticks: { color: COLORS.tick }, grid: { color: COLORS.grid } },
      },
      plugins: { legend: { labels: { color: "#e6edf3", usePointStyle: true } }, zoom: zoomPlugin() },
    },
  });
}

function syncIndicatorsToggle() {
  const wrap = $("showIndicatorsWrap");
  const show = view === "price";
  wrap.classList.toggle("hidden", !show);
}

$("viewToggle").addEventListener("click", (e) => {
  const btn = e.target.closest(".toggle");
  if (!btn) return;
  view = btn.dataset.view;
  for (const b of $("viewToggle").querySelectorAll(".toggle")) b.classList.toggle("active", b === btn);
  syncIndicatorsToggle();
  if (LAST) drawMain(LAST);
});

$("showIndicators").addEventListener("change", () => { if (LAST) drawMain(LAST); });

$("resetZoom").addEventListener("click", () => {
  for (const c of Object.values(charts)) if (c && c.resetZoom) c.resetZoom();
});

$("run").addEventListener("click", runBacktest);
syncIndicatorsToggle();
loadOptions().catch((e) => setStatus("Failed to load options: " + e.message, true));
