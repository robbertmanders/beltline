import type { BrowserContext } from "@playwright/test";
import type { SnapshotCase, SnapshotFixture } from "./cases";

export async function applyFixture(context: BrowserContext, origin: string, item: SnapshotCase, fixture: SnapshotFixture): Promise<void> {
  if (fixture.kind === "agents-empty") {
    await context.route(url => url.origin === origin && url.pathname === "/api/v1/agents", route => route.fulfill({ json: {
      agents: [], actions: [{ id: "new", label: "New agent", enabled: true }],
      browser: { items: [], filters: [{ id: "all", label: "All", count: 0 }, { id: "used", label: "Used", count: 0 },
        { id: "unused", label: "Unused", count: 0 }, { id: "archived", label: "Archived", count: 0 }], totalCount: 0, matchCount: 0,
        actions: [{ id: "new", label: "New agent", enabled: true }] }
    } }));
    return;
  }
  if (fixture.kind === "empty-model-choices" || fixture.kind === "unavailable-model-choice") {
    await context.route(`${origin}/api/v1/models/${encodeURIComponent(fixture.runner)}`, async route => {
      const body = fixture.kind === "empty-model-choices"
        ? { runner: fixture.runner, executable: fixture.runner, catalog: null, usable: true, loading: false, error: null,
            allowedModels: [], choices: [], unavailableModels: [], thinking: [], thinkingNote: null, listing: [],
            actions: { refresh: { available: true, refusalReason: null }, candidates: [] } }
        : { runner: fixture.runner, executable: fixture.runner, catalog: null, usable: true, loading: false, error: null,
            allowedModels: ["openrouter/stealth/space-bunny-alpha"], choices: [{ id: "fixture-model", name: "Fixture model" }],
            unavailableModels: ["openrouter/stealth/space-bunny-alpha"], thinking: [{ id: "low", label: "Low" }], thinkingNote: null, listing: [],
            actions: { refresh: { available: true, refusalReason: null }, candidates: [] } };
      await route.fulfill({ json: body });
    });
    return;
  }
  const listResponse = await context.request.get(`${origin}/api/v1/agents`);
  if (!listResponse.ok()) throw new Error(`${item.id}: agent fixture could not list agents (HTTP ${listResponse.status()})`);
  const list = await listResponse.json() as { agents: Array<{ agent: Record<string, unknown>; archived: boolean }> };
  const record = list.agents.find(entry => entry.agent.id === fixture.id);
  if (!record) throw new Error(`${item.id}: agent fixture could not find ${fixture.id}`);
  if (fixture.kind === "ensure-agent-archived") {
    if (record.archived !== fixture.archived) {
      const action = fixture.archived ? "archive" : "unarchive";
      const response = await context.request.post(`${origin}/api/v1/agents/${encodeURIComponent(fixture.id)}/${action}`, { data: {} });
      if (!response.ok()) throw new Error(`${item.id}: could not set ${fixture.id} archived=${fixture.archived} (HTTP ${response.status()})`);
    }
    return;
  }
}
