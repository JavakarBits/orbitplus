export const ADMIN_ENDPOINTS = Object.freeze({ dashboard: "/orbitplus/api/admin/dashboard", tripHistory: "/orbitplus/api/admin/trip-history", operators: "/orbitplus/api/admin/operators", cache: "/orbitplus/cache", cacheKeys: "/orbitplus/api/cache", cacheValue: "/orbitplus/api/cache/value", reports: "/orbitplus/reports", rabbitmq: "/orbitplus/rabbitmq", routeMetadata: "/orbitplus/api/tables/route-metadata" });

async function readAPI(fetcher, endpoint, signal) {
  const response = await fetcher(endpoint, { credentials: "same-origin", signal });
  const payload = await response.json().catch(() => null);
  if (!response.ok) throw new Error(payload?.message || `Request failed (${response.status})`);
  return payload?.data ?? payload;
}

async function writeAPI(fetcher, endpoint, method, data) {
  const response = await fetcher(endpoint, { method, credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data) });
  const payload = await response.json().catch(() => null);
  if (!response.ok) throw new Error(payload?.message || `Request failed (${response.status})`);
  return payload?.data ?? payload;
}

export class AdminPortalService {
  constructor({ fetcher = fetch } = {}) { this.fetcher = fetcher; }
  dashboard({ signal } = {}) { return readAPI(this.fetcher, ADMIN_ENDPOINTS.dashboard, signal); }
  tripHistory({ operatorCode, tripCode, tripDate, fromStation, toStation, signal } = {}) {
    const query = new URLSearchParams({ operatorCode: operatorCode || "", tripCode: tripCode || "", tripDate: tripDate || "", fromStation: fromStation || "", toStation: toStation || "" });
    return readAPI(this.fetcher, `${ADMIN_ENDPOINTS.tripHistory}?${query}`, signal);
  }
  cacheKeys({ cursor = 0, limit = 25, category = "all", signal } = {}) {
    const query = new URLSearchParams({ cursor: String(cursor), limit: String(limit), category });
    return readAPI(this.fetcher, `${ADMIN_ENDPOINTS.cacheKeys}?${query}`, signal);
  }
  cacheValue({ key, signal } = {}) {
    const query = new URLSearchParams({ key: key || "" });
    return readAPI(this.fetcher, `${ADMIN_ENDPOINTS.cacheValue}?${query}`, signal);
  }
  routeMetadata({ operator, travel, from, to, signal } = {}) {
    const query = new URLSearchParams({ operator: operator || "", travel: travel || "", from: from || "", to: to || "" });
    return readAPI(this.fetcher, `${ADMIN_ENDPOINTS.routeMetadata}?${query}`, signal);
  }
  operators({ signal } = {}) { return readAPI(this.fetcher, ADMIN_ENDPOINTS.operators, signal); }
  createOperator(operatorCode, zoneCode) { return writeAPI(this.fetcher, ADMIN_ENDPOINTS.operators, "POST", { operatorCode, zoneCode }); }
  setOperatorActive(operatorCode, active) { return writeAPI(this.fetcher, `${ADMIN_ENDPOINTS.operators}/${encodeURIComponent(operatorCode)}`, "PATCH", { active }); }
}

export const adminPortalService = new AdminPortalService();
