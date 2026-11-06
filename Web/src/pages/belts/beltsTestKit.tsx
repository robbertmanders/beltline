import { screen } from "@testing-library/react";
import type userEvent from "@testing-library/user-event";
/** Shared by the Belts page tests: a stand-in daemon that answers from the server's goldens, and the editor context it would send. */
import { beltList, type BeltRecord } from "./beltRecords";
import { placeholderStageKind } from "./StageLine";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";
import { renderRoutes, stubDaemon, type DaemonRequest, type Handler } from "../../test/daemon";
import type { Belt, BeltsResponse, StageMenu } from "../../api/wire.gen";

export type { DaemonRequest, Handler };

/** The belts daemon: the goldens, the routes given, and the browser view the daemon computes for the belts list. */
export function mockDaemon(routes: Record<string, Handler | string> = {}) {
  const daemon = stubDaemon(routes, (request, response) => request.method === "GET" && (request.path.split("?")[0] ?? "") === "/api/v1/belts"
    ? withBrowserResponse(response, request.path) : response);
  const posts = (suffix = "") => daemon.posts(suffix).filter(request => request.path !== "/api/v1/belts/context" && request.path !== "/api/v1/belts/stage-template");
  const contextReads = () => daemon.requests.filter(request => request.path === "/api/v1/belts/context");
  return { requests: daemon.requests, posts, contextReads };
}

function liveIdsOf(records: BeltRecord[]): string[] {
  return records.filter(record => !record.archived).map(record => record.belt.id);
}

/** The belts list response for `records`, with the live ids the daemon sends beside them. */
export function beltsList(records: BeltRecord[]) {
  return { belts: records, liveIds: liveIdsOf(records) };
}

function browserFor(records: BeltRecord[], path: string) {
  const url = new URL(path, "http://beltline.local");
  const search = (url.searchParams.get("search") ?? "").trim().toLocaleLowerCase();
  const filter = url.searchParams.get("filter") ?? "all";
  const cards = records.map(record => {
    const status = record.archived ? "Archived" : record.paused || record.effectivelyPaused ? "Paused" : "Running";
    return {
      id: record.belt.id, name: record.belt.name || "Unnamed belt", version: record.belt.version,
      summary: record.detail, detail: record.detail, status,
      statusTone: record.archived || record.paused || record.effectivelyPaused ? "grey" : "good",
      stages: []
    };
  });
  const matchesFilter = (card: (typeof cards)[number]) => filter === "all"
    || (filter === "running" && card.status === "Running")
    || (filter === "paused" && card.status === "Paused")
    || (filter === "archived" && card.status === "Archived");
  const items = cards.filter(card => matchesFilter(card) && (!search || `${card.name} ${card.summary} ${card.detail}`.toLocaleLowerCase().includes(search)));
  const count = (predicate: (card: (typeof cards)[number]) => boolean) => cards.filter(predicate).length;
  return {
    items, totalCount: cards.length, matchCount: items.length,
    filters: [
      { id: "all", label: "All", count: cards.length },
      { id: "running", label: "Running", count: count(card => card.status === "Running") },
      { id: "paused", label: "Paused", count: count(card => card.status === "Paused") },
      { id: "archived", label: "Archived", count: count(card => card.status === "Archived") }
    ],
    actions: [{ id: "new", label: "New belt", enabled: true }]
  };
}

async function withBrowserResponse(response: Response, path: string): Promise<Response> {
  try {
    const body = await response.clone().json() as { belts?: BeltRecord[]; browser?: unknown };
    if (!body || !Array.isArray(body.belts) || body.browser !== undefined) return response;
    return json({ ...body, liveIds: liveIdsOf(body.belts), browser: browserFor(body.belts, path) }, response.status);
  } catch {
    return response;
  }
}

export const fixture = beltList(JSON.parse(goldenText("belts.json")) as BeltsResponse);
export const firstLive = fixture.belts.find(record => !record.archived)!;
export const firstName = firstLive.belt.name;
export const firstStage = firstLive.belt.stages[0]!;
export const goldenContext = JSON.parse(goldenText("belt-context.json"));

/** A stage menu in the daemon's shape for one stage of `belt`: every other stage as a goto target with no counterparts, unless given. */
export function menuFor(belt: Belt, stageId: string, overrides: Partial<StageMenu> = {}): StageMenu {
  const base = goldenContext.stage as StageMenu;
  const index = belt.stages.findIndex(stage => stage.id === stageId);
  const stage = belt.stages[index]!;
  return {
    ...base,
    stage: stageId,
    number: index + 1,
    kind: stage.type === "approve" ? "Approval" : stage.type === "watch" ? "Watcher" : "Agent stage",
    canMoveEarlier: index > 0,
    canMoveLater: index < belt.stages.length - 1,
    watchTargets: stage.type === "watch" ? [{ id: "pullRequest", label: "The pull request" }, { id: "ticket", label: "The issue or ticket" }] : [],
    watchEvents: stage.type === "watch" ? [{ id: "merged", label: "the pull request was merged" }, { id: "closed", label: "the pull request was closed" }] : [],
    watchNote: stage.type === "watch" ? "Watches the run's pull request." : undefined,
    routesAfter: [],
    routePaths: belt.forks.flatMap(fork => fork.routes.map(route => ({ id: route.id, label: route.name }))),
    gotoTargets: belt.stages.filter(item => item.id !== stageId).map(item => ({ id: item.id, label: item.name, counterparts: [], others: [] })),
    ...overrides
  };
}

/** The stage line the daemon would send: line stages as blocks, each fork as a box with its routes' stages. */
export function lineFor(belt: Belt) {
  const index = (id: string) => belt.stages.findIndex(stage => stage.id === id);
  const block = (stage: Belt["stages"][number]) => ({ addAt: index(stage.id), stage: { id: stage.id, kind: placeholderStageKind(stage, index(stage.id) + 1), subtitle: [] as string[] } });
  const items: unknown[] = [];
  const placed = new Set<string>();
  const box = (fork: Belt["forks"][number], at: number) => {
    placed.add(fork.id);
    items.push({ addAt: at, fork: { id: fork.id, heading: `FORK · ${fork.routes.length} ROUTES`, feeding: false, routes: fork.routes.map((route, at) => {
      const stages = belt.stages.filter(item => item.route === route.id);
      return { id: route.id, caption: at === 0 ? "ROUTE · FIRST" : "ROUTE", help: "", removable: true, opens: true, joins: true, items: stages.map(block),
        addEnd: stages.length ? index(stages[stages.length - 1]!.id) + 1 : at, ...(stages.length ? {} : { empty: "No stages: runs skip ahead" }) };
    }) } });
  };
  belt.stages.forEach((stage, at) => {
    for (const fork of belt.forks) if (!placed.has(fork.id) && fork.at === at && !belt.stages.some(item => fork.routes.some(route => route.id === item.route))) box(fork, at);
    if (!stage.route) { items.push(block(stage)); return; }
    const fork = belt.forks.find(item => item.routes.some(route => route.id === stage.route));
    if (fork && !placed.has(fork.id)) box(fork, at);
  });
  for (const fork of belt.forks) if (!placed.has(fork.id)) box(fork, belt.stages.length);
  return { trigger: { kind: "Trigger", title: "GitHub issues", lines: ["1 repository", "label: agent"] }, items, addEnd: belt.stages.length,
    agents: [{ id: "build", version: 1, name: "Build" }], watchAllowed: true };
}

/** The same line with every stage's agent a version behind the library. */
export function outdatedLine(belt: Belt) {
  const line = lineFor(belt);
  return { ...line, items: line.items.map(item => (item as { stage?: object }).stage ? { ...(item as object), stage: { ...(item as { stage: object }).stage, update: 2 } } : item) };
}

/** The context the daemon would answer for the belt on screen: its stages, outcome labels, policies, trigger form and, for a stage, its menu. */
export function contextFor(belt: Belt, label = "Next stage", stageId?: string, menu: Partial<StageMenu> = {}) {
  const stage = stageId ? belt.stages.find(item => item.id === stageId) : undefined;
  const policies = [
    { key: "retriageOnEdit", label: "Start again when the belt is edited", detail: "Retriage detail", enabled: belt.policies.retriageOnEdit },
    { key: "stopOnClose", label: "Stop when the work is closed", detail: "Stop detail", enabled: belt.policies.stopOnClose },
    { key: "commentOnHold", label: "Comment when a stage holds the run", detail: "Comment detail", enabled: belt.policies.commentOnHold },
    { key: "waitForBlockers", label: "Wait for blocking work", detail: "Wait detail", enabled: belt.policies.waitForBlockers }
  ];
  return {
    actions: [{ id: "save", enabled: true }, { id: "update-stage", enabled: true }, { id: "update-all-stages", enabled: true }],
    warnings: [],
    placeholders: { entries: [{ name: "build", description: "The build output" }], legacy: false },
    routes: belt.forks.flatMap(fork => fork.routes.map(route => ({ id: route.id, name: route.name, fork: fork.id }))),
    stages: belt.stages.map(item => ({ id: item.id, name: item.name })),
    outcomes: belt.stages.flatMap(item => item.outcomes.map(outcome => ({ stage: item.id, outcome: outcome.id, label }))),
    schedules: [],
    policies,
    policySection: { heading: "While an issue is on the belt", options: policies, note: "A base label note." },
    triggerForm: goldenContext.triggerForm,
    line: lineFor(belt),
    ...(stage ? { stage: menuFor(belt, stage.id, menu) } : {}),
    ...(stage?.agent ? { agent: { id: stage.agent.id, version: stage.agent.version, name: `Agent ${stage.agent.id}`, brief: `The ${stage.agent.id} brief.`, key: stage.agent.id, archived: false, missing: false } } : {})
  };
}

/**
 * The repository base group the daemon sends for a draft: one field per repository the triggers list, in
 * first-appearance order, and none when no trigger takes a branch (only pull request reviews do not).
 */
export function repositoryBasesFor(belt: Belt) {
  const fields = [...new Set(belt.triggers.flatMap(entry => entry.repos))].map(repository => ({ repository, label: repository }));
  const showsBase = belt.triggers.some(entry => entry.trigger.kind !== "reviews");
  if (!showsBase || fields.length === 0) return undefined;
  return { heading: "Repository base branches", help: "An empty field starts that repository from the shared base branch, or from its default branch when the shared base is empty.", placeholder: "Use shared base branch", fields };
}

/** The context for a draft, with the repository base group the daemon would compute for it. */
export function contextWithBases(belt: Belt) {
  const context = contextFor(belt);
  const showsBase = belt.triggers.some(entry => entry.trigger.kind !== "reviews");
  return { ...context, triggerForm: { ...context.triggerForm, showsBase, repositoryBases: repositoryBasesFor(belt) } };
}

/** Answers every context read for the belt it was sent, with the stage it asked about. */
export const contextRoute = (label?: string, menu?: Partial<StageMenu>): Handler => request => json(contextFor(request.body.draft, label, request.body.stageId, menu));

/** Renders the belts page; a bare /belts opens the first live belt, as the page's first load does. */
export function renderAt(path: string) {
  const editorPath = path === "/belts" ? `/belts?belt=${encodeURIComponent(firstLive.belt.id)}` : path;
  return renderRoutes(editorPath);
}

// Helpers for the page-level rule tests.
export const docs = fixture.belts.find(record => record.belt.id === "docs")!;
export const answer = (name: string) => JSON.parse(goldenText(name));
/** The golden answer with the submitted draft's id and name, so it stays the belt on screen. */
export const echo = (name: string): Handler => request => json({ ...answer(name), draft: { ...answer(name).draft, id: request.body.draft.id, name: request.body.draft.name } });
export const nameInput = () => screen.getByLabelText("Belt name") as HTMLInputElement;
export const heading = (name: string) => screen.findByRole("heading", { level: 2, name });
/** Opens a saved belt from the daemon-backed browser, returning to the browser when starting from detail. */
export const selectBelt = async (user: ReturnType<typeof userEvent.setup>, name: string) => {
  const back = screen.queryByRole("button", { name: /Back to Belts/ });
  if (back) await user.click(back);
  return user.click(await screen.findByRole("button", { name: `Open ${name}` }));
};

/** A saved answer for `record` with some fields changed, in the shape save, pause and archive return. */
export const resource = (record: BeltRecord, change: Partial<BeltRecord>) => json({ resource: { ...record, ...change }, warnings: [] });

/** A library that a command changes, so the refetch after it reads the changed belt. */
export function library() {
  let belts = fixture.belts.map(record => ({ ...record }));
  return {
    read: ((request: DaemonRequest) => json({ belts, liveIds: liveIdsOf(belts), browser: browserFor(belts, request.path) })) as Handler,
    change: (id: string, change: Partial<BeltRecord>): Response => {
      belts = belts.map(record => record.belt.id === id ? { ...record, ...change } : record);
      return json({ resource: belts.find(record => record.belt.id === id)!, warnings: [] });
    }
  };
}
