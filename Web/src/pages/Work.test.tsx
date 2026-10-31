import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BrowserRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppProviders, AppRoutes } from "../app/App";
import { AppSocket } from "../test/setup";
import { defaultPreferences, preferencesStorageKey } from "../state/preferences";
import { json } from "../test/response";
import { goldenText } from "../test/golden";
import { renderRoutes, stubDaemon, type Handler } from "../test/daemon";

const workPath = "/api/v1/views/work";

type WorkBody = Record<string, unknown> & { mainGroups: Array<Record<string, unknown>>; watchingGroups: Array<Record<string, unknown>> };
const workGolden = (): WorkBody => JSON.parse(goldenText("views-work.json")) as WorkBody;

/** The Work daemon: each query is answered by `answer`, which gets the call number; the queries it received are listed. `extra` adds routes. */
function workDaemon(answer: (call: number) => Response | Promise<Response>, extra: Record<string, Handler> = {}) {
  let calls = 0;
  const daemon = stubDaemon({ [workPath]: (() => { calls += 1; return answer(calls); }) as Handler, ...extra });
  return { ...daemon, queries: () => daemon.requests.filter(request => request.path.startsWith(workPath)).map(request => request.path) };
}

function renderWork(entry = "/work") {
  return renderRoutes(entry);
}

/** Waits until the page has its answer: loaded rows, an empty state, or an error. */
const answered = () => waitFor(() => expect(document.querySelector(".loaded-state, .empty-state, .error-panel")).not.toBeNull());
/** The golden Work view with its rows in one named repository group, in the Runs and Watching sections. */
function repoGroupBody(): WorkBody {
  const body = workGolden();
  const rows = body.mainGroups.flatMap(group => group.rows as unknown[]);
  body.mainGroups = [{ dimension: "repo", value: "acme/web", title: `acme/web · ${rows.length}`, label: "acme/web", count: rows.length, rows }];
  body.watchingGroups = [];
  return body;
}

const headingNames = () => screen.getAllByRole("heading", { level: 2 }).map(heading => heading.textContent);
const fold = (label: string) => screen.getByRole("button", { name: new RegExp(`^.*${label}`) });
/** Clicks the acme/web group heading, folding it. */
const clickFold = () => userEvent.setup().click(fold("acme/web"));
/** Makes every read and write of browser storage throw, as a blocked storage does. */
function throwStorage() {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("storage blocked"); });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("storage blocked"); });
}
const search = () => screen.getByRole("searchbox", { name: "Search issues, runs and belts" }) as HTMLInputElement;

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  cleanup();
});

describe("Work page", () => {
  it("renders the daemon's sections in order: Runs, Waiting to start, Shadow, then Watching", async () => {
    workDaemon(() => json(goldenText("views-work.json")));
    renderWork();
    await answered();
    const names = headingNames();
    expect(names[0]).toBe("Runs");
    expect(names[1]).toBe("Waiting to start");
    expect(names[2]).toMatch(/^Shadow · \d+$/);
    expect(names[3]).toMatch(/^Watching · \d+$/);
  });

  it("marks each run on a shadow version with the daemon's chip", async () => {
    const body = workGolden();
    workDaemon(() => json(JSON.stringify(body)));
    renderWork();
    await answered();
    const shadow = (body.shadowGroups as Array<{ rows: Array<{ run: { id: string }; shadow?: string }> }>).flatMap(group => group.rows);
    expect(shadow.length).toBeGreaterThan(0);
    for (const row of shadow) {
      expect(document.querySelector(`tr[data-run-id='${row.run.id}'] .work-shadow-chip`)?.textContent).toBe(row.shadow);
    }
  });

  it("keeps the shadow section, its chips and each run's link to its run page with shadow tests off", async () => {
    const body = workGolden();
    const shadow = (body.shadowGroups as Array<{ rows: Array<{ run: { id: string }; shadow?: string }> }>).flatMap(group => group.rows);
    workDaemon(() => json(JSON.stringify(body)), {
      "/api/v1/settings": () => json({ ...JSON.parse(goldenText("settings.json")), enableShadowTests: false })
    });
    renderWork();
    await answered();
    expect(headingNames().some(name => name?.startsWith("Shadow · "))).toBe(true);
    expect(shadow.length).toBeGreaterThan(0);
    for (const row of shadow) {
      const line = document.querySelector(`tr[data-run-id='${row.run.id}']`);
      expect(line?.querySelector(".work-shadow-chip")?.textContent).toBe(row.shadow);
      expect(line?.querySelector("a")?.getAttribute("href")).toBe(`/runs/${encodeURIComponent(row.run.id)}`);
    }
  });

  it("requests the query the URL names, and sends the matching query when a control changes, keeping the other selections", async () => {
    const user = userEvent.setup();
    const daemon = workDaemon(() => json(goldenText("views-work.json")));
    renderWork("/work?filter=you&belt=issues&search=pay&groupBy=epic");
    await answered();
    expect(daemon.queries()[0]).toBe(`${workPath}?filter=you&belt=issues&search=pay&groupBy=epic`);
    await user.click(screen.getByRole("button", { name: /^All$/ }));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${workPath}?filter=all&belt=issues&search=pay&groupBy=epic`));
    await user.selectOptions(screen.getByLabelText("Group by"), "status");
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${workPath}?filter=all&belt=issues&search=pay&groupBy=status`));
  });

  it("shows each control at its default when the URL names none", async () => {
    workDaemon(() => json(goldenText("views-work.json")));
    renderWork("/work");
    await answered();
    expect((screen.getByLabelText("Belt") as HTMLSelectElement).value).toBe("all");
    expect((screen.getByLabelText("Group by") as HTMLSelectElement).value).toBe("none");
    expect(screen.getByRole("button", { name: "Active" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("searches as you type, and Escape restores the unsearched query", async () => {
    const user = userEvent.setup();
    const daemon = workDaemon(() => json(goldenText("views-work.json")));
    renderWork("/work?groupBy=repo");
    await answered();
    await user.type(search(), "pay");
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${workPath}?search=pay&groupBy=repo`));
    await user.type(search(), "{Escape}");
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(`${workPath}?groupBy=repo`));
    expect(search().value).toBe("");
  });

  it("requests the daemon's stage and clears it back to every stage", async () => {
    const user = userEvent.setup();
    const body = workGolden();
    body.stage = { id: "review", name: "Review" };
    const daemon = workDaemon(() => json(JSON.stringify(body)));
    renderWork("/work?stage=review");
    expect(await screen.findByRole("heading", { level: 1, name: "Review" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Clear stage" }));
    await waitFor(() => expect(daemon.queries().at(-1)).toBe(workPath));
  });

  it("keeps the daemon's group order and labels, nonalphabetical, with no duplicate rows", async () => {
    const body = workGolden();
    const rowOf = (group: Record<string, unknown>) => (group.rows as Array<Record<string, unknown>>)[0]!;
    body.mainGroups = [
      { dimension: "repo", value: "zeta/api", title: "zeta/api · 1", label: "zeta", count: 1, rows: [{ ...rowOf(body.mainGroups[0]!), reference: "#1" }] },
      { dimension: "repo", value: "alpha/web", title: "alpha/web · 1", label: "alpha", count: 1, rows: [{ ...rowOf(body.mainGroups[0]!), reference: "#2" }] }
    ];
    body.watchingGroups = [];
    body.shadowGroups = [];
    workDaemon(() => json(JSON.stringify(body)));
    renderWork("/work?groupBy=repo");
    await answered();
    const headers = screen.getAllByRole("button", { expanded: true }).map(button => button.textContent?.replace(/\s+/g, " ").trim());
    expect(headers).toEqual(["zeta · 1", "alpha · 1"]);
    expect(document.querySelectorAll("tr.work-row")).toHaveLength(2);
  });

  it("folds a group in Runs without a request, and keeps the fold after a remount", async () => {
    const user = userEvent.setup();
    const daemon = workDaemon(() => json(JSON.stringify(repoGroupBody())));
    const first = renderWork("/work?groupBy=repo");
    await answered();
    const header = fold("acme/web");
    const before = daemon.queries().length;
    await user.click(header);
    expect(header.getAttribute("aria-expanded")).toBe("false");
    expect(header.textContent).toMatch(/acme\/web · 15/);
    expect(daemon.queries()).toHaveLength(before);
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).runsFolded.repo).toEqual(["acme/web"]);
    first.unmount();
    renderWork("/work?groupBy=repo");
    await answered();
    expect(fold("acme/web").getAttribute("aria-expanded")).toBe("false");
  });

  it("uses default expanded groups when stored preferences are corrupt, and stays usable when browser storage throws", async () => {
    const user = userEvent.setup();
    localStorage.setItem(preferencesStorageKey, "{not json");
    workDaemon(() => json(JSON.stringify(repoGroupBody())));
    const first = renderWork("/work?groupBy=repo");
    await answered();
    expect(fold("acme/web").getAttribute("aria-expanded")).toBe("true");
    first.unmount();
    localStorage.clear();
    throwStorage();
    renderWork("/work?groupBy=repo");
    await answered();
    await user.click(fold("acme/web"));
    expect(fold("acme/web").getAttribute("aria-expanded")).toBe("false");
  });

  it("shows a Done run under the daemon's own Done group, apart from Shipped", async () => {
    const body = workGolden();
    const row = (body.mainGroups[0]!.rows as Array<Record<string, unknown>>)[0]!;
    const done = { ...row, reference: "#12", needsYou: false, pausedByYou: false, location: { glyph: "✓", look: "done", text: "Done · #12" }, run: { ...(row.run as object), state: "done" } };
    body.mainGroups = [{ dimension: "status", value: "Done", title: "Done · 1", label: "Done", count: 1, rows: [done] }];
    body.watchingGroups = [];
    workDaemon(() => json(JSON.stringify(body)));
    renderWork("/work?groupBy=status");
    await answered();
    expect(screen.getAllByRole("button", { expanded: true }).map(button => button.textContent?.replace(/\s+/g, " ").trim())).toContain("Done · 1");
    expect(screen.getByText("Done · #12")).toBeTruthy();
  });

  it("shows the daemon's empty copy for a Work view with no rows", async () => {
    workDaemon(() => json(goldenText("views-work-empty.json")));
    renderWork();
    expect(await screen.findByText("No runs yet. Label an issue on a watched repository, then check GitHub.")).toBeTruthy();
    expect(screen.queryByRole("heading", { level: 2, name: /^Watching/ })).toBeNull();
  });

  it("shows the daemon's rejection of an unsupported groupBy instead of a fallback", async () => {
    const daemon = workDaemon(() => json(JSON.stringify({ message: "groupBy must be one of none, repo, status, belt, epic", problems: [] }), 422));
    renderWork("/work?groupBy=owner");
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByText("groupBy must be one of none, repo, status, belt, epic")).toBeTruthy();
    expect(screen.queryByRole("table", { name: "Runs" })).toBeNull();
    expect(daemon.queries()).toEqual([`${workPath}?groupBy=owner`]);
  });

  it("links a row that needs you to its decision stage, and another row to its run, by encoded id", async () => {
    const body = workGolden();
    const group = body.mainGroups[0]!;
    const rows = group.rows as Array<Record<string, unknown>>;
    const decision = rows.find(row => row.openStage) ?? rows[0]!;
    const passive = { ...rows[0]!, openStage: undefined, needsYou: false, chip: undefined, run: { ...(rows[0]!.run as object), id: "nightly:scheduled/2026 #1" } };
    body.mainGroups = [{ ...group, rows: [decision, passive] }];
    body.watchingGroups = [];
    workDaemon(() => json(JSON.stringify(body)));
    renderWork();
    await answered();
    const decisionId = (decision.run as { id: string }).id;
    const decisionLink = document.querySelector<HTMLAnchorElement>(`tr[data-run-id="${decisionId}"] a`)!;
    expect(decisionLink.getAttribute("href")).toBe(`/runs/${encodeURIComponent(decisionId)}?stage=${encodeURIComponent(decision.openStage as string)}`);
    const passiveLink = document.querySelector<HTMLAnchorElement>(`tr[data-run-id="nightly:scheduled/2026 #1"] a`)!;
    expect(passiveLink.getAttribute("href")).toBe(`/runs/${encodeURIComponent("nightly:scheduled/2026 #1")}`);
  });

  it("shows a read failure instead of an empty table when the first load fails", async () => {
    workDaemon(() => json(goldenText("error-not-found.json"), 500));
    renderWork();
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.queryByRole("table", { name: "Runs" })).toBeNull();
  });

  it("adds one history entry for a search, replaces it for each further keystroke, and adds one for a filter", async () => {
    const user = userEvent.setup();
    workDaemon(() => json(goldenText("views-work.json")));
    window.history.replaceState({}, "", "/work");
    render(<AppProviders><BrowserRouter><AppRoutes /></BrowserRouter></AppProviders>);
    await answered();
    const before = window.history.length;
    await user.type(search(), "abc");
    expect(window.history.length - before).toBe(1);
    await user.click(screen.getByRole("button", { name: /^All$/ }));
    expect(window.history.length - before).toBe(2);
    const params = new URLSearchParams(window.location.search);
    expect(params.get("filter")).toBe("all");
    expect(params.get("search")).toBe("abc");
  });

  it("returns to the Work URL and its folds on Back from a run", async () => {
    const user = userEvent.setup();
    window.history.replaceState({}, "", "/work?groupBy=repo");
    const body = repoGroupBody();
    const [first] = body.mainGroups[0]!.rows as unknown[];
    body.mainGroups.push({ dimension: "repo", value: "acme/api", title: "acme/api · 1", label: "acme/api", count: 1, rows: [first] });
    const daemon = workDaemon(() => json(JSON.stringify(body)));
    render(<AppProviders><BrowserRouter><AppRoutes /></BrowserRouter></AppProviders>);
    await answered();
    await user.click(fold("acme/web"));
    await user.click(document.querySelector<HTMLAnchorElement>("tr.work-row a")!);
    await waitFor(() => expect(window.location.pathname.startsWith("/runs/")).toBe(true));
    window.history.back();
    await waitFor(() => expect(window.location.pathname).toBe("/work"));
    expect(window.location.search).toBe("?groupBy=repo");
    await answered();
    expect(fold("acme/web").getAttribute("aria-expanded")).toBe("false");
    expect(daemon.queries().at(-1)).toBe(`${workPath}?groupBy=repo`);
    window.history.forward();
    await waitFor(() => expect(window.location.pathname.startsWith("/runs/")).toBe(true));
  });

  it("renders only the current query when an older response arrives last", async () => {
    const user = userEvent.setup();
    const current = workGolden();
    current.caption = "current caption";
    const obsolete = workGolden();
    obsolete.caption = "obsolete caption";
    let releaseObsolete!: (response: Response) => void;
    const daemon = workDaemon(call => {
      if (call === 1) return json(goldenText("views-work.json"));
      if (call === 2) return new Promise<Response>(resolve => { releaseObsolete = resolve; });
      return json(JSON.stringify(current));
    });
    renderWork();
    await answered();
    await user.click(screen.getByRole("button", { name: /^Needs you/ }));
    await waitFor(() => expect(daemon.queries()).toHaveLength(2));
    await user.click(screen.getByRole("button", { name: /^All$/ }));
    await waitFor(() => expect(screen.getByText("current caption")).toBeTruthy());
    await act(async () => {
      releaseObsolete(json(JSON.stringify(obsolete)));
      await new Promise(resolve => setTimeout(resolve, 20));
    });
    expect(screen.getByText("current caption")).toBeTruthy();
    expect(screen.queryByText("obsolete caption")).toBeNull();
  });

  it("keeps the last rows and a stale warning when a refresh of the same query fails", async () => {
    const daemon = workDaemon(call => call === 1 ? json(JSON.stringify(repoGroupBody())) : json(goldenText("error-not-found.json"), 500));
    renderWork("/work?groupBy=repo&search=pay");
    await answered();
    AppSocket.instances.at(-1)!.send({ type: "hello", apiVersion: 1 });
    AppSocket.instances.at(-1)!.send({ type: "changed", resource: "runs", revision: 1 });
    expect(await screen.findByText("Showing the last loaded results; refresh failed.")).toBeTruthy();
    expect(daemon.queries().length).toBeGreaterThanOrEqual(2);
    expect(document.querySelectorAll("tr.work-row").length).toBeGreaterThan(0);
  });

  it("keeps watcher-only matches, and waiting-only results with no run links, with the daemon's empty copy", async () => {
    const watcherOnly = workGolden();
    watcherOnly.mainGroups = [];
    watcherOnly.waiting = [];
    watcherOnly.mainEmptyMessage = "No regular runs match these filters.";
    workDaemon(() => json(JSON.stringify(watcherOnly)));
    const first = renderWork("/work?filter=you");
    expect(await screen.findByText("No regular runs match these filters.")).toBeTruthy();
    expect(within(screen.getByRole("table", { name: "Watching" })).getAllByRole("row").length).toBeGreaterThan(1);
    first.unmount();

    const waitingOnly = workGolden();
    waitingOnly.mainGroups = [];
    waitingOnly.watchingGroups = [];
    waitingOnly.mainEmptyMessage = "No runs match these filters.";
    workDaemon(() => json(JSON.stringify(waitingOnly)));
    renderWork("/work?search=zzz");
    expect(await screen.findByText("No runs match these filters.")).toBeTruthy();
    const waiting = screen.getByRole("list", { name: "Waiting to start" });
    expect(within(waiting).getAllByRole("listitem")).toHaveLength(2);
    expect(within(waiting).queryAllByRole("link")).toHaveLength(0);
  });

  it("gives attention rows their styling and leaves passive watches unmarked", async () => {
    const body = repoGroupBody();
    const rows = body.mainGroups[0]!.rows as Array<Record<string, unknown>>;
    body.mainGroups[0]!.rows = [
      { ...rows[0]!, needsYou: true, chip: "Answer" },
      { ...rows[1]!, needsYou: true, chip: "Decide" },
      { ...rows[2]!, needsYou: false, chip: undefined }
    ];
    workDaemon(() => json(JSON.stringify(body)));
    renderWork("/work?groupBy=repo");
    await answered();
    const rowOf = (index: number) => document.querySelector(`tr[data-run-id="${(rows[index]!.run as { id: string }).id}"]`)!;
    expect(rowOf(0).classList.contains("work-row-held")).toBe(true);
    expect(rowOf(1).classList.contains("work-row-needs")).toBe(true);
    expect(rowOf(2).classList.contains("work-row-passive")).toBe(true);
    expect(rowOf(2).querySelector(".work-chip")).toBeNull();
  });

  it("keeps the navigation width and other preferences when a fold changes", async () => {
    localStorage.setItem(preferencesStorageKey, JSON.stringify({ ...defaultPreferences, navigationWidth: 300, sessionsOpen: true }));
    workDaemon(() => json(JSON.stringify(repoGroupBody())));
    renderWork("/work?groupBy=repo");
    await answered();
    await clickFold();
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!)).toMatchObject({ navigationWidth: 300, sessionsOpen: true, runsFolded: { repo: ["acme/web"] } });
  });
});
