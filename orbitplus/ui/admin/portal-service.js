export const ADMIN_ENDPOINTS = Object.freeze({ dashboard: "/orbitplus/api/admin/dashboard", tripHistory: "/orbitplus/api/admin/trip-history", cache: "/orbitplus/cache", reports: "/orbitplus/reports", rabbitmq: "/orbitplus/rabbitmq", routeMetadata: "/orbitplus/tables/route-metadata" });

async function readAPI(fetcher, endpoint, signal) {
  const response = await fetcher(endpoint, { credentials: "same-origin", signal });
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
}

export const adminPortalService = new AdminPortalService();
