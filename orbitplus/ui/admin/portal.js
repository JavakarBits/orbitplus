import { adminPortalService, ADMIN_ENDPOINTS } from "/orbitplus/admin/portal-service.js";

const mount = document.getElementById("admin-portal");
const routes = [
  { id: "dashboard", label: "Dashboard", icon: "◈" },
  { id: "queues", label: "Queues", icon: "≣" },
  { id: "workers", label: "Workers", icon: "⚙" },
  { id: "tripdetails", label: "TripDetails", icon: "✦" },
  { id: "trip-analyzer", label: "Trip Analyzer", icon: "⌕" },
  { id: "inventory-events", label: "Inventory Events", icon: "⇄" },
  { id: "failures", label: "Failures & DLQ", icon: "⚠" },
  { id: "dragonfly", label: "Dragonfly (Cache)", icon: "◆" },
  { id: "cassandra", label: "Cassandra (Metadata)", icon: "▤" },
  { id: "reports", label: "Reports", icon: "▥" }
];
const SERIES = ["#6c5ce7", "#3f7ff0", "#16a06a", "#dd9016", "#d94b5c", "#1899b0", "#a05bd0", "#7b879e"];

let controller;
let overview;
const trip = { operatorCode: "", tripCode: "", tripDate: "", fromStation: "", toStation: "", state: "idle", message: "", result: null };
const TRIP_FIELDS = [["trip-operator", "operatorCode"], ["trip-code", "tripCode"], ["trip-date", "tripDate"], ["trip-from", "fromStation"], ["trip-to", "toStation"]];

const esc = (value) => String(value ?? "—").replace(/[&<>"]/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[char]);
const num = (value) => Number(value || 0).toLocaleString();
const share = (value, total) => total ? `${(value / total * 100).toFixed(2)}%` : "0%";
const clock = (value) => value ? new Date(value).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "—";
const capability = (name) => overview?.[name] || { status: "source_unavailable", detail: "This telemetry is unavailable." };
const live = (name) => capability(name).status === "available";
const data = (name) => capability(name).data || {};
const route = () => { const id = location.pathname.split("/").filter(Boolean).at(-1); return routes.some((item) => item.id === id) ? id : "dashboard"; };
const href = (id) => id === "dashboard" ? "/orbitplus/admin" : `/orbitplus/admin/${id}`;
const chip = (value, tone) => `<span class="chip ${tone || ""}">${esc(value)}</span>`;
const na = (name, link, linkText) => { const source = capability(name); return `<div class="na-box"><div><b>${esc(source.status.replaceAll("_", " "))}</b>${esc(source.detail)}${link ? `<br><a href="${link}">${esc(linkText)}</a>` : ""}</div></div>`; };

function shell(current, content) {
  const nav = routes.map((item) => `<a href="${href(item.id)}"${current === item.id ? ' aria-current="page"' : ""}><i>${item.icon}</i>${esc(item.label)}</a>`).join("");
  const degraded = ["queueMetrics", "rabbitmq", "tripDetailsFreshness"].filter((name) => capability(name).status === "source_unavailable");
  const status = live("runtime") ? (degraded.length ? "Degraded telemetry" : "Core services responding") : "Runtime unavailable";
  return `<div class="app"><aside class="side"><a class="side-brand" href="/orbitplus/admin"><i>◆</i>OrbitPlus</a><p class="side-label">ADMIN PORTAL</p><nav class="nav">${nav}</nav><div class="side-foot"><p class="side-label" style="padding:0">SYSTEM STATUS</p><b><i></i>${esc(status)}</b><small>OrbitPlus · ${esc(data("runtime").goVersion || "runtime")}</small></div></aside><main class="main">${top(current)}<div class="body">${content}</div></main></div>`;
}

function top(current) {
  const page = routes.find((item) => item.id === current);
  const subtitle = current === "dashboard" ? "Live overview of the TripDetails refresh system" : "Live data where a backend source exists";
  const failures = live("queueMetrics") ? (data("queueMetrics").recentFailures || []).length : 0;
  return `<header class="top"><div class="top-l"><button class="burger" id="burger" type="button" title="Navigation">☰</button><div><h1>${esc(page?.label || "Dashboard")}</h1><p>${esc(subtitle)}</p></div></div><div class="top-r"><span class="pill">◷ Last 24 hours</span><button class="bell" id="bell" type="button" title="Failed records in sample">⌁${failures ? `<span>${failures}</span>` : ""}</button><span class="user"><i>A</i>admin</span></div></header>`;
}

function kpis() {
  const queue = data("queueMetrics");
  const rabbit = data("rabbitmq");
  const fresh = data("tripDetailsFreshness");
  const cards = [];
  if (live("tripDetailsFreshness")) cards.push(["✦", "Active Trip Stages", num(fresh.activeTripStages), "Future trip dates in route metadata", "t-violet"]);
  else cards.push(["✦", "Active Trip Stages", "Unavailable", capability("tripDetailsFreshness").detail, "t-slate na"]);
  if (live("queueMetrics")) {
    cards.push(["⇄", "Queue Updates (24h sample)", num(queue.loadedRecords), queue.scope, "t-green"]);
    cards.push(["◉", "Orionmax Events (24h sample)", num(queue.loadedRecords), "All tracked Queue Metrics originate in Orionmax", "t-blue"]);
    cards.push(["↗", "Queued Records (24h sample)", num(queue.queued), "Lifecycle QUEUED records", "t-cyan"]);
    cards.push(["✔", "Completed (24h sample)", num(queue.completed), "Lifecycle COMPLETED records", "t-green"]);
    cards.push(["⚠", "Failed / DEAD (24h sample)", num(queue.dead), "Application DEAD records, not broker DLQ depth", "t-red"]);
  } else {
    cards.push(["⇄", "Queue Updates (24h)", "Unavailable", capability("queueMetrics").detail, "t-slate na"]);
  }
  if (live("rabbitmq")) {
    cards.push(["≋", "Pending Broker Messages", num((rabbit.queues || []).reduce((sum, item) => sum + Number(item.messages || 0), 0)), "Live RabbitMQ, configured vhost", "t-amber"]);
    cards.push(["⚙", "Active Workers", num((rabbit.consumers || []).length), "Live RabbitMQ consumers", "t-violet"]);
  } else {
    cards.push(["⚙", "Active Workers", "Unavailable", capability("rabbitmq").detail, "t-slate na"]);
  }
  return `<section class="kpis">${cards.map(([icon, label, value, note, tone]) => `<article class="kpi ${tone}"><i>${icon}</i><div><span>${esc(label)}</span><strong>${esc(value)}</strong><small>${esc(note)}</small></div></article>`).join("")}</section>`;
}

function queueDepth() {
  if (!live("rabbitmq")) return na("rabbitmq");
  const queues = data("rabbitmq").queues || [];
  if (!queues.length) return `<div class="na-box"><div>RabbitMQ reported no queues in the configured vhost.</div></div>`;
  const max = Math.max(1, ...queues.map((item) => Number(item.messages || 0)));
  const legend = `<div class="legend"><span><i style="background:${SERIES[1]}"></i>Ready</span><span><i style="background:${SERIES[3]}"></i>Unacknowledged</span></div>`;
  const rows = queues.map((item) => {
    const ready = Number(item.ready || 0);
    const unacked = Number(item.unacknowledged || 0);
    return `<div class="row"><b title="${esc(item.name)}">${esc(item.name)}</b><div class="track" style="display:flex"><i style="width:${ready / max * 100}%;background:${SERIES[1]}"></i><i style="width:${unacked / max * 100}%;background:${SERIES[3]}"></i></div><strong>${num(item.messages)}</strong></div>`;
  }).join("");
  return `${legend}<div class="rows">${rows}</div><p class="note">Point-in-time broker depth. No historical depth series is stored, so this is not a time chart.</p>`;
}

function freshness() {
  if (!live("tripDetailsFreshness")) return na("tripDetailsFreshness");
  const value = data("tripDetailsFreshness");
  const total = Number(value.activeTripStages || 0);
  const bands = [["Fresh ( < 10 min )", Number(value.fresh || 0), "#16a06a"], ["Aging ( 10 - 30 min )", Number(value.aging || 0), "#dd9016"], ["Stale ( 30 - 60 min )", Number(value.stale || 0), "#e8703f"], ["Critical ( > 60 min )", Number(value.critical || 0), "#d94b5c"]];
  let cursor = 0;
  const stops = bands.map(([, count, color]) => { const start = total ? cursor / total * 100 : 0; cursor += count; const end = total ? cursor / total * 100 : 0; return `${color} ${start}% ${end}%`; }).join(",");
  return `<div class="donut-box"><div class="donut" style="background:${total ? `conic-gradient(${stops})` : "#eceff6"}"><div><strong>${num(total)}</strong><small>Future stages</small></div></div><div class="f-legend">${bands.map(([label, count, color]) => `<div><i style="background:${color}"></i><span><b>${esc(label)}</b><small>${num(count)} (${share(count, total)})</small></span></div>`).join("")}</div></div><p class="note">${esc(value.scope)}</p>`;
}

function workerStatus(limit) {
  if (!live("rabbitmq")) return na("rabbitmq");
  const consumers = data("rabbitmq").consumers || [];
  if (!consumers.length) return `<div class="na-box"><div>RabbitMQ reported no consumers in the configured vhost.</div></div>`;
  const state = (value) => /up|running|active/i.test(value || "") ? ["Healthy", ""] : /wait|idle/i.test(value || "") ? ["Idle", "warn"] : [value || "Unknown", "na"];
  const rows = consumers.slice(0, limit || consumers.length).map((item) => {
    const [label, tone] = state(item.status);
    return `<tr><td title="${esc(item.tag)}">${esc(item.tag || item.connection)}</td><td>${chip(label, tone)}</td><td>${esc(item.queue)}</td><td>${num(item.prefetchCount)}</td><td>${esc(item.user)}</td></tr>`;
  }).join("");
  const healthy = consumers.filter((item) => /up|running|active/i.test(item.status || "")).length;
  return `<div class="t-wrap"><table><thead><tr><th>Consumer tag</th><th>Status</th><th>Queue</th><th>Prefetch</th><th>User</th></tr></thead><tbody>${rows}</tbody></table></div><div class="totals"><span>Total workers: <b>${num(consumers.length)}</b></span><span>Healthy: <b>${num(healthy)}</b></span><span>Other: <b>${num(consumers.length - healthy)}</b></span></div><p class="note">Processed and failed per-worker counters are not exposed by RabbitMQ and are therefore not shown.</p>`;
}

function areaGraph(hours, actions) {
  if (!hours.length) return `<div class="na-box"><div>No Queue Metrics records in the loaded 24 hour sample.</div></div>`;
  const width = 720;
  const height = 210;
  const left = 42;
  const right = 14;
  const topPad = 14;
  const bottom = 32;
  const plotW = width - left - right;
  const plotH = height - topPad - bottom;
  const max = Math.max(1, ...hours.map((item) => Number(item.count || 0)));
  const xAt = (index) => left + index * plotW / Math.max(1, hours.length - 1);
  const yAt = (value) => topPad + plotH - value / max * plotH;
  const totals = {};
  actions.forEach((item) => { totals[item.actionType] = (totals[item.actionType] || 0) + Number(item.count || 0); });
  const names = Object.keys(totals).sort((a, b) => totals[b] - totals[a]);
  const lookup = new Map(actions.map((item) => [`${item.hour}|${item.actionType}`, Number(item.count || 0)]));
  const grid = [0, .5, 1].map((ratio) => { const y = yAt(max * ratio); return `<g><line x1="${left}" x2="${width - right}" y1="${y}" y2="${y}"/><text x="${left - 8}" y="${y + 4}">${num(Math.round(max * ratio))}</text></g>`; }).join("");
  const labels = hours.map((item, index) => index % 4 === 0 || index === hours.length - 1 ? `<text x="${xAt(index)}" y="${height - 10}">${esc(clock(item.hour))}</text>` : "").join("");
  let bands = "";
  if (names.length) {
    const stack = hours.map((item, index) => { let base = 0; const perName = {}; names.forEach((name) => { const count = lookup.get(`${item.hour}|${name}`) || 0; perName[name] = [base, base + count]; base += count; }); return { x: xAt(index), perName }; });
    bands = names.map((name, index) => {
      const color = SERIES[index % SERIES.length];
      const upper = stack.map((point) => `${point.x.toFixed(1)},${yAt(point.perName[name][1]).toFixed(1)}`);
      const lower = stack.map((point) => `${point.x.toFixed(1)},${yAt(point.perName[name][0]).toFixed(1)}`).reverse();
      return `<path class="area" fill="${color}" stroke="${color}" d="M${upper.join(" L")} L${lower.join(" L")} Z"/>`;
    }).join("");
  } else {
    const line = hours.map((item, index) => `${xAt(index).toFixed(1)},${yAt(Number(item.count || 0)).toFixed(1)}`);
    bands = `<path class="area" fill="${SERIES[0]}" stroke="${SERIES[0]}" d="M${line.join(" L")} L${xAt(hours.length - 1)},${topPad + plotH} L${left},${topPad + plotH} Z"/>`;
  }
  const legend = names.length ? `<div class="legend">${names.map((name, index) => `<span><i style="background:${SERIES[index % SERIES.length]}"></i>${esc(name)}</span>`).join("")}</div>` : "";
  return `${legend}<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="Queue Metrics events by UTC hour"><g class="grid-line">${grid}</g>${bands}<g class="x-lab">${labels}</g></svg>`;
}

function eventCounts() {
  const queue = data("queueMetrics");
  const cells = [["Received", queue.received], ["Queued", queue.queued], ["Completed", queue.completed], ["Failed / DEAD", queue.dead]];
  return `<div class="counts">${cells.map(([label, value]) => `<div><span>${esc(label)}</span><strong>${num(value)}</strong></div>`).join("")}</div>`;
}

function operatorRows() {
  if (!live("queueMetrics")) return na("queueMetrics");
  const operators = data("queueMetrics").operators || [];
  if (!operators.length) return `<div class="na-box"><div>No operator records in the loaded sample.</div></div>`;
  const max = Math.max(1, ...operators.map((item) => item.count));
  return `<div class="rows">${operators.map((item, index) => `<div class="row"><b>${esc(item.operatorCode)}</b><div class="track"><i style="width:${item.count / max * 100}%;background:${SERIES[index % SERIES.length]}"></i></div><strong>${num(item.count)}</strong></div>`).join("")}</div>`;
}

function actionRows() {
  if (!live("queueMetrics")) return na("queueMetrics");
  const actions = data("queueMetrics").actions || [];
  if (!actions.length) return `<div class="na-box"><div>No action records in the loaded sample.</div></div>`;
  const max = Math.max(1, ...actions.map((item) => item.count));
  return `<div class="rows">${actions.map((item, index) => `<div class="row"><b>${esc(item.actionType)}</b><div class="track"><i style="width:${item.count / max * 100}%;background:${SERIES[index % SERIES.length]}"></i></div><strong>${num(item.count)}</strong></div>`).join("")}</div>`;
}

function failuresTable(limit) {
  if (!live("queueMetrics")) return na("queueMetrics");
  const rows = (data("queueMetrics").recentFailures || []).slice(0, limit || undefined).map((item) => `<tr><td>${esc(clock(item.deadLetteredAt || item.completedAt || item.updatedAt))}</td><td>${esc(item.referenceId)}</td><td>${esc(item.activityType)}</td><td>${esc(item.operatorCode)}</td><td>${esc(item.actionType)}</td><td title="${esc(item.failureMessage)}">${esc(item.failureMessage || "No failure message")}</td></tr>`).join("");
  if (!rows) return `<div class="na-box"><div>No failed records in the loaded 24 hour sample.</div></div>`;
  return `<div class="t-wrap"><table><thead><tr><th>Time</th><th>Ref ID</th><th>Activity</th><th>Operator</th><th>Action</th><th>Error</th></tr></thead><tbody>${rows}</tbody></table></div><p class="note">Read-only view. Retry actions require a dedicated authorized backend API.</p>`;
}

function runtimeStats() {
  if (!live("runtime")) return na("runtime");
  const runtime = data("runtime");
  const rows = [["Status", runtime.status], ["Uptime", `${Math.floor(Number(runtime.uptimeSeconds || 0) / 60)} min`], ["Goroutines", num(runtime.goroutines)], ["Heap in use", `${(Number(runtime.heapInUseBytes || 0) / 1048576).toFixed(1)} MB`], ["GOMAXPROCS", num(runtime.goMaxProcs)], ["GC cycles", num(runtime.gcCycles)]];
  if (runtime.goCpuCapacityPercent != null) rows.push(["Go CPU capacity", `${Number(runtime.goCpuCapacityPercent).toFixed(1)}%`]);
  return rows.map(([label, value]) => `<div class="stat"><span>${esc(label)}</span><strong>${esc(value)}</strong></div>`).join("");
}

function dashboard() {
  const queue = data("queueMetrics");
  return `<p class="updated">Last updated: ${esc(overview?.observedAt ? new Date(overview.observedAt).toLocaleString() : "—")} ⟳</p>${kpis()}
  <section class="grid g-3">
    <article class="panel"><div class="p-head"><h2>Queue Depth (Messages)</h2><a href="/orbitplus/admin/queues">View all</a></div>${queueDepth()}</article>
    <article class="panel"><div class="p-head"><h2>TripDetails Freshness</h2></div>${freshness()}</article>
    <article class="panel"><div class="p-head"><h2>Workers Status</h2><a href="/orbitplus/admin/workers">View all</a></div>${workerStatus(6)}</article>
  </section>
  <section class="grid g-a">
    <article class="panel"><div class="p-head"><h2>Action Distribution</h2><span class="tag${live("queueMetrics") ? "" : " na"}">24h sample</span></div>${actionRows()}</article>
    <article class="panel"><div class="p-head"><h2>Inventory Events (24h)</h2><span class="tag${live("queueMetrics") ? "" : " na"}">Queue Metrics</span></div>${live("queueMetrics") ? `${eventCounts()}${areaGraph(queue.hourlyVolumes || [], queue.hourlyActionVolumes || [])}` : na("queueMetrics")}</article>
  </section>
  <section class="grid g-2">
    <article class="panel"><div class="p-head"><h2>Recent Failures</h2><a href="/orbitplus/admin/failures">View all failures & DLQ</a></div>${failuresTable(5)}</article>
    <article class="panel"><div class="p-head"><h2>Master Runtime</h2><span class="tag${live("runtime") ? "" : " na"}">${live("runtime") ? "Healthy" : "unavailable"}</span></div>${runtimeStats()}</article>
  </section>
  <section class="grid">
    <article class="panel"><div class="p-head"><h2>Trip Analyzer / Trip History</h2><a href="/orbitplus/admin/trip-analyzer">Open full view</a></div>${tripAnalyzer()}</article>
  </section>`;
}

function queuesPage() {
  if (!live("rabbitmq")) return `<section class="panel"><div class="p-head"><h2>Queue Monitoring</h2></div>${na("rabbitmq")}</section>`;
  const rabbit = data("rabbitmq");
  const rows = (rabbit.queues || []).map((item) => `<tr><td>${esc(item.name)}</td><td>${num(item.ready)}</td><td>${num(item.unacknowledged)}</td><td>${num(item.messages)}</td><td>${num(item.consumers)}</td><td>${item.durable ? "Yes" : "No"}</td><td>${chip(item.state || "unknown", /run/i.test(item.state || "") ? "" : "warn")}</td></tr>`).join("");
  return `<section class="panel"><div class="p-head"><h2>Queue Monitoring</h2><a href="${ADMIN_ENDPOINTS.rabbitmq}">Broker details</a></div><div class="t-wrap"><table><thead><tr><th>Queue</th><th>Ready</th><th>Unacked</th><th>Messages</th><th>Consumers</th><th>Durable</th><th>State</th></tr></thead><tbody>${rows || '<tr><td colspan="7">No queues in the configured vhost.</td></tr>'}</tbody></table></div><p class="note">Cluster ${esc(rabbit.clusterName)} · broker ${esc(rabbit.version)}</p></section>`;
}

function recordsPage() {
  if (!live("queueMetrics")) return `<section class="panel"><div class="p-head"><h2>Queue Lifecycle Records</h2></div>${na("queueMetrics")}</section>`;
  const rows = (data("queueMetrics").recentJobs || []).map((item) => `<tr><td>${esc(item.referenceId)}</td><td>${esc(item.operatorCode)}</td><td>${esc(item.actionType)}</td><td>${esc(item.activityType)}</td><td>${esc(item.tripDate)}</td><td>${chip(item.queueStatus, item.queueStatus === "DEAD" ? "bad" : item.queueStatus === "COMPLETED" ? "" : "warn")}</td><td>${esc(clock(item.updatedAt))}</td></tr>`).join("");
  return `<section class="panel"><div class="p-head"><h2>Queue Lifecycle Records</h2><span class="tag">24h sample</span></div><div class="t-wrap"><table><thead><tr><th>Ref ID</th><th>Operator</th><th>Action</th><th>Activity</th><th>Trip date</th><th>Status</th><th>Updated</th></tr></thead><tbody>${rows || '<tr><td colspan="7">No records in the loaded sample.</td></tr>'}</tbody></table></div><p class="note">This is Orionmax queue lifecycle state, not a dedicated inventory-event telemetry stream.</p></section>`;
}

const cacheViewer = { category: "all", cursor: 0, items: [], state: "idle", message: "", valueState: "idle", valueKey: "", valueContent: "", valueFound: true };
let cacheKeysController;
let cacheValueController;

function cacheViewerPage() {
  const tabs = [["all", "All"], ["trip", "Trip cache"], ["stage", "Stage cache"], ["busmap", "BusMap cache"], ["other", "Other"]];
  const status = cacheViewer.state === "error" ? "Cache keys are unavailable." : cacheViewer.state === "loading" ? "Loading cache keys…" : `Showing ${num(cacheViewer.items.length)} ${esc(cacheViewer.category)} cache keys. Click Show to load data.`;
  const rows = cacheViewer.items.map((item) => `<tr><td class="df-key">${esc(item.key)}</td><td>${esc(item.category)}</td><td><button class="df-show" type="button" data-cache-key="${esc(item.key)}">Show</button></td></tr>`).join("");
  const empty = cacheViewer.state === "loading" && !cacheViewer.items.length ? "Loading cache keys…" : cacheViewer.state === "error" ? cacheViewer.message || "Unable to load cache keys. Check the Dragonfly connection and try again." : "No cache keys found in this group.";
  const value = cacheViewer.valueState === "idle" ? "" : `<section class="df-value"><div class="df-value-head"><strong>${cacheViewer.valueState === "loading" ? "Loading cache data…" : cacheViewer.valueState === "error" ? "Unable to load cache data" : cacheViewer.valueFound ? "Cache data" : "Cache key no longer exists"}</strong><span>${esc(cacheViewer.valueKey)}</span></div>${cacheViewer.valueState === "done" && cacheViewer.valueFound ? `<pre>${esc(formatCacheValue(cacheViewer.valueContent))}</pre>` : ""}${cacheViewer.valueState === "error" ? `<p>${esc(cacheViewer.message)}</p>` : ""}</section>`;
  return `<section class="panel df-viewer" id="dragonfly-cache-viewer"><div class="p-head"><div><h2>Dragonfly cache viewer</h2><p class="df-subtitle">Browse cache keys by type. Stored data loads only after you click Show.</p></div><button class="df-refresh" id="cache-refresh" type="button">Refresh viewer</button></div><div class="df-tabs" role="tablist" aria-label="Cache groups">${tabs.map(([id, label]) => `<button type="button" role="tab" data-cache-category="${id}" aria-selected="${cacheViewer.category === id}">${label}</button>`).join("")}</div><p class="df-status">${status}</p>${rows ? `<div class="t-wrap"><table><thead><tr><th>Cache key</th><th>Category</th><th></th></tr></thead><tbody>${rows}</tbody></table></div>` : `<div class="df-empty">${esc(empty)}</div>`}<div class="df-actions"><button class="df-load-more" id="cache-load-more" type="button"${cacheViewer.cursor === 0 ? " hidden" : ""}${cacheViewer.state === "loading" ? " disabled" : ""}>Load more keys</button></div>${value}</section>`;
}

function formatCacheValue(content) {
  try { return JSON.stringify(JSON.parse(content), null, 2); } catch { return content; }
}

function bindCacheViewer() {
  document.querySelectorAll("[data-cache-category]").forEach((button) => button.addEventListener("click", () => {
    if (cacheViewer.category === button.dataset.cacheCategory && cacheViewer.state !== "error") return;
    cacheViewer.category = button.dataset.cacheCategory;
    loadCacheViewer(true);
  }));
  document.getElementById("cache-refresh")?.addEventListener("click", () => loadCacheViewer(true));
  document.getElementById("cache-load-more")?.addEventListener("click", () => loadCacheViewer());
  document.querySelectorAll("[data-cache-key]").forEach((button) => button.addEventListener("click", () => showCacheValue(button.dataset.cacheKey)));
}

function paintCacheViewer() {
  const target = document.getElementById("dragonfly-cache-viewer");
  if (target) {
    target.outerHTML = cacheViewerPage();
    bindCacheViewer();
  }
}

async function loadCacheViewer(reset = false) {
  if (!reset && (!cacheViewer.cursor || cacheViewer.state === "loading")) return;
  cacheKeysController?.abort();
  const activeController = new AbortController();
  cacheKeysController = activeController;
  if (reset) {
    cacheViewer.cursor = 0;
    cacheViewer.items = [];
    cacheViewer.valueState = "idle";
    cacheViewer.valueKey = "";
    cacheViewer.valueContent = "";
  }
  cacheViewer.state = "loading";
  cacheViewer.message = "";
  paintCacheViewer();
  try {
    const result = await adminPortalService.cacheKeys({ cursor: cacheViewer.cursor, limit: 25, category: cacheViewer.category, signal: activeController.signal });
    if (cacheKeysController !== activeController) return;
    const seen = new Set(cacheViewer.items.map((item) => item.key));
    for (const item of result.items || []) {
      if (item?.key && !seen.has(item.key)) {
        seen.add(item.key);
        cacheViewer.items.push({ key: item.key, category: item.category || "other" });
      }
    }
    cacheViewer.cursor = Number(result.nextCursor) || 0;
    cacheViewer.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || cacheKeysController !== activeController) return;
    cacheViewer.state = "error";
    cacheViewer.message = error.message || "Unable to load cache keys.";
  } finally {
    if (cacheKeysController === activeController) {
      cacheKeysController = undefined;
      paintCacheViewer();
    }
  }
}

async function showCacheValue(key) {
  cacheValueController?.abort();
  const activeController = new AbortController();
  cacheValueController = activeController;
  cacheViewer.valueKey = key;
  cacheViewer.valueState = "loading";
  cacheViewer.message = "";
  paintCacheViewer();
  try {
    const result = await adminPortalService.cacheValue({ key, signal: activeController.signal });
    if (cacheValueController !== activeController) return;
    cacheViewer.valueFound = Boolean(result.found);
    cacheViewer.valueContent = result.content || "";
    cacheViewer.valueState = "done";
  } catch (error) {
    if (error.name === "AbortError" || cacheValueController !== activeController) return;
    cacheViewer.valueState = "error";
    cacheViewer.message = error.message || "Unable to load cache data.";
  } finally {
    if (cacheValueController === activeController) {
      cacheValueController = undefined;
      paintCacheViewer();
    }
  }
}

const routeMetadata = { operator: "", travel: "", from: "", to: "", state: "idle", message: "", items: [] };
const ROUTE_METADATA_FIELDS = [["metadata-operator", "operator"], ["metadata-travel", "travel"], ["metadata-from", "from"], ["metadata-to", "to"]];
let routeMetadataController;

function routeMetadataResult() {
  if (routeMetadata.state === "idle") return `<div class="cm-empty">Enter the complete route key to look up persisted metadata.</div>`;
  if (routeMetadata.state === "loading") return `<div class="cm-empty">Loading route metadata…</div>`;
  if (routeMetadata.state === "error") return `<div class="cm-empty cm-error">${esc(routeMetadata.message)}</div>`;
  if (!routeMetadata.items.length) return `<div class="cm-empty">No route metadata found for this complete route key.</div>`;
  const rows = routeMetadata.items.map((item) => `<tr><td>${esc(item.operatorCode)}</td><td>${esc(item.tripDate)}</td><td>${esc(item.fromStation)}</td><td>${esc(item.toStation)}</td><td>${esc(item.tripCode)}</td><td>${esc(item.tripStageCode)}</td><td>${esc(item.updatedAt ? stamp(item.updatedAt) : "—")}</td></tr>`).join("");
  return `<div class="t-wrap"><table><thead><tr><th>Operator</th><th>Trip date</th><th>From</th><th>To</th><th>Trip code</th><th>Trip stage code</th><th>Updated at</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function routeMetadataPage() {
  return `<section class="panel cm-viewer"><div class="p-head"><div><h2>Route Metadata</h2><p class="cm-subtitle">Read-only Cassandra lookup for one complete route partition.</p></div><span class="tag na">on-demand lookup</span></div><form class="cm-form" id="route-metadata-form" autocomplete="off"><label>Operator<input id="metadata-operator" name="operator" value="${esc(routeMetadata.operator)}" maxlength="128" required></label><label>Trip date<input id="metadata-travel" name="travel" type="date" value="${esc(routeMetadata.travel)}" required></label><label>From<input id="metadata-from" name="from" value="${esc(routeMetadata.from)}" maxlength="128" required></label><label>To<input id="metadata-to" name="to" value="${esc(routeMetadata.to)}" maxlength="128" required></label><button type="submit">Look up metadata</button></form><div id="route-metadata-result">${routeMetadataResult()}</div></section>`;
}

function bindRouteMetadata() {
  document.getElementById("route-metadata-form")?.addEventListener("submit", (event) => { event.preventDefault(); runRouteMetadataLookup(); });
  ROUTE_METADATA_FIELDS.forEach(([id, key]) => document.getElementById(id)?.addEventListener("input", (event) => { routeMetadata[key] = event.target.value; }));
}

function paintRouteMetadataResult() {
  const target = document.getElementById("route-metadata-result");
  if (target) target.innerHTML = routeMetadataResult();
}

async function runRouteMetadataLookup() {
  ROUTE_METADATA_FIELDS.forEach(([id, key]) => { routeMetadata[key] = document.getElementById(id)?.value.trim() || ""; });
  if (Object.values(routeMetadata).slice(0, 4).some((value) => !value || value.includes(":"))) {
    routeMetadata.state = "error";
    routeMetadata.message = "Operator, trip date, from, and to are required and cannot contain a colon.";
    paintRouteMetadataResult();
    return;
  }
  routeMetadataController?.abort();
  const activeController = new AbortController();
  routeMetadataController = activeController;
  routeMetadata.state = "loading";
  routeMetadata.message = "";
  paintRouteMetadataResult();
  try {
    routeMetadata.items = await adminPortalService.routeMetadata({ operator: routeMetadata.operator, travel: routeMetadata.travel, from: routeMetadata.from, to: routeMetadata.to, signal: activeController.signal });
    if (routeMetadataController !== activeController) return;
    routeMetadata.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || routeMetadataController !== activeController) return;
    routeMetadata.state = "error";
    routeMetadata.message = error.message || "Unable to load route metadata.";
  } finally {
    if (routeMetadataController === activeController) {
      routeMetadataController = undefined;
      paintRouteMetadataResult();
    }
  }
}

function page(current) {
  if (current === "dashboard") return dashboard();
  if (current === "queues") return queuesPage();
  if (current === "workers") return `<section class="panel"><div class="p-head"><h2>Workers Status</h2><span class="tag${live("rabbitmq") ? "" : " na"}">RabbitMQ consumers</span></div>${workerStatus()}</section>`;
  if (current === "failures") return `<section class="panel"><div class="p-head"><h2>Failures & DLQ</h2><span class="tag${live("queueMetrics") ? "" : " na"}">24h sample</span></div>${failuresTable()}</section>`;
  if (current === "inventory-events") return recordsPage();
  if (current === "tripdetails") return `<section class="panel"><div class="p-head"><h2>TripDetails Freshness</h2></div>${freshness()}</section>`;
  if (current === "trip-analyzer") return `<section class="panel"><div class="p-head"><h2>Trip Analyzer / Trip History</h2><span class="tag na">bounded scan · queue_metrix</span></div>${tripAnalyzer()}</section>`;
  if (current === "dragonfly") return cacheViewerPage();
  if (current === "cassandra") return routeMetadataPage();
  if (current === "reports") return `<section class="panel"><div class="p-head"><h2>Reports</h2><a href="${ADMIN_ENDPOINTS.reports}">Open Reports</a></div><div class="na-box"><div>Detailed Queue Metrics reporting and filtering remain available in the existing reports area.</div></div></section>`;
  return dashboard();
}

function bind() {
  document.getElementById("trip-form")?.addEventListener("submit", (event) => { event.preventDefault(); runTripLookup(); });
  TRIP_FIELDS.forEach(([id, key]) => document.getElementById(id)?.addEventListener("input", (event) => { trip[key] = event.target.value; }));
  document.getElementById("bell")?.addEventListener("click", () => { location.href = href("failures"); });
  document.getElementById("burger")?.addEventListener("click", () => document.querySelector(".side")?.scrollIntoView({ behavior: "smooth" }));
  if (route() === "dragonfly") bindCacheViewer();
  if (route() === "cassandra") bindRouteMetadata();
}

async function load() {
  controller?.abort();
  controller = new AbortController();
  const current = route();
  mount.innerHTML = shell(current, `<section class="kpis">${Array.from({ length: 8 }, () => '<div class="skeleton"></div>').join("")}</section>`);
  try {
    overview = await adminPortalService.dashboard({ signal: controller.signal });
    mount.innerHTML = shell(current, page(current));
    bind();
    if (current === "dragonfly") loadCacheViewer(true);
  } catch (error) {
    if (error.name === "AbortError") return;
    mount.innerHTML = shell(current, '<div class="err">Unable to load the protected admin snapshot. Check the session and refresh to retry.</div>');
    bind();
  }
}

load();

function spanOf(seconds) {
  if (seconds === null || seconds === undefined) return "—";
  const total = Number(seconds);
  if (total < 60) return `${total} sec`;
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  if (minutes < 60) return rest ? `${minutes} min ${rest} sec` : `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  return `${hours} hr ${minutes % 60} min`;
}

const stamp = (value) => value ? new Date(value).toLocaleString() : "—";

function tripEntryCard(entry) {
  const tone = entry.queueStatus === "DEAD" ? "bad" : entry.queueStatus === "COMPLETED" ? "" : "warn";
  const finished = entry.completedAt ? ["Queue completed at", stamp(entry.completedAt)] : entry.deadLetteredAt ? ["Dead lettered at", stamp(entry.deadLetteredAt)] : ["Not completed", `Last update ${stamp(entry.updatedAt)}`];
  const facts = [entry.actionType, entry.tripCode || "no trip code", entry.tripDate, entry.fromStation && entry.toStation ? `${entry.fromStation} → ${entry.toStation}` : "", entry.scheduleCode].filter(Boolean).map((value) => esc(value)).join(" · ");
  const updatedTripCodes = [...new Set((Array.isArray(entry.updatedTripCodes) ? entry.updatedTripCodes : []).filter(Boolean))];
  const updatedTrips = updatedTripCodes.length ? updatedTripCodes.map((tripCode) => `<b class="trip-updated-trip">${esc(tripCode)}</b>`).join("") : "<small>No completed trip codes captured</small>";
  const message = entry.message ? `<details class="trip-message"><summary>View queued message</summary><pre>${esc(entry.message)}</pre></details>` : `<p class="trip-message-empty">Queued message was not captured for this historical record.</p>`;
  return `<article class="trip-card">
    <div class="trip-row trip-row-top"><div><span>Reference ID</span><strong>${esc(entry.referenceId)}</strong></div><div><span>Activity type</span><strong>${esc(entry.activityType)}</strong></div><div><span>Queued at</span><strong>${esc(stamp(entry.queuedAt))}</strong></div><div class="trip-status">${chip(entry.queueStatus, tone)}</div></div>
    <div class="trip-row trip-row-mid"><div><span>Total time taken</span><strong>${esc(spanOf(entry.durationSeconds))}</strong></div><div class="trip-updated-trips"><span>Updated trips</span><div>${updatedTrips}</div></div>${facts ? `<p class="trip-facts">${facts}</p>` : ""}</div>
    <div class="trip-row trip-row-bot"><div><span>${esc(finished[0])}</span><strong>${esc(finished[1])}</strong></div>${entry.failureMessage ? `<p class="trip-error" title="${esc(entry.failureMessage)}">⚠ ${esc(entry.failureMessage)}</p>` : ""}</div>
    <div class="trip-row trip-row-message">${message}</div>
  </article>`;
}

function tripResultBody() {
  if (trip.state === "loading") return `<div class="na-box"><div>Loading trip history…</div></div>`;
  if (trip.state === "error") return `<div class="na-box"><div><b>Lookup unavailable</b>${esc(trip.message)}</div></div>`;
  if (trip.state === "idle" || !trip.result) return `<div class="na-box"><div>Enter an operator code with a trip code, or a trip date and stations, then run the analyzer.</div></div>`;
  const result = trip.result;
  if (!result.totalRecords) return `<div class="na-box"><div>No queue_metrix records matched operator <b>${esc(result.operatorCode)}</b> with the supplied selectors. Search-family activities only match on trip date and stations.</div></div>`;
  const stats = [["Records", num(result.totalRecords)], ["Completed", num(result.completed)], ["Dead", num(result.dead)], ["Pending", num(result.pending)], ["Average time", spanOf(result.averageDurationSeconds)], ["Longest time", spanOf(result.longestDurationSeconds)]];
  const cost = `<p class="trip-cost">Scanned ${num(result.rowsExamined)} rows in ${num(result.elapsedMilliseconds)} ms${result.truncated ? " · stopped at a scan limit" : ""}</p>`;
  const activities = (result.activities || []).length ? `<div class="trip-activities">${(result.activities || []).map((item, index) => `<span><i style="background:${SERIES[index % SERIES.length]}"></i>${esc(item.activityType)}<b>${num(item.count)}</b></span>`).join("")}</div>` : "";
  return `${result.truncated ? `<p class="trip-warn trip-truncated">Partial result. The scan hit a row, match, or time limit before reaching the end of the table.</p>` : ""}<div class="counts trip-counts">${stats.map(([label, value]) => `<div><span>${esc(label)}</span><strong>${esc(value)}</strong></div>`).join("")}</div>
    ${activities}
    ${cost}
    <p class="note">First queued ${esc(stamp(result.firstQueuedAt))} · last activity ${esc(stamp(result.lastActivityAt))}</p>
    <div class="trip-list">${result.entries.map(tripEntryCard).join("")}</div>
    <p class="note">${esc(result.scope)}</p>`;
}

function tripAnalyzer() {
  return `<p class="trip-warn">Only busmap-family records store a trip code. To see <b>every</b> activity type for a trip, also enter its trip date and stations, which every record carries. Matches from either selector are combined. The lookup scans queue_metrix in 5,000 row pages, capped at 200,000 rows, 500 matches, and 10 seconds, one at a time.</p>
    <form class="trip-form" id="trip-form" autocomplete="off">
      <label>Operator code<input id="trip-operator" name="operatorCode" placeholder="e.g. bits" value="${esc(trip.operatorCode)}" maxlength="128" required></label>
      <label>Trip code<input id="trip-code" name="tripCode" placeholder="e.g. 2N38731S260820D" value="${esc(trip.tripCode)}" maxlength="128"></label>
      <label>Trip date<input id="trip-date" name="tripDate" type="date" value="${esc(trip.tripDate)}"></label>
      <label>From station<input id="trip-from" name="fromStation" placeholder="e.g. STF17D52" value="${esc(trip.fromStation)}" maxlength="128"></label>
      <label>To station<input id="trip-to" name="toStation" placeholder="e.g. STF17D51" value="${esc(trip.toStation)}" maxlength="128"></label>
      <button type="submit">Analyze trip</button>
    </form><div id="trip-result">${tripResultBody()}</div>`;
}

async function runTripLookup() {
  TRIP_FIELDS.forEach(([id, key]) => { trip[key] = document.getElementById(id)?.value.trim() || ""; });
  if (!trip.operatorCode || (!trip.tripCode && !trip.tripDate && !trip.fromStation && !trip.toStation)) {
    trip.state = "error";
    trip.message = "Enter an operator code plus a trip code, or a trip date and stations.";
    paintTripResult();
    return;
  }
  trip.state = "loading";
  trip.message = "";
  paintTripResult();
  try {
    trip.result = await adminPortalService.tripHistory({ operatorCode: trip.operatorCode, tripCode: trip.tripCode, tripDate: trip.tripDate, fromStation: trip.fromStation, toStation: trip.toStation });
    trip.state = "done";
  } catch (error) {
    trip.state = "error";
    trip.message = error.message || "Unable to load trip history.";
  }
  paintTripResult();
}

function paintTripResult() {
  const target = document.getElementById("trip-result");
  if (target) target.innerHTML = tripResultBody();
}
