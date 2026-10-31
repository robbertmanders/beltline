import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { apiQueryKey, getRoutes, requireEventResourceCoverage, requireGetCoverage, resourcesForPath, routeForPath, type SourceEndpoint } from "./resources";

function productionInventory(): SourceEndpoint[] {
  const source = readFileSync(resolve(process.cwd(), "../daemon/internal/server/routes/routes_endpoint_registry_table.go"), "utf8");
  const entries = [...source.matchAll(/\{Case: "([^"]+)", Method: EndpointMethod(Get|Post), Path: "([^"]+)"/g)];
  if (entries.length === 0) throw new Error("The endpoint table lists no endpoints.");
  return entries.map(match => ({ caseName: match[1]!, path: match[3]!, method: match[2] === "Post" ? "POST" : "GET" }));
}

function eventResources(): string[] {
  const source = readFileSync(resolve(process.cwd(), "../daemon/internal/server/events_message.go"), "utf8");
  return [...source.matchAll(/^\s*EventResource[A-Za-z]+\s+EventResource = "([^"]+)"/gm)].map(match => match[1]!);
}

describe("GET resource invalidation coverage", () => {
  it("covers the production GET descriptors and all daemon EventResources", () => {
    const inventory = productionInventory();
    requireGetCoverage(inventory);
    requireEventResourceCoverage(eventResources());
    expect(getRoutes.length).toBeGreaterThan(20);
  });

  it("rejects a new GET or EventResource without coverage", () => {
    const inventory = productionInventory();
    expect(() => requireGetCoverage([...inventory, { method: "GET", path: "/api/v1/future", caseName: "future" }]))
      .toThrow(/future/);
    expect(() => requireEventResourceCoverage([...eventResources(), "futureResource"]))
      .toThrow(/futureResource/);
  });

  it("matches parameterized routes as one encoded segment and keeps query variants distinct", () => {
    const path = "/api/v1/runs/issues%3Aacme%2Fweb%2361";
    expect(routeForPath(path)?.path).toBe("/api/v1/runs/:id");
    expect(resourcesForPath(path)).toEqual(["runs", "library", "settings", "state"]);
    expect(apiQueryKey("/api/v1/runs", { limit: 10 })).not.toEqual(apiQueryKey("/api/v1/runs", { limit: 20 }));
  });

  it("refreshes the settings read when the library changes, since the shadow test switch depends on live tests", () => {
    expect(resourcesForPath("/api/v1/settings")).toEqual(["settings", "library"]);
  });

  it("records explicit exceptions for non-cacheable and unobservable GETs", () => {
    for (const path of ["/api/v1/version", "/api/v1/tools", "/api/v1/terminals", "/api/v1/events", "/artifacts/:run/:attempt/:name", "/auth"]) {
      expect(routeForPath(path)?.exception, path).toBeTruthy();
    }
  });
});
