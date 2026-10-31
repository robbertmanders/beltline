import { readFileSync } from "node:fs";
import { resolve } from "node:path";
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

/** A Library card as the browser answers it. */
const card = (kind: "belt" | "agent", id: string, name: string, status: string, statusTone: string, icon: string, tint: string, stages: { icon: string; tint: string }[] = []) => ({
  id, name, version: 3, summary: "", detail: "acme/team-library / shared", status, statusTone, icon, tint, stages, kind, repository: "acme/team-library", path: "shared"
});

/** The Belts tab over the shared repository: a belt in each state, with their stage strips. */
const beltBrowser = {
  tabs: [{ id: "belt", label: "Belts", count: 3, attention: true }, { id: "agent", label: "Agents", count: 2, attention: true }],
  items: [
    card("belt", "issue-flow", "Issue flow", "Update", "gold", "bolt", "grey", [{ icon: "funnel", tint: "blue" }, { icon: "doc", tint: "sand" }, { icon: "approve", tint: "gold" }, { icon: "wrench", tint: "green" }, { icon: "scope", tint: "violet" }, { icon: "ship", tint: "coral" }, { icon: "merge", tint: "teal" }]),
    card("belt", "jira-tickets", "Jira tickets", "Available", "grey", "jira", "grey", [{ icon: "funnel", tint: "blue" }, { icon: "wrench", tint: "green" }, { icon: "ship", tint: "coral" }]),
    card("belt", "reviews", "Pull request reviews", "Installed", "good", "eye", "grey", [{ icon: "eye", tint: "rose" }, { icon: "send", tint: "rose" }])
  ],
  filters: [{ id: "all", label: "All", count: 3 }, { id: "available", label: "Available", count: 1 }, { id: "installed", label: "Installed", count: 1 },
    { id: "update", label: "Updates", count: 1 }, { id: "conflict", label: "Conflicts", count: 0 }],
  totalCount: 3, matchCount: 3
};

const golden = (name: string) => readFileSync(resolve(process.cwd(), `../daemon/testdata/golden/${name}`), "utf8");

const comparedFiles = {
  files: [{
    path: "belts/team-nightly/v1.json",
    repo: '{\n  "id": "team-nightly",\n  "version": 1,\n  "name": "Nightly (team)"\n}',
    local: null
  }]
};

const repositoriesResponse = { path: "/api/v1/library/repos", status: 200, body: JSON.stringify(repositories) };
const browserResponse = { path: "/api/v1/library/browser", status: 200, body: JSON.stringify(beltBrowser) };
const beltResponse = { path: "/api/v1/library/belt", status: 200, body: golden("library-belt.json") };

/**
 * The Library page and its read-only editors. The repositories, the cards and the comparison are typed API fixtures,
 * and the belt and agent are the contract goldens, so the states (a changed backup, an update, a conflict) are the
 * same on every run. The open and closed pair is the demo's own.
 */
export const libraryCases: SnapshotCase[] = [
  ...shellCases("library", "/library"),
  {
    id: "library-shared",
    route: "/library",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse, browserResponse],
    readyMarker: ".library-card-grid"
  },
  {
    id: "library-belt",
    route: "/library?tab=belt&repository=example%2Fshared&path=&id=team-nightly",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse, beltResponse],
    readyMarker: ".stage-line-card"
  },
  {
    id: "library-belt-stage",
    route: "/library?tab=belt&repository=example%2Fshared&path=&id=team-nightly",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse, beltResponse],
    steps: [{ kind: "click", target: { role: "button", name: "Sweep", exact: true } }],
    readyMarker: '.stage-form[data-ready="true"]'
  },
  {
    id: "library-agent",
    route: "/library?tab=agent&repository=example%2Fshared&path=&id=team-build",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse, { path: "/api/v1/library/agent", status: 200, body: golden("library-agent.json") }],
    readyMarker: ".agent-editor"
  },
  {
    id: "library-compare",
    route: "/library?tab=belt&repository=example%2Fshared&path=&id=team-nightly",
    sidebar: sidebarState({ navigationOpen: true }),
    readiness: "shell",
    apiResponses: [repositoriesResponse, beltResponse, { path: "/api/v1/library/item", status: 200, body: JSON.stringify(comparedFiles) }],
    steps: [{ kind: "click", target: { role: "button", name: "Compare", exact: true } }],
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
