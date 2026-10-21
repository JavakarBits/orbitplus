import { adminPortalService, ADMIN_ENDPOINTS } from "/orbitplus/admin/portal-service.js?v=14";

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
  { id: "cache-settings", label: "Cache Settings", icon: "⚙" },
  { id: "cassandra", label: "Cassandra (Metadata)", icon: "▤" },
  { id: "operators", label: "Operators", icon: "♙" },
  { id: "cache-freshness", label: "Cache Freshness", icon: "❄" },
  { id: "memory", label: "Memory", icon: "▣" },
  { id: "logs", label: "Logs", icon: "▦" },
  { id: "reports", label: "Reports", icon: "▥" }
];
// Report sub-pages reachable from the Reports section. They render inside the
// SPA (so the sidebar stays) but are not top-level sidebar items themselves.
const reportRoutes = ["top-routes", "queue-metrics", "queue-analytics", "busmap-analytics"];
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
const route = () => { const id = location.pathname.split("/").filter(Boolean).at(-1); return (routes.some((item) => item.id === id) || reportRoutes.includes(id)) ? id : "dashboard"; };
const href = (id) => id === "dashboard" ? "/orbitplus/admin" : `/orbitplus/admin/${id}`;
// hrefFor resolves a nav item's link: an external page URL when the item
// declares one, otherwise the in-SPA section path.
const hrefFor = (item) => item.url ? item.url : href(item.id);
const chip = (value, tone) => `<span class="chip ${tone || ""}">${esc(value)}</span>`;
const na = (name, link, linkText) => { const source = capability(name); return `<div class="na-box"><div><b>${esc(source.status.replaceAll("_", " "))}</b>${esc(source.detail)}${link ? `<br><a href="${link}">${esc(linkText)}</a>` : ""}</div></div>`; };

function shell(current, content) {
  const nav = routes.map((item) => `<a href="${hrefFor(item)}"${current === item.id ? ' aria-current="page"' : ""}><i>${item.icon}</i>${esc(item.label)}</a>`).join("");
  const degraded = ["queueMetrics", "rabbitmq", "tripDetailsFreshness"].filter((name) => capability(name).status === "source_unavailable");
  const status = live("runtime") ? (degraded.length ? "Degraded telemetry" : "Core services responding") : "Runtime unavailable";
  return `<div class="app"><aside class="side"><a class="side-brand" href="/orbitplus/admin"><i>◆</i>OrbitPlus</a><p class="side-label">ADMIN PORTAL</p><nav class="nav">${nav}</nav><div class="side-foot"><p class="side-label" style="padding:0">SYSTEM STATUS</p><b><i></i>${esc(status)}</b><small>OrbitPlus · ${esc(data("runtime").goVersion || "runtime")}</small></div></aside><main class="main">${top(current)}<div class="body">${content}</div></main></div>`;
}

const reportLabels = { "top-routes": "Top Routes", "queue-metrics": "Queue Metrics", "queue-analytics": "Queue Analytics", "busmap-analytics": "Busmap Data Analytics" };

function top(current) {
  const page = routes.find((item) => item.id === current) || (reportLabels[current] ? { label: reportLabels[current] } : null);
  const subtitle = current === "dashboard" ? "Live overview of the TripDetails refresh system" : "Live data where a backend source exists";
  const failures = live("queueMetrics") ? (data("queueMetrics").recentFailures || []).length : 0;
  return `<header class="top"><div class="top-l"><button class="burger" id="burger" type="button" title="Navigation">☰</button><div><h1>${esc(page?.label || "Dashboard")}</h1><p>${esc(subtitle)}</p></div></div><div class="top-r"><span class="pill">◷ Last 24 hours</span><button class="bell" id="bell" type="button" title="Failed records in sample">⌁${failures ? `<span>${failures}</span>` : ""}</button><div class="user-menu"><button class="user" id="user-trigger" type="button" aria-haspopup="true"><i>A</i><span class="user-meta"><b>Admin</b><small>OrbitPlus</small></span><em class="caret">▾</em></button><div class="user-dropdown" id="user-dropdown" role="menu"><nav class="user-dropdown-list"><button type="button" id="logout-item" role="menuitem" class="danger"><i>⎋</i>Logout</button></nav></div></div></div></header>`;
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
  // A consumer that is up or momentarily blocked/busy (prefetch in-flight) is a
  // live worker. Only waiting/idle standby consumers are not actively healthy.
  const isHealthy = (value) => /up|running|active|blocked|busy|single/i.test(value || "");
  const state = (value) => isHealthy(value) ? ["Healthy", ""] : /wait|idle/i.test(value || "") ? ["Idle", "warn"] : [value || "Unknown", "na"];
  const rows = consumers.slice(0, limit || consumers.length).map((item) => {
    const [label, tone] = state(item.status);
    return `<tr><td title="${esc(item.tag)}">${esc(item.tag || item.connection)}</td><td>${chip(label, tone)}</td><td>${esc(item.queue)}</td><td>${num(item.prefetchCount)}</td><td>${esc(item.user)}</td></tr>`;
  }).join("");
  const healthy = consumers.filter((item) => isHealthy(item.status)).length;
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

const memory = { state: "idle", message: "", data: null };
let memoryController;
let memoryTimer;

const memBytes = (value) => { if (value === null || value === undefined) return "—"; const n = Number(value); if (n < 1024) return `${n} B`; const units = ["KB", "MB", "GB", "TB"]; let size = n / 1024, unit = 0; while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1; } return `${size.toFixed(1)} ${units[unit]}`; };
const memDuration = (seconds) => { if (seconds === null || seconds === undefined) return "—"; const total = Number(seconds); if (total < 60) return `${total} sec`; const m = Math.floor(total / 60); if (m < 60) return `${m} min`; const h = Math.floor(m / 60); return `${h} hr ${m % 60} min`; };

function stopMemoryAutoRefresh() {
  if (memoryTimer) { clearInterval(memoryTimer); memoryTimer = undefined; }
}

function memoryCards() {
  const info = memory.data;
  if (!info) return "";
  const heapUsed = Number(info.heapAllocBytes || 0);
  const heapSys = Number(info.heapSystemBytes || 0);
  const heapPercent = heapSys > 0 ? Math.min(100, heapUsed / heapSys * 100) : 0;
  const cards = [
    ["▣", "Heap in use", memBytes(info.heapAllocBytes), "Live heap objects", "t-violet"],
    ["◆", "Heap reserved", memBytes(info.heapSystemBytes), "Heap obtained from OS", "t-blue"],
    ["▦", "Process memory (Sys)", memBytes(info.goRuntimeSystemBytes), "Total Go runtime from OS", "t-cyan"],
    ["⌗", "Heap objects", num(info.heapObjects), "Allocated live objects", "t-green"],
    ["↻", "GC cycles", num(info.gcCycles), "Completed collections", "t-amber"],
    ["⚙", "Goroutines", num(info.goroutines), "Active Go routines", "t-violet"],
    ["⊡", "Next GC target", memBytes(info.nextGCTargetBytes), "Heap size that triggers GC", "t-blue"],
    ["◉", "GOMAXPROCS", num(info.goMaxProcs), `${num(info.logicalCPUs)} logical CPUs`, "t-slate"],
  ];
  const kpiHTML = cards.map(([icon, label, value, note, tone]) => `<article class="kpi ${tone}"><i>${icon}</i><div><span>${esc(label)}</span><strong>${esc(value)}</strong><small>${esc(note)}</small></div></article>`).join("");
  const totalPauseMs = Number(info.totalGCPauseNanoseconds || 0) / 1e6;
  const stats = [
    ["Status", info.status || "UP"],
    ["Go version", info.goVersion],
    ["OS / Arch", `${info.operatingSystem || "—"} / ${info.architecture || "—"}`],
    ["Uptime", memDuration(info.uptimeSeconds)],
    ["Go CPU capacity", info.goCpuCapacityPercent == null ? "measuring…" : `${Number(info.goCpuCapacityPercent).toFixed(1)}%`],
    ["Total GC pause", `${totalPauseMs.toFixed(1)} ms`],
    ["Last GC", info.lastGCAt ? new Date(info.lastGCAt).toLocaleString() : "—"],
  ];
  const statHTML = stats.map(([label, value]) => `<div class="stat"><span>${esc(label)}</span><strong>${esc(value ?? "—")}</strong></div>`).join("");
  return `<section class="kpis">${kpiHTML}</section>
  <section class="grid g-2">
    <article class="panel"><div class="p-head"><h2>Heap utilization</h2><span class="tag">${heapPercent.toFixed(0)}% of reserved</span></div><div class="mem-bar"><i style="width:${heapPercent}%"></i></div><p class="note">${esc(memBytes(info.heapAllocBytes))} in use of ${esc(memBytes(info.heapSystemBytes))} reserved from the OS. In-use heap is live data; reserved is what the Go runtime holds and may reuse before returning to the OS.</p></article>
    <article class="panel"><div class="p-head"><h2>Runtime</h2><span class="tag${info.status ? "" : " na"}">${esc(info.status || "unavailable")}</span></div>${statHTML}</article>
  </section>`;
}

function memoryBody() {
  if (memory.state === "loading" && !memory.data) return `<div class="na-box"><div>Loading memory metrics…</div></div>`;
  if (memory.state === "error") return `<div class="na-box"><div><b>Memory metrics unavailable</b>${esc(memory.message || "Unable to load runtime memory metrics.")}</div></div>`;
  if (!memory.data) return `<div class="na-box"><div>No memory metrics are available yet.</div></div>`;
  return memoryCards();
}

function memoryPage() {
  return `<section class="panel mem-panel" id="memory-viewer"><div class="p-head"><div><h2>Go runtime memory</h2><p class="df-subtitle">Live heap, garbage-collection, and goroutine metrics for the Master process. Refreshes automatically every 5 seconds.</p></div><button class="df-refresh" id="memory-refresh" type="button">Refresh</button></div><div id="memory-body">${memoryBody()}</div></section>`;
}

function bindMemory() {
  document.getElementById("memory-refresh")?.addEventListener("click", () => loadMemory());
  stopMemoryAutoRefresh();
  memoryTimer = setInterval(() => { if (route() === "memory") loadMemory(); else stopMemoryAutoRefresh(); }, 5000);
}

function paintMemoryBody() {
  const target = document.getElementById("memory-body");
  if (target) target.innerHTML = memoryBody();
}

async function loadMemory() {
  memoryController?.abort();
  const activeController = new AbortController();
  memoryController = activeController;
  if (!memory.data) { memory.state = "loading"; paintMemoryBody(); }
  try {
    const result = await adminPortalService.systemHealth({ signal: activeController.signal });
    if (memoryController !== activeController) return;
    memory.data = result;
    memory.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || memoryController !== activeController) return;
    memory.state = "error";
    memory.message = error.message || "Unable to load runtime memory metrics.";
  } finally {
    if (memoryController === activeController) {
      memoryController = undefined;
      paintMemoryBody();
    }
  }
}

const logs = { files: [], state: "idle", message: "", selected: "", selectedSource: "", content: "", truncated: false, contentState: "idle", contentMessage: "", contentSize: 0 };
let logsController;
let logsContentController;

const logSize = (value) => { const n = Number(value || 0); if (n < 1024) return `${n} B`; const units = ["KB", "MB", "GB"]; let size = n / 1024, unit = 0; while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1; } return `${size.toFixed(1)} ${units[unit]}`; };

function logsPage() {
  const status = logs.state === "error" ? (logs.message || "Unable to load log files.") : logs.state === "loading" ? "Loading log files…" : `${logs.files.length} log file${logs.files.length === 1 ? "" : "s"} across the Master and Worker log directories. Newest first.`;
  const rows = logs.files.map((file) => `<tr${file.name === logs.selected && (file.source || "") === logs.selectedSource ? ' class="log-row-active"' : ""}><td>${chip(file.source || "—", (file.source === "worker") ? "" : "na")}</td><td class="log-name">${esc(file.name)}</td><td>${esc(logSize(file.sizeBytes))}</td><td>${esc(file.modifiedUtc)} UTC</td><td><button class="log-show" type="button" data-log-name="${esc(file.name)}" data-log-source="${esc(file.source || "")}">View</button></td></tr>`).join("");
  const empty = logs.state === "loading" && !logs.files.length ? "Loading log files…" : logs.state === "error" ? (logs.message || "Unable to load log files. Check the log directory and try again.") : "No log files found. A service writes logs once it starts with file logging enabled.";
  const viewerTitle = logs.contentState === "loading" ? "Loading log file…" : logs.contentState === "error" ? "Unable to load log file" : `${logs.selectedSource ? esc(logs.selectedSource) + " · " : ""}${esc(logs.selected)}`;
  const viewer = logs.contentState === "idle" ? "" : `<section class="log-viewer"><div class="log-viewer-head"><strong>${viewerTitle}</strong>${logs.contentState === "done" ? `<span>${esc(logSize(logs.contentSize))}${logs.truncated ? " · showing last 512 KB" : ""}</span>` : ""}</div>${logs.contentState === "done" ? `<pre class="log-pre">${esc(logs.content) || "<em>(empty file)</em>"}</pre>` : ""}${logs.contentState === "error" ? `<p class="cm-error">${esc(logs.contentMessage)}</p>` : ""}</section>`;
  return `<section class="panel log-panel" id="logs-viewer"><div class="p-head"><div><h2>Application logs</h2><p class="df-subtitle">Browse log files written by the Master and Worker services. Select a file to view its most recent lines.</p></div><button class="df-refresh" id="logs-refresh" type="button">Refresh</button></div><p class="df-status">${status}</p>${rows ? `<div class="t-wrap"><table><thead><tr><th>Source</th><th>File</th><th>Size</th><th>Modified</th><th></th></tr></thead><tbody>${rows}</tbody></table></div>` : `<div class="df-empty">${esc(empty)}</div>`}${viewer}</section>`;
}

function bindLogs() {
  document.getElementById("logs-refresh")?.addEventListener("click", () => loadLogs(true));
  document.querySelectorAll("[data-log-name]").forEach((button) => button.addEventListener("click", () => showLogContent(button.dataset.logName, button.dataset.logSource)));
}

function paintLogs() {
  const target = document.getElementById("logs-viewer");
  if (target) {
    target.outerHTML = logsPage();
    bindLogs();
  }
}

async function loadLogs(reset = false) {
  logsController?.abort();
  const activeController = new AbortController();
  logsController = activeController;
  if (reset) {
    logs.selected = "";
    logs.selectedSource = "";
    logs.content = "";
    logs.contentState = "idle";
  }
  logs.state = "loading";
  logs.message = "";
  paintLogs();
  try {
    const result = await adminPortalService.logs({ signal: activeController.signal });
    if (logsController !== activeController) return;
    logs.files = Array.isArray(result.files) ? result.files : [];
    logs.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || logsController !== activeController) return;
    logs.state = "error";
    logs.message = error.message || "Unable to load log files.";
  } finally {
    if (logsController === activeController) {
      logsController = undefined;
      paintLogs();
    }
  }
}

async function showLogContent(name, source) {
  logsContentController?.abort();
  const activeController = new AbortController();
  logsContentController = activeController;
  logs.selected = name;
  logs.selectedSource = source || "";
  logs.contentState = "loading";
  logs.contentMessage = "";
  paintLogs();
  try {
    const result = await adminPortalService.logContent({ name, source, signal: activeController.signal });
    if (logsContentController !== activeController) return;
    logs.content = result.content || "";
    logs.truncated = Boolean(result.truncated);
    logs.contentSize = Number(result.sizeBytes || 0);
    logs.contentState = "done";
  } catch (error) {
    if (error.name === "AbortError" || logsContentController !== activeController) return;
    logs.contentState = "error";
    logs.contentMessage = error.message || "Unable to load log file.";
  } finally {
    if (logsContentController === activeController) {
      logsContentController = undefined;
      paintLogs();
    }
  }
}

const operators = { code: "", zoneCode: "", items: [], state: "idle", message: "", mutation: "" };
let operatorsController;

function operatorsResult() {
  if (operators.state === "loading" && !operators.items.length) return `<div class="op-empty">Loading operators…</div>`;
  if (operators.state === "error") return `<div class="op-empty cm-error">${esc(operators.message || "Unable to load operators.")}</div>`;
  if (!operators.items.length) return `<div class="op-empty">No operators are registered yet.</div>`;
  const rows = operators.items.map((operator) => {
    const action = operator.active ? "Disable" : "Enable";
    const disabled = operators.mutation ? " disabled" : "";
    return `<tr><td>${esc(operator.operatorCode)}</td><td>${esc(operator.zoneCode || "—")}</td><td>${chip(operator.active ? "Active" : "Inactive", operator.active ? "" : "na")}</td><td><button class="op-status" type="button" data-operator-code="${esc(operator.operatorCode)}" data-operator-active="${operator.active ? "true" : "false"}"${disabled}>${action}</button></td></tr>`;
  }).join("");
  return `<div class="t-wrap"><table><thead><tr><th>Operator code</th><th>Zone code</th><th>Status</th><th></th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function operatorsPage() {
  const saving = Boolean(operators.mutation);
  return `<section class="panel op-viewer" id="operators-viewer"><div class="p-head"><div><h2>Operators</h2><p class="op-subtitle">Manage the operator registry used by inventory events and periodic route refresh.</p></div><span class="tag na">registry</span></div><form class="op-form" id="operator-create-form" autocomplete="off"><label>Operator code<input id="operator-code" name="operatorCode" value="${esc(operators.code)}" maxlength="128" required${saving ? " disabled" : ""}></label><label>Zone code<input id="operator-zone-code" name="zoneCode" value="${esc(operators.zoneCode)}" maxlength="128" placeholder="e.g. r3bits" required${saving ? " disabled" : ""}></label><button type="submit"${saving ? " disabled" : ""}>Save operator</button></form><p class="note">Zone codes must be supported Orionmax zones. Saving an existing operator updates its zone while preserving its active status; disabling does not delete the record.</p><div id="operators-result">${operatorsResult()}</div></section>`;
}

function bindOperators() {
  document.getElementById("operator-create-form")?.addEventListener("submit", (event) => { event.preventDefault(); createOperator(); });
  document.getElementById("operator-code")?.addEventListener("input", (event) => { operators.code = event.target.value; });
  document.getElementById("operator-zone-code")?.addEventListener("input", (event) => { operators.zoneCode = event.target.value; });
  document.querySelectorAll("[data-operator-code]").forEach((button) => button.addEventListener("click", () => setOperatorActive(button.dataset.operatorCode, button.dataset.operatorActive !== "true")));
}

function paintOperators() {
  const target = document.getElementById("operators-viewer");
  if (target) {
    target.outerHTML = operatorsPage();
    bindOperators();
  }
}

async function loadOperators() {
  operatorsController?.abort();
  const activeController = new AbortController();
  operatorsController = activeController;
  operators.state = "loading";
  operators.message = "";
  paintOperators();
  try {
    operators.items = await adminPortalService.operators({ signal: activeController.signal });
    if (operatorsController !== activeController) return;
    operators.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || operatorsController !== activeController) return;
    operators.state = "error";
    operators.message = error.message || "Unable to load operators.";
  } finally {
    if (operatorsController === activeController) {
      operatorsController = undefined;
      paintOperators();
    }
  }
}

async function createOperator() {
  operators.code = document.getElementById("operator-code")?.value.trim() || "";
  operators.zoneCode = document.getElementById("operator-zone-code")?.value.trim() || "";
  if (!operators.code || !operators.zoneCode) {
    operators.state = "error";
    operators.message = "Operator code and zone code are required.";
    paintOperators();
    return;
  }
  operators.mutation = "create";
  paintOperators();
  try {
    await adminPortalService.createOperator(operators.code, operators.zoneCode);
    operators.code = "";
    operators.zoneCode = "";
    await loadOperators();
  } catch (error) {
    operators.state = "error";
    operators.message = error.message || "Unable to create operator.";
  } finally {
    operators.mutation = "";
    paintOperators();
  }
}

async function setOperatorActive(operatorCode, active) {
  operators.mutation = operatorCode;
  paintOperators();
  try {
    await adminPortalService.setOperatorActive(operatorCode, active);
    await loadOperators();
  } catch (error) {
    operators.state = "error";
    operators.message = error.message || "Unable to update operator.";
  } finally {
    operators.mutation = "";
    paintOperators();
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

const cacheSettings = { form: { hotMaxAgeHours: 4, warmMaxAgeHours: 6, windowEnabled: false, windowStart: "06:00", windowEnd: "22:00" }, current: null, state: "idle", message: "", saveState: "idle", saveMessage: "" };
let cacheSettingsController;
const CACHE_SETTINGS_FIELDS = [["cs-hot", "hotMaxAgeHours"], ["cs-warm", "warmMaxAgeHours"], ["cs-window-enabled", "windowEnabled"], ["cs-window-start", "windowStart"], ["cs-window-end", "windowEnd"]];

function cacheSettingsSummary() {
  const current = cacheSettings.current;
  if (!current) return "";
  const windowText = current.windowEnabled ? `cache ${esc(current.windowStart)}–${esc(current.windowEnd)} IST, live outside` : "cache on 24/7";
  const saved = current.updatedAt ? ` · last saved ${new Date(current.updatedAt).toLocaleString()}` : "";
  return `<div class="cs-summary"><span class="chip">Hot ≤ ${esc(current.hotMaxAgeHours)}h</span><span class="chip">Warm ≤ ${esc(current.warmMaxAgeHours)}h</span>${current.windowEnabled ? chip(windowText, "") : chip(windowText, "na")}<small>${esc(current.timezone || "Asia/Kolkata (IST)")}${saved}</small></div>`;
}

function cacheSettingsBody() {
  if (cacheSettings.state === "loading" && !cacheSettings.current) return `<div class="na-box"><div>Loading cache settings…</div></div>`;
  if (cacheSettings.state === "error" && !cacheSettings.current) return `<div class="na-box"><div><b>Cache settings unavailable</b>${esc(cacheSettings.message || "Unable to load cache settings. Create the cache_settings table and restart the Master.")}</div></div>`;
  const form = cacheSettings.form;
  const saving = cacheSettings.saveState === "saving";
  const timeDisabled = form.windowEnabled ? "" : " disabled";
  const saveNote = cacheSettings.saveState === "error" ? `<span class="cs-msg bad">${esc(cacheSettings.saveMessage)}</span>` : cacheSettings.saveState === "done" ? `<span class="cs-msg ok">Saved ✓ Applied immediately.</span>` : "";
  return `${cacheSettingsSummary()}
  <form class="cs-form" id="cache-settings-form" autocomplete="off">
    <fieldset class="cs-group">
      <legend>Freshness thresholds</legend>
      <p class="note">Age = now − last update. Within Hot → serve Hot; within Warm → serve Warm; older → Cold (refresh from BITS). Hot must be ≤ Warm.</p>
      <div class="cs-fields">
        <label>Hot max age (hours)<input id="cs-hot" type="number" min="0.1" step="0.1" value="${esc(form.hotMaxAgeHours)}"${saving ? " disabled" : ""}></label>
        <label>Warm max age (hours)<input id="cs-warm" type="number" min="0.1" step="0.1" value="${esc(form.warmMaxAgeHours)}"${saving ? " disabled" : ""}></label>
      </div>
    </fieldset>
    <fieldset class="cs-group">
      <legend>Cache window (IST)</legend>
      <p class="note">Off = cache on 24/7. On = cache only during the hours below; outside them every read goes live to BITS. A start later than the end wraps past midnight (e.g. 22:00–06:00).</p>
      <div class="cs-fields">
        <label>Cache window<select id="cs-window-enabled"${saving ? " disabled" : ""}><option value="false"${form.windowEnabled ? "" : " selected"}>Off — cache always on</option><option value="true"${form.windowEnabled ? " selected" : ""}>On — cache only in these hours</option></select></label>
        <label>Cache start (IST)<input id="cs-window-start" type="time" value="${esc(form.windowStart)}"${timeDisabled}${saving ? " disabled" : ""}></label>
        <label>Cache end (IST)<input id="cs-window-end" type="time" value="${esc(form.windowEnd)}"${timeDisabled}${saving ? " disabled" : ""}></label>
      </div>
    </fieldset>
    <div class="cs-actions"><button type="submit"${saving ? " disabled" : ""}>${saving ? "Saving…" : "Save settings"}</button>${saveNote}</div>
  </form>`;
}

function cacheSettingsPage() {
  return `<section class="panel cs-panel" id="cache-settings-viewer"><div class="p-head"><div><h2>Cache settings</h2><p class="df-subtitle">Edit the Hot/Warm freshness thresholds and the daily IST cache window at runtime. Saved in the Master and applied immediately.</p></div><button class="df-refresh" id="cache-settings-refresh" type="button">Reload</button></div><div id="cache-settings-body">${cacheSettingsBody()}</div></section>`;
}

function bindCacheSettings() {
  document.getElementById("cache-settings-refresh")?.addEventListener("click", () => loadCacheSettings());
  document.getElementById("cache-settings-form")?.addEventListener("submit", (event) => { event.preventDefault(); saveCacheSettingsForm(); });
  CACHE_SETTINGS_FIELDS.forEach(([id, key]) => {
    const element = document.getElementById(id);
    if (!element) return;
    const handler = (event) => {
      cacheSettings.form[key] = key === "windowEnabled" ? event.target.value === "true" : event.target.value;
      if (key === "windowEnabled") paintCacheSettings();
    };
    element.addEventListener(key === "windowEnabled" ? "change" : "input", handler);
  });
}

function paintCacheSettings() {
  const target = document.getElementById("cache-settings-body");
  if (target) { target.innerHTML = cacheSettingsBody(); bindCacheSettings(); }
}

function applyCacheSettings(payload) {
  cacheSettings.current = payload;
  cacheSettings.form = {
    hotMaxAgeHours: payload.hotMaxAgeHours,
    warmMaxAgeHours: payload.warmMaxAgeHours,
    windowEnabled: Boolean(payload.windowEnabled),
    windowStart: payload.windowStart || "06:00",
    windowEnd: payload.windowEnd || "22:00"
  };
}

async function loadCacheSettings() {
  cacheSettingsController?.abort();
  const activeController = new AbortController();
  cacheSettingsController = activeController;
  cacheSettings.state = "loading";
  cacheSettings.message = "";
  cacheSettings.saveState = "idle";
  paintCacheSettings();
  try {
    const result = await adminPortalService.cacheSettings({ signal: activeController.signal });
    if (cacheSettingsController !== activeController) return;
    applyCacheSettings(result);
    cacheSettings.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || cacheSettingsController !== activeController) return;
    cacheSettings.state = "error";
    cacheSettings.message = error.message || "Unable to load cache settings.";
  } finally {
    if (cacheSettingsController === activeController) {
      cacheSettingsController = undefined;
      paintCacheSettings();
    }
  }
}

async function saveCacheSettingsForm() {
  const form = cacheSettings.form;
  const hot = parseFloat(form.hotMaxAgeHours);
  const warm = parseFloat(form.warmMaxAgeHours);
  if (!(hot > 0) || !(warm > 0) || hot > warm) {
    cacheSettings.saveState = "error";
    cacheSettings.saveMessage = "Hot and Warm must be positive and Hot ≤ Warm.";
    paintCacheSettings();
    return;
  }
  cacheSettings.saveState = "saving";
  cacheSettings.saveMessage = "";
  paintCacheSettings();
  try {
    const result = await adminPortalService.saveCacheSettings({
      hotMaxAgeHours: hot,
      warmMaxAgeHours: warm,
      windowEnabled: Boolean(form.windowEnabled),
      windowStart: form.windowStart || "00:00",
      windowEnd: form.windowEnd || "00:00"
    });
    applyCacheSettings(result);
    cacheSettings.saveState = "done";
  } catch (error) {
    cacheSettings.saveState = "error";
    cacheSettings.saveMessage = error.message || "Unable to save cache settings.";
  } finally {
    paintCacheSettings();
  }
}

const freshnessCounts = { state: "idle", message: "", data: null, tier: "", entriesState: "idle", entriesMessage: "", entries: [], entriesCount: 0 };
let freshnessController;
let freshnessEntriesController;

const ageText = (seconds) => {
  if (seconds == null) return "—";
  if (seconds < 60) return seconds + "s";
  if (seconds < 3600) return Math.floor(seconds / 60) + "m";
  if (seconds < 86400) return Math.floor(seconds / 3600) + "h " + Math.floor((seconds % 3600) / 60) + "m";
  return Math.floor(seconds / 86400) + "d " + Math.floor((seconds % 86400) / 3600) + "h";
};

function cacheFreshnessCards() {
  const d = freshnessCounts.data;
  if (freshnessCounts.state === "loading" && !d) return `<div class="na-box"><div>Loading cache freshness counts…</div></div>`;
  if (freshnessCounts.state === "error") return `<div class="na-box"><div><b>Cache freshness unavailable</b>${esc(freshnessCounts.message || "Check Cassandra connectivity.")}</div></div>`;
  if (!d) return `<div class="na-box"><div>No cache freshness data yet.</div></div>`;
  const cards = [
    ["❄", "Hot", num(d.hot), "Age ≤ " + d.hotMaxAge, "t-green", "hot"],
    ["◐", "Warm", num(d.warm), d.hotMaxAge + " < age ≤ " + d.warmMaxAge, "t-amber", "warm"],
    ["✖", "Cold", num(d.cold), "Age > " + d.warmMaxAge + " — refreshed from BITS", "t-red", "cold"],
    ["Σ", "Total", num(d.total), "Unique trip stages", "t-violet", ""],
  ];
  return `<section class="kpis">${cards.map(([icon, label, value, note, tone, tier]) =>
    `<article class="kpi ${tone}"${tier ? ` data-fresh-tier="${tier}" role="button" tabindex="0" style="cursor:pointer"` : ""}><i>${icon}</i><div><span>${esc(label)}</span><strong>${esc(value)}</strong><small>${esc(note)}</small></div></article>`).join("")}</section>`;
}

function cacheFreshnessEntriesBlock() {
  if (!freshnessCounts.tier) return "";
  const title = `Trip stages (${esc(freshnessCounts.tier)})`;
  if (freshnessCounts.entriesState === "loading") return `<section class="panel"><div class="p-head"><h2>${title}</h2><span class="tag">Loading…</span></div><div class="na-box"><div>Loading trip stages…</div></div></section>`;
  if (freshnessCounts.entriesState === "error") return `<section class="panel"><div class="p-head"><h2>${title}</h2></div><div class="na-box"><div>${esc(freshnessCounts.entriesMessage || "Unavailable")}</div></div></section>`;
  const rows = freshnessCounts.entries.map((e) => `<tr><td>${esc(e.operatorCode)}</td><td>${esc(e.tripCode)}</td><td>${esc(e.tripStageCode)}</td><td>${esc(e.tripDate)}</td><td>${esc(e.fromStation)}</td><td>${esc(e.toStation)}</td><td>${esc(e.updatedAt ? new Date(e.updatedAt).toLocaleString() : "—")}</td><td>${esc(ageText(e.ageSeconds))}</td></tr>`).join("");
  return `<section class="panel"><div class="p-head"><h2>${title}</h2><span class="tag">${num(freshnessCounts.entriesCount)} ${freshnessCounts.entriesCount === 1 ? "entry" : "entries"}</span></div><div class="t-wrap"><table><thead><tr><th>Operator</th><th>Trip code</th><th>Stage</th><th>Trip date</th><th>From</th><th>To</th><th>Updated</th><th>Age</th></tr></thead><tbody>${rows || '<tr><td colspan="8">No trip stages in this tier.</td></tr>'}</tbody></table></div></section>`;
}

function cacheFreshnessPage() {
  const observed = freshnessCounts.data?.observedAt ? new Date(freshnessCounts.data.observedAt).toLocaleString() : "now";
  return `<section class="panel cf-panel" id="cache-freshness-viewer"><div class="p-head"><div><h2>Cache freshness</h2><p class="df-subtitle">Unique cached trip stages by age as of ${esc(observed)}. Click Hot, Warm, or Cold to list them.</p></div><button class="df-refresh" id="cache-freshness-refresh" type="button">Refresh</button></div>${cacheFreshnessCards()}</section>${cacheFreshnessEntriesBlock()}`;
}

function bindCacheFreshness() {
  document.getElementById("cache-freshness-refresh")?.addEventListener("click", () => loadCacheFreshness());
  document.querySelectorAll("[data-fresh-tier]").forEach((card) => {
    const tier = card.dataset.freshTier;
    card.addEventListener("click", () => loadFreshnessEntries(tier));
    card.addEventListener("keydown", (ev) => { if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); loadFreshnessEntries(tier); } });
  });
}

function paintCacheFreshness() {
  const target = document.getElementById("cache-freshness-viewer");
  if (target) { target.outerHTML = cacheFreshnessPage(); bindCacheFreshness(); }
}

async function loadCacheFreshness() {
  freshnessController?.abort();
  const activeController = new AbortController();
  freshnessController = activeController;
  freshnessCounts.state = "loading";
  freshnessCounts.message = "";
  paintCacheFreshness();
  try {
    freshnessCounts.data = await adminPortalService.cacheFreshness({ signal: activeController.signal });
    if (freshnessController !== activeController) return;
    freshnessCounts.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || freshnessController !== activeController) return;
    freshnessCounts.state = "error";
    freshnessCounts.message = error.message || "Unable to load cache freshness.";
  } finally {
    if (freshnessController === activeController) { freshnessController = undefined; paintCacheFreshness(); }
  }
}

async function loadFreshnessEntries(tier) {
  freshnessEntriesController?.abort();
  const activeController = new AbortController();
  freshnessEntriesController = activeController;
  freshnessCounts.tier = tier;
  freshnessCounts.entriesState = "loading";
  freshnessCounts.entriesMessage = "";
  paintCacheFreshness();
  try {
    const result = await adminPortalService.cacheFreshnessEntries({ tier, signal: activeController.signal });
    if (freshnessEntriesController !== activeController) return;
    freshnessCounts.entries = result.entries || [];
    freshnessCounts.entriesCount = result.count || 0;
    freshnessCounts.entriesState = "done";
  } catch (error) {
    if (error.name === "AbortError" || freshnessEntriesController !== activeController) return;
    freshnessCounts.entriesState = "error";
    freshnessCounts.entriesMessage = error.message || "Unable to load trip stages.";
  } finally {
    if (freshnessEntriesController === activeController) { freshnessEntriesController = undefined; paintCacheFreshness(); }
  }
}

const topRoutes = { from: "", to: "", limit: 100, operator: "", zone: "", state: "idle", message: "", routes: [], scope: "" };
let topRoutesController;
const TOP_ROUTES_FIELDS = [["tr-from", "from"], ["tr-to", "to"], ["tr-limit", "limit"], ["tr-operator", "operator"], ["tr-zone", "zone"]];

function topRoutesInitDates() {
  if (topRoutes.from && topRoutes.to) return;
  const iso = (d) => d.toISOString().slice(0, 10);
  const today = new Date(), plus7 = new Date(); plus7.setDate(today.getDate() + 7);
  topRoutes.from = iso(today); topRoutes.to = iso(plus7);
}

function topRoutesVisible() {
  const op = topRoutes.operator.trim().toLowerCase(), zone = topRoutes.zone.trim().toLowerCase();
  return topRoutes.routes.filter((r) =>
    (!op || (r.operators || []).some((o) => o.toLowerCase().includes(op))) &&
    (!zone || (r.zones || []).some((z) => z.toLowerCase().includes(zone))));
}

function topRoutesTable() {
  if (topRoutes.state === "loading") return `<div class="na-box"><div>Running Top Routes report…</div></div>`;
  if (topRoutes.state === "error") return `<div class="na-box"><div><b>Unable to load Top Routes</b>${esc(topRoutes.message)}</div></div>`;
  const visible = topRoutesVisible();
  if (!visible.length) return `<div class="na-box"><div>No loaded routes match the selected filters.</div></div>`;
  const rows = visible.map((r, index) => `<tr><td>${index + 1}</td><td>${esc(r.fromStation)}</td><td>${esc(r.toStation)}</td><td>${esc(r.tripDate)}</td><td>${num(r.count)}</td><td>${esc((r.operators || []).join(", ") + (r.operatorCount ? " (" + r.operatorCount + ")" : ""))}</td><td>${esc((r.zones || []).join(", "))}</td><td>${esc(r.lastUpdatedAt ? new Date(r.lastUpdatedAt).toLocaleString() : "—")}</td></tr>`).join("");
  return `<div class="t-wrap"><table><thead><tr><th>#</th><th>From</th><th>To</th><th>Trip date</th><th>Count</th><th>Operators</th><th>Zones</th><th>Last updated</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function topRoutesPage() {
  topRoutesInitDates();
  return `<section class="panel cm-viewer"><div class="p-head"><div><h2>Top Routes</h2><p class="cm-subtitle">Most-requested from/to pairs ranked by count within a trip-date range.</p></div><span class="tag na">bounded scan · queue_metrix</span></div>
    <form class="cm-form" id="top-routes-form" autocomplete="off">
      <label>From trip date<input id="tr-from" type="date" value="${esc(topRoutes.from)}"></label>
      <label>To trip date<input id="tr-to" type="date" value="${esc(topRoutes.to)}"></label>
      <label>Top N<input id="tr-limit" type="number" min="1" value="${esc(topRoutes.limit)}"></label>
      <label>Operator<input id="tr-operator" type="search" placeholder="Filter operator" value="${esc(topRoutes.operator)}"></label>
      <label>Zone<input id="tr-zone" type="search" placeholder="Filter zone" value="${esc(topRoutes.zone)}"></label>
      <button type="submit">Run report</button>
    </form>
    ${topRoutes.scope ? `<p class="note">${esc(topRoutes.scope)}</p>` : ""}
    <div id="top-routes-result">${topRoutesTable()}</div></section>`;
}

function bindTopRoutes() {
  document.getElementById("top-routes-form")?.addEventListener("submit", (event) => { event.preventDefault(); loadTopRoutes(); });
  TOP_ROUTES_FIELDS.forEach(([id, key]) => document.getElementById(id)?.addEventListener("input", (event) => {
    topRoutes[key] = event.target.value;
    if (key === "operator" || key === "zone") paintTopRoutesResult();
  }));
}

function paintTopRoutesResult() {
  const target = document.getElementById("top-routes-result");
  if (target) target.innerHTML = topRoutesTable();
}

function paintTopRoutes() {
  const target = document.querySelector(".cm-viewer");
  if (target && route() === "top-routes") { target.outerHTML = topRoutesPage(); bindTopRoutes(); }
}

async function loadTopRoutes() {
  TOP_ROUTES_FIELDS.forEach(([id, key]) => { const el = document.getElementById(id); if (el) topRoutes[key] = el.value; });
  if (!topRoutes.from || !topRoutes.to) { topRoutes.state = "error"; topRoutes.message = "Pick both a from and to trip date."; paintTopRoutesResult(); return; }
  topRoutesController?.abort();
  const activeController = new AbortController();
  topRoutesController = activeController;
  topRoutes.state = "loading";
  topRoutes.message = "";
  paintTopRoutesResult();
  try {
    const result = await adminPortalService.topRoutes({ from: topRoutes.from, to: topRoutes.to, limit: topRoutes.limit || 100, signal: activeController.signal });
    if (topRoutesController !== activeController) return;
    topRoutes.routes = result.routes || [];
    let note = "Filters apply to the loaded routes for trip dates " + result.fromDate + " to " + result.toDate + " (maximum " + topRoutes.routes.length + "). Examined " + result.rowsExamined + " records";
    if (result.truncated) note += " (scan capped — results may be partial)";
    topRoutes.scope = note + ".";
    topRoutes.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || topRoutesController !== activeController) return;
    topRoutes.state = "error";
    topRoutes.message = error.message || "Top Routes report is unavailable.";
  } finally {
    if (topRoutesController === activeController) { topRoutesController = undefined; paintTopRoutes(); }
  }
}

const queueMetrics = { state: "idle", message: "", jobs: [], status: "", activity: "", action: "", operator: "", reference: "" };
let queueMetricsController;
const QM_FIELDS = [["qm-status", "status"], ["qm-activity", "activity"], ["qm-action", "action"], ["qm-operator", "operator"], ["qm-reference", "reference"]];

function qmOptions(values, selected) {
  return [...new Set(values.filter(Boolean))].sort().map((v) => `<option value="${esc(v)}"${selected === v ? " selected" : ""}>${esc(v)}</option>`).join("");
}

function queueMetricsVisible() {
  const ref = queueMetrics.reference.trim().toLowerCase();
  return queueMetrics.jobs.filter((j) =>
    (!queueMetrics.status || j.queueStatus === queueMetrics.status) &&
    (!queueMetrics.activity || j.activityType === queueMetrics.activity) &&
    (!queueMetrics.action || j.actionType === queueMetrics.action) &&
    (!queueMetrics.operator || j.operatorCode === queueMetrics.operator) &&
    (!ref || (j.referenceId || "").toLowerCase().includes(ref)));
}

function queueMetricsTable() {
  if (queueMetrics.state === "loading") return `<div class="na-box"><div>Loading Queue Metrics records…</div></div>`;
  if (queueMetrics.state === "error") return `<div class="na-box"><div><b>Unable to load Queue Metrics</b>${esc(queueMetrics.message)}</div></div>`;
  const visible = queueMetricsVisible();
  if (!visible.length) return `<div class="na-box"><div>No loaded records match the selected filters.</div></div>`;
  const stamp = (v) => v ? new Date(v).toLocaleString() : "—";
  const rows = visible.map((j) => `<tr><td>${esc(j.referenceId)}</td><td>${esc(j.activityType)}</td><td>${esc(j.actionType)}</td><td>${esc(j.operatorCode)}</td><td>${esc(j.fromStation)}</td><td>${esc(j.toStation)}</td><td>${esc(j.tripDate)}</td><td>${chip(j.queueStatus, j.queueStatus === "DEAD" ? "bad" : j.queueStatus === "COMPLETED" ? "" : "warn")}</td><td>${esc(stamp(j.queuedAt))}</td><td>${esc(stamp(j.completedAt))}</td><td title="${esc(j.failureMessage)}">${esc(j.failureMessage || "—")}</td></tr>`).join("");
  return `<div class="t-wrap"><table><thead><tr><th>Ref ID</th><th>Activity</th><th>Action</th><th>Operator</th><th>From</th><th>To</th><th>Trip date</th><th>Status</th><th>Queued</th><th>Completed</th><th>Failure</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function queueMetricsPage() {
  const jobs = queueMetrics.jobs;
  const note = queueMetrics.state === "done" ? `Filters apply to the loaded Queue Metrics records (maximum ${num(jobs.length)}).` : "";
  return `<section class="panel cm-viewer"><div class="p-head"><div><h2>Queue Metrics</h2><p class="cm-subtitle">Filter recent Queue Metrics lifecycle records.</p></div><button class="df-refresh" id="qm-refresh" type="button">Refresh</button></div>
    <form class="cm-form" id="queue-metrics-form" autocomplete="off">
      <label>Status<select id="qm-status"><option value="">All statuses</option>${["RECEIVED","QUEUED","COMPLETED","DEAD"].map((s) => `<option value="${s}"${queueMetrics.status === s ? " selected" : ""}>${s}</option>`).join("")}</select></label>
      <label>Activity<select id="qm-activity"><option value="">All activities</option>${qmOptions(jobs.map((j) => j.activityType), queueMetrics.activity)}</select></label>
      <label>Action<select id="qm-action"><option value="">All actions</option>${qmOptions(jobs.map((j) => j.actionType), queueMetrics.action)}</select></label>
      <label>Operator<select id="qm-operator"><option value="">All operators</option>${qmOptions(jobs.map((j) => j.operatorCode), queueMetrics.operator)}</select></label>
      <label>Reference ID<input id="qm-reference" type="search" placeholder="Search reference ID" value="${esc(queueMetrics.reference)}"></label>
    </form>
    ${note ? `<p class="note">${esc(note)}</p>` : ""}
    <div id="queue-metrics-result">${queueMetricsTable()}</div></section>`;
}

function bindQueueMetrics() {
  document.getElementById("qm-refresh")?.addEventListener("click", () => loadQueueMetrics());
  QM_FIELDS.forEach(([id, key]) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.addEventListener(key === "reference" ? "input" : "change", (event) => { queueMetrics[key] = event.target.value; paintQueueMetricsResult(); });
  });
}

function paintQueueMetricsResult() {
  const target = document.getElementById("queue-metrics-result");
  if (target) target.innerHTML = queueMetricsTable();
}

function paintQueueMetrics() {
  const target = document.querySelector(".cm-viewer");
  if (target && route() === "queue-metrics") { target.outerHTML = queueMetricsPage(); bindQueueMetrics(); }
}

async function loadQueueMetrics() {
  queueMetricsController?.abort();
  const activeController = new AbortController();
  queueMetricsController = activeController;
  queueMetrics.state = "loading";
  queueMetrics.message = "";
  paintQueueMetrics();
  try {
    const result = await adminPortalService.queueMetrics({ signal: activeController.signal });
    if (queueMetricsController !== activeController) return;
    queueMetrics.jobs = Array.isArray(result) ? result : (result.jobs || result.data || []);
    queueMetrics.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || queueMetricsController !== activeController) return;
    queueMetrics.state = "error";
    queueMetrics.message = error.message || "Unable to load Queue Metrics.";
  } finally {
    if (queueMetricsController === activeController) { queueMetricsController = undefined; paintQueueMetrics(); }
  }
}

const queueAnalytics = { state: "idle", message: "", data: null };
let queueAnalyticsController;

function queueAnalyticsBody() {
  if (queueAnalytics.state === "loading" && !queueAnalytics.data) return `<div class="na-box"><div>Loading Queue Analytics…</div></div>`;
  if (queueAnalytics.state === "error") return `<div class="na-box"><div><b>Queue Analytics unavailable</b>${esc(queueAnalytics.message)}</div></div>`;
  const d = queueAnalytics.data;
  if (!d) return `<div class="na-box"><div>No Queue Analytics available.</div></div>`;
  const dur = (v) => v == null ? "—" : v < 60 ? v + " sec" : Math.round(v / 60) + " min";
  const cards = [
    ["⇄", "Jobs updated", num(d.loadedRecords), "Last 24h / loaded sample", "t-blue"],
    ["✔", "Completion rate", (Number(d.completionRate || 0)).toFixed(1) + "%", "Last 24h / loaded sample", "t-green"],
    ["◷", "Average completion", dur(d.averageCompletionSeconds), "Last 24h / loaded sample", "t-violet"],
    ["⚠", "Dead jobs", num(d.dead), "Last 24h / loaded sample", "t-red"],
  ];
  const kpiHTML = cards.map(([icon, label, value, note, tone]) => `<article class="kpi ${tone}"><i>${icon}</i><div><span>${esc(label)}</span><strong>${esc(value)}</strong><small>${esc(note)}</small></div></article>`).join("");
  const statuses = [["Received", d.received], ["Queued", d.queued], ["Completed", d.completed], ["Dead", d.dead]];
  const statusHTML = `<div class="counts">${statuses.map(([label, count]) => `<div><span>${esc(label)}</span><strong>${num(count)}</strong></div>`).join("")}</div>`;
  const actions = d.actions || [];
  const max = Math.max(1, ...actions.map((a) => a.count));
  const actionHTML = actions.length ? `<div class="rows">${actions.map((a, i) => `<div class="row"><b>${esc(a.actionType)}</b><div class="track"><i style="width:${a.count / max * 100}%;background:${SERIES[i % SERIES.length]}"></i></div><strong>${num(a.count)}</strong></div>`).join("")}</div>` : `<div class="na-box"><div>No Queue Metrics updates in the last 24 hours.</div></div>`;
  return `<section class="kpis">${kpiHTML}</section>
    <section class="grid g-2">
      <article class="panel"><div class="p-head"><h2>Current distribution</h2><span class="tag">24h sample</span></div>${statusHTML}</article>
      <article class="panel"><div class="p-head"><h2>Action type volume</h2><span class="tag">24h sample</span></div>${actionHTML}</article>
    </section>
    <p class="note">${esc(d.scope || "")}</p>`;
}

function queueAnalyticsPage() {
  return `<section class="panel"><div class="p-head"><div><h2>Queue Analytics</h2><p class="df-subtitle">Status distribution, completion performance, and action breakdown for the last 24h sample.</p></div><button class="df-refresh" id="qa-refresh" type="button">Refresh</button></div><div id="queue-analytics-body">${queueAnalyticsBody()}</div></section>`;
}

function bindQueueAnalytics() {
  document.getElementById("qa-refresh")?.addEventListener("click", () => loadQueueAnalytics());
}

function paintQueueAnalytics() {
  const target = document.getElementById("queue-analytics-body");
  if (target) target.innerHTML = queueAnalyticsBody();
}

async function loadQueueAnalytics() {
  queueAnalyticsController?.abort();
  const activeController = new AbortController();
  queueAnalyticsController = activeController;
  queueAnalytics.state = "loading";
  queueAnalytics.message = "";
  paintQueueAnalytics();
  try {
    queueAnalytics.data = await adminPortalService.queueSummary({ signal: activeController.signal });
    if (queueAnalyticsController !== activeController) return;
    queueAnalytics.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || queueAnalyticsController !== activeController) return;
    queueAnalytics.state = "error";
    queueAnalytics.message = error.message || "Unable to load Queue Analytics.";
  } finally {
    if (queueAnalyticsController === activeController) { queueAnalyticsController = undefined; paintQueueAnalytics(); }
  }
}

const busmap = { state: "idle", message: "", records: [], outcome: "", action: "", operator: "", repaired: "", trip: "" };
let busmapController;
const BUSMAP_FIELDS = [["bm-outcome", "outcome"], ["bm-action", "action"], ["bm-operator", "operator"], ["bm-repaired", "repaired"], ["bm-trip", "trip"]];

function busmapOptions(values, selected) {
  return [...new Set(values.filter(Boolean))].sort().map((v) => `<option value="${esc(v)}"${selected === v ? " selected" : ""}>${esc(v)}</option>`).join("");
}

function busmapVisible() {
  const trip = busmap.trip.trim().toLowerCase();
  return busmap.records.filter((r) =>
    (!busmap.outcome || r.verificationOutcome === busmap.outcome) &&
    (!busmap.action || r.actionType === busmap.action) &&
    (!busmap.operator || r.operatorCode === busmap.operator) &&
    (!busmap.repaired || String(r.cacheRepaired) === busmap.repaired) &&
    (!trip || (r.tripCode || "").toLowerCase().includes(trip)));
}

function busmapTable() {
  if (busmap.state === "loading") return `<div class="na-box"><div>Loading busmap data analytics…</div></div>`;
  if (busmap.state === "error") return `<div class="na-box"><div><b>Unable to load busmap analytics</b>${esc(busmap.message)}</div></div>`;
  const visible = busmapVisible();
  if (!visible.length) return `<div class="na-box"><div>No loaded records match the selected filters.</div></div>`;
  const stamp = (v) => v ? new Date(v).toLocaleString() : "—";
  const rows = visible.map((r) => `<tr><td>${esc(stamp(r.detectedAt))}</td><td>${esc(r.operatorCode)}</td><td>${esc(r.actionType)}</td><td>${esc(r.tripCode)}</td><td>${esc(r.tripStageCode)}</td><td>${esc(r.fromCode)}</td><td>${esc(r.toCode)}</td><td>${esc(r.tripDate)}</td><td>${chip(r.verificationOutcome, r.verificationOutcome === "DIFFERENT" ? "warn" : "")}</td><td>${num(r.differenceCount)}</td><td>${r.cacheRepaired ? "Yes" : "No"}</td></tr>`).join("");
  return `<div class="t-wrap"><table><thead><tr><th>Detected</th><th>Operator</th><th>Action</th><th>Trip code</th><th>Stage</th><th>From</th><th>To</th><th>Trip date</th><th>Outcome</th><th>Diffs</th><th>Repaired</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function busmapPage() {
  const recs = busmap.records;
  const note = busmap.state === "done" ? `Filters apply to the loaded difference records (maximum ${num(recs.length)}).` : "";
  return `<section class="panel cm-viewer"><div class="p-head"><div><h2>Busmap Data Analytics</h2><p class="cm-subtitle">Cache-versus-BITS differences recorded when a live read found the cache stale.</p></div><button class="df-refresh" id="bm-refresh" type="button">Refresh</button></div>
    <form class="cm-form" id="busmap-form" autocomplete="off">
      <label>Outcome<select id="bm-outcome"><option value="">All outcomes</option>${["DIFFERENT","CACHE_MISSING"].map((o) => `<option value="${o}"${busmap.outcome === o ? " selected" : ""}>${o}</option>`).join("")}</select></label>
      <label>Action<select id="bm-action"><option value="">All actions</option>${busmapOptions(recs.map((r) => r.actionType), busmap.action)}</select></label>
      <label>Operator<select id="bm-operator"><option value="">All operators</option>${busmapOptions(recs.map((r) => r.operatorCode), busmap.operator)}</select></label>
      <label>Repaired<select id="bm-repaired"><option value="">Any</option><option value="true"${busmap.repaired === "true" ? " selected" : ""}>Repaired</option><option value="false"${busmap.repaired === "false" ? " selected" : ""}>Not repaired</option></select></label>
      <label>Trip code<input id="bm-trip" type="search" placeholder="Search trip code" value="${esc(busmap.trip)}"></label>
    </form>
    ${note ? `<p class="note">${esc(note)}</p>` : ""}
    <div id="busmap-result">${busmapTable()}</div></section>`;
}

function bindBusmap() {
  document.getElementById("bm-refresh")?.addEventListener("click", () => loadBusmap());
  BUSMAP_FIELDS.forEach(([id, key]) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.addEventListener(key === "trip" ? "input" : "change", (event) => { busmap[key] = event.target.value; paintBusmapResult(); });
  });
}

function paintBusmapResult() {
  const target = document.getElementById("busmap-result");
  if (target) target.innerHTML = busmapTable();
}

function paintBusmap() {
  const target = document.querySelector(".cm-viewer");
  if (target && route() === "busmap-analytics") { target.outerHTML = busmapPage(); bindBusmap(); }
}

async function loadBusmap() {
  busmapController?.abort();
  const activeController = new AbortController();
  busmapController = activeController;
  busmap.state = "loading";
  busmap.message = "";
  paintBusmap();
  try {
    const result = await adminPortalService.busmapAnalytics({ signal: activeController.signal });
    if (busmapController !== activeController) return;
    busmap.records = Array.isArray(result) ? result : (result.records || result.data || []);
    busmap.state = "done";
  } catch (error) {
    if (error.name === "AbortError" || busmapController !== activeController) return;
    busmap.state = "error";
    busmap.message = error.message || "Unable to load busmap analytics.";
  } finally {
    if (busmapController === activeController) { busmapController = undefined; paintBusmap(); }
  }
}

function reportsPage() {
  // Every report opens as an in-SPA section so the sidebar always stays visible.
  const cards = [
    ["Top Routes", "Rank the most-booked from/to routes by count over a trip-date range.", "/orbitplus/admin/top-routes", "Open report →"],
    ["Queue Metrics", "Inspect recent Queue Metrics lifecycle records and refine them with filters.", "/orbitplus/admin/queue-metrics", "Open report →"],
    ["Queue Analytics", "Status distribution, completion performance, action breakdown, and failures.", "/orbitplus/admin/queue-analytics", "Open analytics →"],
    ["Busmap Data Analytics", "Cache-versus-Bits differences recorded when a live read found the cache stale.", "/orbitplus/admin/busmap-analytics", "Open analytics →"],
  ];
  return `<section class="rep-grid">${cards.map(([title, desc, url, action]) =>
    `<a class="rep-card" href="${url}"><strong>${esc(title)}</strong><span>${esc(desc)}</span><b>${esc(action)}</b></a>`).join("")}</section>`;
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
  if (current === "cache-settings") return cacheSettingsPage();
  if (current === "cache-freshness") return cacheFreshnessPage();
  if (current === "top-routes") return topRoutesPage();
  if (current === "queue-metrics") return queueMetricsPage();
  if (current === "queue-analytics") return queueAnalyticsPage();
  if (current === "busmap-analytics") return busmapPage();
  if (current === "cassandra") return routeMetadataPage();
  if (current === "operators") return operatorsPage();
  if (current === "memory") return memoryPage();
  if (current === "logs") return logsPage();
  if (current === "reports") return reportsPage();
  return dashboard();
}

function bind() {
  document.getElementById("trip-form")?.addEventListener("submit", (event) => { event.preventDefault(); runTripLookup(); });
  TRIP_FIELDS.forEach(([id, key]) => document.getElementById(id)?.addEventListener("input", (event) => { trip[key] = event.target.value; }));
  document.getElementById("bell")?.addEventListener("click", () => { location.href = href("failures"); });
  document.getElementById("burger")?.addEventListener("click", () => document.querySelector(".side")?.scrollIntoView({ behavior: "smooth" }));
  bindUserMenu();
  if (route() === "dragonfly") bindCacheViewer();
  if (route() === "cache-settings") bindCacheSettings();
  if (route() === "cache-freshness") bindCacheFreshness();
  if (route() === "top-routes") bindTopRoutes();
  if (route() === "queue-metrics") bindQueueMetrics();
  if (route() === "queue-analytics") bindQueueAnalytics();
  if (route() === "busmap-analytics") bindBusmap();
  if (route() === "cassandra") bindRouteMetadata();
  if (route() === "operators") bindOperators();
  if (route() === "memory") bindMemory();
  if (route() === "logs") bindLogs();
}

async function load() {
  controller?.abort();
  controller = new AbortController();
  stopMemoryAutoRefresh();
  const current = route();
  mount.innerHTML = shell(current, `<section class="kpis">${Array.from({ length: 8 }, () => '<div class="skeleton"></div>').join("")}</section>`);
  try {
    overview = await adminPortalService.dashboard({ signal: controller.signal });
    mount.innerHTML = shell(current, page(current));
    bind();
    if (current === "dragonfly") loadCacheViewer(true);
    if (current === "cache-settings") loadCacheSettings();
    if (current === "cache-freshness") loadCacheFreshness();
    if (current === "top-routes") loadTopRoutes();
    if (current === "queue-metrics") loadQueueMetrics();
    if (current === "queue-analytics") loadQueueAnalytics();
    if (current === "busmap-analytics") loadBusmap();
    if (current === "operators") loadOperators();
    if (current === "memory") loadMemory();
    if (current === "logs") loadLogs(true);
  } catch (error) {
    if (error.name === "AbortError") return;
    mount.innerHTML = shell(current, '<div class="err">Unable to load the protected admin snapshot. Check the session and refresh to retry.</div>');
    bind();
  }
}

load();

async function logout() {
  try {
    await fetch("/orbitplus/logout", { method: "POST", credentials: "same-origin" });
  } catch (error) {
    // Ignore network errors; the session cookie clearing is best-effort and we
    // always send the operator back to the login page below.
  }
  location.href = "/orbitplus/login";
}

function bindUserMenu() {
  // The dropdown opens on hover/focus via CSS (.user-menu:hover .user-dropdown).
  // Here we only wire the logout action.
  document.getElementById("logout-item")?.addEventListener("click", () => logout());
}

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
