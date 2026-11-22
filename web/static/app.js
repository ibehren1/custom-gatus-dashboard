"use strict";

const STATES = ["up", "flapping", "down", "unknown"];
const STATE_LABELS = { up: "Up", flapping: "Flapping", down: "Down", unknown: "No data" };
const MIN_SCALE = 0.3;
const MAX_SCALE = 1;

let refreshSeconds = 15;
let timer = null;

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

async function refresh() {
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
    document.getElementById("stale").hidden = true;

    renderCounts(data.groups);
    renderGroups(data.groups);

    if (data.refreshSeconds && data.refreshSeconds !== refreshSeconds) {
      refreshSeconds = data.refreshSeconds;
      schedule();
    }
  } catch (err) {
    console.error("refresh failed", err);
    document.getElementById("stale").hidden = false;
  }
  fitToScreen();
}

function schedule() {
  clearInterval(timer);
  timer = setInterval(refresh, refreshSeconds * 1000);
}

let resizeTimer = null;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(fitToScreen, 100);
});

refresh();
schedule();
