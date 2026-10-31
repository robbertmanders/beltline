/** Shared by the Library tests: a stand-in daemon with inline library answers, and the two repositories they describe. */
import { stubDaemon, type Handler } from "../../test/daemon";

/** A daemon action as the library answers it: enabled, or disabled with the reason it gives. */
const action = (id: string, enabled = true, reason?: string) => (enabled ? { id, enabled } : { id, enabled, reason });

export const backupRepo = {
  repository: "acme/beltline-backup", path: "", role: "backup", publish: "pullRequest", takesShares: false, syncedAt: "2026-10-14T09:00:00Z", commit: "4f2a9c1",
  problem: null, changedSinceBackup: true, items: [],
  actions: [action("refresh"), action("backup"), action("restore")]
};

export const sharedRepo = {
  repository: "acme/team-library", path: "", role: "shared", publish: "pullRequest", takesShares: true, syncedAt: "2026-10-14T09:00:00Z", commit: "9c0be37",
  problem: null,
  items: [
    { kind: "belt", id: "issue-flow", name: "Issue flow", version: 3, localVersion: 2, state: "update", reason: null, actions: [action("install")] },
    { kind: "agent", id: "review", name: "Review", version: 1, localVersion: 1, state: "installed", reason: null, actions: [action("install", false, "This version is already in your library.")] },
    {
      kind: "agent", id: "triage", name: "Triage", version: 2, localVersion: 1, state: "conflict",
      reason: "You changed triage v1 locally; save it under a new id before updating.", actions: [action("install", false, "You changed triage v1 locally.")]
    }
  ],
  actions: [action("refresh")]
};

/** A Library card as the browser answers it, for a shared repository's belt or agent in the state given. */
const card = (kind: "belt" | "agent", id: string, name: string, state: string, status: string, statusTone: string) => ({
  id, name, version: 1, summary: "", detail: "acme/team-library", status, statusTone, state, icon: kind === "belt" ? "bolt" : "eye", tint: "grey",
  stages: kind === "belt" ? [{ icon: "funnel", tint: "blue" }, { icon: "wrench", tint: "green" }] : [], kind, repository: "acme/team-library", path: ""
});

/** The two tabs of the Library page over sharedRepo: one belt with an update, two agents. */
export const libraryTabs = [{ id: "belt", label: "Belts", count: 1, attention: true }, { id: "agent", label: "Agents", count: 2, attention: true }];

/** The browser's answer for kind: the cards of that tab, with the state filters and their counts. */
export function libraryBrowser(kind: "belt" | "agent") {
  const cards = kind === "belt" ? [card("belt", "issue-flow", "Issue flow", "update", "Update", "gold")]
    : [card("agent", "review", "Review", "installed", "Installed", "good"), card("agent", "triage", "Triage", "conflict", "Conflict", "problem")];
  const filters = [["all", "All"], ["available", "Available"], ["installed", "Installed"], ["update", "Updates"], ["conflict", "Conflicts"]]
    .map(([id, label]) => ({ id: id!, label: label!, count: cards.filter(entry => id === "all" || entry.state === id).length }));
  const items = cards.map(({ state: _state, ...rest }) => rest);
  return { tabs: libraryTabs, items, filters, totalCount: items.length, matchCount: items.length };
}

/** The browser route: each tab's cards by the kind asked for. */
export const libraryBrowserRoute: Handler = request => new Response(JSON.stringify(libraryBrowser(
  new URLSearchParams(request.path.split("?")[1]).get("kind") === "agent" ? "agent" : "belt")), { status: 200, headers: { "content-type": "application/json" } });

/** The library's daemon: the routes given, the browser over sharedRepo unless a test gives its own, and anything else a 404. */
export function mockLibrary(routes: Record<string, Handler | string> = {}) {
  return stubDaemon({ "/api/v1/library/browser": libraryBrowserRoute, ...routes });
}
