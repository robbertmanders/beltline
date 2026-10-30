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

/** The library's daemon: the routes given, the version from the golden, and anything else a 404. */
export function mockLibrary(routes: Record<string, Handler | string> = {}) {
  return stubDaemon(routes);
}
