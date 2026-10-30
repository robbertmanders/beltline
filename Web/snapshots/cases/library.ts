import { shellCases, sidebarState, type SnapshotCase } from "../caseTypes";

/** A daemon action as the library answers it: enabled, or disabled with the reason it gives. */
const action = (id: string, enabled = true, reason?: string) => (enabled ? { id, enabled } : { id, enabled, reason });

/** Two repositories: your backup, changed since its last backup, and a shared library with items in each state. */
const repositories = {
  repos: [
    {
      repository: "acme/beltline-backup", path: "", role: "backup", publish: "pullRequest", takesShares: false, syncedAt: "2026-10-14T09:00:00Z", commit: "4f2a9c1",
      problem: null, changedSinceBackup: true, items: [],
      actions: [action("refresh"), action("backup"), action("restore")]
    },
    {
      repository: "acme/team-library", path: "shared", role: "shared", publish: "pullRequest", takesShares: true, syncedAt: "2026-10-14T09:00:00Z", commit: "9c0be37",
      problem: null,
      items: [
        { kind: "belt", id: "issue-flow", name: "Issue flow", version: 3, localVersion: 2, state: "update", reason: null, actions: [action("install")] },
        { kind: "agent", id: "review", name: "Review", version: 1, localVersion: 1, state: "installed", reason: null, actions: [action("install", false, "This version is already in your library.")] },
        {
          kind: "agent", id: "triage", name: "Triage", version: 2, localVersion: 1, state: "conflict",
          reason: "You changed triage v2 locally; save it under a new id before updating.", actions: [action("install", false, "You changed triage v2 locally.")]
        }
      ],
      actions: [action("refresh")]
    }
  ]
};

const comparedFiles = {
  files: [{
    path: "belts/issue-flow/v3.json",
    repo: '{\n  "id": "issue-flow",\n  "version": 3,\n  "name": "Issue flow"\n}',
    local: '{\n  "id": "issue-flow",\n  "version": 2,\n  "name": "Issue flow"\n}'
  }]
};

const repositoriesResponse = { path: "/api/v1/library/repos", status: 200, body: JSON.stringify(repositories) };

/**
 * The Library page and its sheets. The repositories and the comparison are typed API fixtures, so the states (a
 * changed backup, an update, a conflict) are the same on every run. The open and closed pair is the demo's own.
 */
export const libraryCases: SnapshotCase[] = [
  ...shellCases("library", "/library"),
  {
    id: "library-shared",
    route: "/library",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse],
    readyMarker: '.library-item[data-state="update"]'
  },
  {
    id: "library-compare",
    route: "/library",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse, { path: "/api/v1/library/item", status: 200, body: JSON.stringify(comparedFiles) }],
    steps: [{ kind: "click", target: { selector: '.library-item[data-state="update"] > .paper-button' } }],
    readyMarker: '[role="dialog"]'
  },
  {
    id: "library-share-belt",
    route: "/belts?belt=issues",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse],
    steps: [
      { kind: "click", target: { role: "button", name: "Share…", exact: true } },
      { kind: "wait", target: { selector: '[role="dialog"]' }, state: "visible" }
    ],
    readyMarker: '[role="dialog"]'
  }
];
