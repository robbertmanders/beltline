import type { EventResource } from "./wire.gen";

export interface GetRouteDescriptor {
  method: "GET";
  path: string;
  resources?: readonly EventResource[];
  exception?: string;
}

export const getRoutes: readonly GetRouteDescriptor[] = [
  { method: "GET", path: "/auth", exception: "Cookie exchange redirects the browser; it is not a cached API read." },
  { method: "GET", path: "/api/v1/version", exception: "Version is tracked independently and checked on reconnect." },
  { method: "GET", path: "/api/v1/state", resources: ["state", "library", "afk"] },
  { method: "GET", path: "/api/v1/runs", resources: ["runs", "library", "settings", "state"] },
  { method: "GET", path: "/api/v1/runs/:id", resources: ["runs", "library", "settings", "state"] },
  { method: "GET", path: "/api/v1/runs/:id/attempts/:attempt/output", resources: ["runs", "library", "settings", "state"] },
  { method: "GET", path: "/api/v1/runs/:id/attempts/:attempt/artifacts", resources: ["runs", "library", "settings", "state"] },
  { method: "GET", path: "/api/v1/views/overview", resources: ["runs", "library", "settings", "state"] },
  { method: "GET", path: "/api/v1/views/work", resources: ["runs", "library", "settings", "state", "intake"] },
  { method: "GET", path: "/api/v1/views/run/:id", resources: ["runs", "library", "settings", "state"] },
  { method: "GET", path: "/api/v1/views/run/:id/sessions", resources: ["runs", "library", "settings", "state"] },
  { method: "GET", path: "/api/v1/views/insights", resources: ["runs", "library"] },
  { method: "GET", path: "/api/v1/views/insights/evolution", resources: ["runs", "library"] },
  { method: "GET", path: "/api/v1/waiting", resources: ["intake"] },
  { method: "GET", path: "/api/v1/afk/decisions", resources: ["afk"] },
  { method: "GET", path: "/api/v1/terminals", exception: "Terminal is a streaming WebSocket transport, not a cacheable response." },
  { method: "GET", path: "/api/v1/belts", resources: ["library", "settings"] },
  { method: "GET", path: "/api/v1/belts/:id", resources: ["library", "settings"] },
  { method: "GET", path: "/api/v1/placeholders", resources: ["library", "settings", "models"] },
  { method: "GET", path: "/api/v1/agents", resources: ["library", "settings", "models"] },
  { method: "GET", path: "/api/v1/agents/:id", resources: ["library", "settings", "models"] },
  { method: "GET", path: "/api/v1/settings", resources: ["settings"] },
  { method: "GET", path: "/api/v1/tools", exception: "Executable availability has no EventResource; this on-demand read is invalidated on reconnect." },
  { method: "GET", path: "/api/v1/models/:runner", resources: ["models", "settings"] },
  { method: "GET", path: "/api/v1/assistant", resources: ["assistant", "settings", "models"] },
  { method: "GET", path: "/api/v1/library/repos", resources: ["library", "settings"] },
  { method: "GET", path: "/api/v1/library/item", resources: ["library"] },
  { method: "GET", path: "/api/v1/library/browser", resources: ["library", "settings"] },
  { method: "GET", path: "/api/v1/library/belt", resources: ["library", "settings"] },
  { method: "GET", path: "/api/v1/library/agent", resources: ["library", "settings"] },
  { method: "GET", path: "/artifacts/:run/:attempt/:name", exception: "Signed artifact HTML is a document response, not a JSON query." },
  { method: "GET", path: "/api/v1/events", exception: "The single authenticated event WebSocket owns this transport." }
];

function routeMatches(template: string, value: string): boolean {
  const templateParts = template.split("/");
  const valueParts = value.split("?")[0]?.split("/") ?? [];
  return templateParts.length === valueParts.length && templateParts.every((part, index) => part.startsWith(":") || part === valueParts[index]);
}

export function routeForPath(path: string): GetRouteDescriptor | undefined {
  return getRoutes.find(route => routeMatches(route.path, path));
}

export function resourcesForPath(path: string): readonly EventResource[] {
  return routeForPath(path)?.resources ?? [];
}

export function apiQueryKey(path: string, variant: unknown = null): readonly unknown[] {
  return ["api", path, variant] as const;
}

export interface SourceEndpoint {
  method: "GET" | "POST";
  path: string;
  caseName: string;
}

export function requireGetCoverage(inventory: readonly SourceEndpoint[], routes: readonly GetRouteDescriptor[] = getRoutes): void {
  const table = new Set(routes.map(route => `${route.method} ${route.path}`));
  const missing = inventory.filter(endpoint => endpoint.method === "GET" && !table.has(`GET ${endpoint.path}`));
  const extra = routes.filter(route => !inventory.some(endpoint => endpoint.method === route.method && endpoint.path === route.path));
  if (missing.length || extra.length) {
    throw new Error(`GET route coverage mismatch; missing: ${missing.map(item => `${item.caseName} ${item.path}`).join(", ") || "none"}; extra: ${extra.map(item => item.path).join(", ") || "none"}`);
  }
  for (const route of routes) {
    if ((route.resources?.length ?? 0) === 0 && !route.exception) throw new Error(`GET route ${route.path} needs an EventResource or an explicit exception.`);
    if (route.resources?.some(resource => !["runs", "library", "settings", "state", "intake", "assistant", "afk", "models"].includes(resource))) {
      throw new Error(`GET route ${route.path} names an unknown EventResource.`);
    }
  }
}

export function requireEventResourceCoverage(resources: readonly string[], routes: readonly GetRouteDescriptor[] = getRoutes): void {
  const covered = new Set<string>(routes.flatMap(route => route.resources ?? []));
  const missing = resources.filter(resource => !covered.has(resource));
  if (missing.length) throw new Error(`EventResource coverage is missing: ${missing.join(", ")}`);
}

/** The resources a cached read depends on, as recorded in its query meta. Event-driven and command-driven refreshes use this same predicate. */
export function queryResources(meta: unknown): EventResource[] {
  if (!meta || typeof meta !== "object") return [];
  const resources = (meta as { resourceDeps?: unknown }).resourceDeps;
  return Array.isArray(resources) ? resources.filter((item): item is EventResource => typeof item === "string") : [];
}
