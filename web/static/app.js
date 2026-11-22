"use strict";

const STATES = ["up", "flapping", "down", "unknown"];
const STATE_LABELS = { up: "Up", flapping: "Flapping", down: "Down", unknown: "No data" };
const MIN_SCALE = 0.3;
const MAX_SCALE = 1;
const TICKER_SPEED = 20; // pixels per second
const INCIDENT_LABELS = { down: "Down", flapping: "Flapping", resolved: "Resolved" };

let refreshSeconds = 15;
let retentionText = "24h";
let timer = null;
let nextRefreshAt = 0;
let countdownSeconds = null;

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function tooltip(ep) {
  const lines = [ep.name, `State: ${STATE_LABELS[ep.state]}`];
  if (ep.lastDuration) lines.push(`Response: ${ep.lastDuration}`);
  if (ep.lastTimestamp) lines.push(`Checked: ${new Date(ep.lastTimestamp).toLocaleString()}`);
  if (ep.recent.length) lines.push(`Recent: ${ep.recent.map((ok) => (ok ? "✓" : "✗")).join("")}`);
  for (const err of ep.errors || []) lines.push(`Error: ${err}`);
  return lines.join("\n");
}

function renderCounts(groups) {
  const totals = Object.fromEntries(STATES.map((s) => [s, 0]));
  for (const g of groups) for (const ep of g.endpoints) totals[ep.state]++;

  const counts = document.getElementById("counts");
  counts.replaceChildren(
    ...STATES.filter((s) => s === "up" || totals[s] > 0).map((s) => {
      const span = el("span", null, `${totals[s]} ${STATE_LABELS[s]}`);
      span.style.setProperty("--dot", `var(--${s})`);
      return span;
    })
  );
}

function renderGroups(groups) {
  const main = document.getElementById("groups");
  main.replaceChildren(
    ...groups.map((g) => {
      const card = el("section", "group");
      const upCount = g.endpoints.filter((ep) => ep.state === "up").length;
      if (g.endpoints.some((ep) => ep.state === "down")) card.classList.add("has-down");

      const heading = el("h2", null, g.name);
      heading.append(el("span", "group-count", `${upCount}/${g.endpoints.length}`));

      const pills = el("div", "pills");
      for (const ep of g.endpoints) {
        const pill = el("a", `pill ${ep.state}`, ep.label);
        pill.href = `/endpoints/${encodeURI(ep.key)}`;
        pill.title = tooltip(ep);
        pills.append(pill);
      }

      card.append(heading, pills);
      return card;
    })
  );
}

function fits() {
  const doc = document.documentElement;
  return doc.scrollHeight <= window.innerHeight && doc.scrollWidth <= window.innerWidth;
}

// Find the largest scale at which the whole dashboard fits the viewport.
function fitToScreen() {
  const root = document.documentElement.style;
  let lo = MIN_SCALE;
  let hi = MAX_SCALE;
  for (let i = 0; i < 12; i++) {
    const mid = (lo + hi) / 2;
    root.setProperty("--scale", mid);
    if (fits()) lo = mid;
    else hi = mid;
  }
  root.setProperty("--scale", lo);
}

function formatDuration(ms) {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return m % 60 ? `${h}h ${m % 60}m` : `${h}h`;
  const d = Math.floor(h / 24);
  return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
}

function formatTime(iso) {
  const d = new Date(iso);
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  if (d.toDateString() === new Date().toDateString()) return time;
  return `${d.toLocaleDateString([], { month: "short", day: "numeric" })} ${time}`;
}

function failureRow(f) {
  const ongoing = f.state === "down";
  const row = el("a", `failure ${f.state}`);
  row.href = `/endpoints/${encodeURI(f.key)}`;
  row.title = `${f.name}\n${f.reason}`;

  const end = ongoing ? Date.now() : new Date(f.resolved).getTime();
  const span = formatDuration(end - new Date(f.start).getTime());
  const checks = `${f.count} failed check${f.count === 1 ? "" : "s"}`;
  const duration = ongoing ? `down ${span}, ${checks}` : `recovered after ${span}, ${checks}`;

  row.append(
    el("span", "badge", INCIDENT_LABELS[f.state]),
    el("span", "time", formatTime(ongoing ? f.start : f.resolved)),
    el("span", "failure-group", f.group || "Ungrouped"),
    el("span", "label", f.label),
    el("span", "duration", duration)
  );
  if (f.reason) row.lastChild.append(el("span", "reason", ` · ${f.reason}`));
  return row;
}

// Ticker scrolls the failure list upward in a seamless loop when it overflows.
const ticker = {
  node: null,
  track: null,
  failures: [],
  offset: 0,
  loopHeight: 0,
  paused: false,
  last: 0,
};

function renderFailures(failures) {
  ticker.failures = failures;
  const counts = { down: 0, flapping: 0, resolved: 0 };
  for (const f of failures) counts[f.state]++;
  const parts = Object.entries(counts)
    .filter(([, n]) => n > 0)
    .map(([state, n]) => `${n} ${INCIDENT_LABELS[state].toLowerCase()}`);
  document.getElementById("failure-count").textContent = parts.length
    ? `${parts.join(", ")} · resolved entries clear after ${retentionText}`
    : "";
  layoutTicker();
}

function layoutTicker() {
  const track = el("div", "ticker-track");
  ticker.node.replaceChildren(track);
  ticker.track = track;
  ticker.loopHeight = 0;

  if (!ticker.failures.length) {
    track.append(el("div", "ticker-empty", `No failures in the last ${retentionText}.`));
    return;
  }

  const copy = el("div");
  copy.append(...ticker.failures.map(failureRow));
  track.append(copy);
  if (copy.offsetHeight <= ticker.node.clientHeight) return;

  // Duplicate the list so scrolling past the end wraps without a jump.
  const gap = el("div", "ticker-gap");
  track.append(gap, copy.cloneNode(true));
  ticker.loopHeight = copy.offsetHeight + gap.offsetHeight;
  ticker.offset %= ticker.loopHeight;
  moveTicker();
}

function moveTicker() {
  ticker.track.style.transform = `translateY(${-ticker.offset}px)`;
}

function tickerFrame(ts) {
  const dt = ticker.last ? (ts - ticker.last) / 1000 : 0;
  ticker.last = ts;
  if (ticker.loopHeight && !ticker.paused) {
    ticker.offset = (ticker.offset + TICKER_SPEED * dt) % ticker.loopHeight;
    moveTicker();
  }
  updateCountdown();
  requestAnimationFrame(tickerFrame);
}

// Fill the refresh bar and count down the seconds until the next refresh.
function updateCountdown() {
  const remaining = Math.max(0, nextRefreshAt - Date.now());
  const fraction = nextRefreshAt ? 1 - remaining / (refreshSeconds * 1000) : 1;
  document.getElementById("refresh-progress").style.transform = `scaleX(${fraction})`;

  const seconds = Math.ceil(remaining / 1000);
  if (seconds !== countdownSeconds) {
    countdownSeconds = seconds;
    document.getElementById("countdown").textContent = nextRefreshAt
      ? seconds > 0 ? `refresh in ${seconds}s` : "refreshing…"
      : "";
  }
}

async function refresh() {
  nextRefreshAt = 0;
  try {
    const resp = await fetch("/dash/api/summary", { cache: "no-store" });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const data = await resp.json();

    if (data.title) {
      document.title = data.title;
      document.getElementById("title").textContent = data.title;
    }
    document.getElementById("version").textContent = `custom-gatus-dashboard v${data.version}`;
    document.getElementById("updated").textContent =
      `Updated ${new Date(data.updated).toLocaleTimeString()}`;
    document.getElementById("stale").hidden = !data.stale;

    renderCounts(data.groups);
    renderGroups(data.groups);
    fitToScreen();
    if (data.failureRetentionSeconds) retentionText = formatDuration(data.failureRetentionSeconds * 1000);
    renderFailures(data.failures || []);

    if (data.refreshSeconds) refreshSeconds = data.refreshSeconds;
  } catch (err) {
    console.error("refresh failed", err);
    document.getElementById("stale").hidden = false;
  }
  scheduleNext();
}

function scheduleNext() {
  clearTimeout(timer);
  nextRefreshAt = Date.now() + refreshSeconds * 1000;
  timer = setTimeout(refresh, refreshSeconds * 1000);
}

let resizeTimer = null;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    fitToScreen();
    layoutTicker();
  }, 100);
});

ticker.node = document.getElementById("ticker");
ticker.node.addEventListener("mouseenter", () => (ticker.paused = true));
ticker.node.addEventListener("mouseleave", () => (ticker.paused = false));
requestAnimationFrame(tickerFrame);

refresh();
