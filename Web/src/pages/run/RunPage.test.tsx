import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AppProviders, AppRoutes } from "../../app/App";
import type { AttemptOutputResponse, AttemptOutputSection, MarkdownBlock, RunViewResponse } from "../../api/wire.gen";
import { runDraftStore } from "../../state/runDrafts";
import { defaultPreferences, preferencesStorageKey } from "../../state/preferences";
import { outputFoldsStorageKey } from "../../state/outputFolds";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";
import { fixtureRoute, refusal, stubDaemon as daemonRoutes, type Handler } from "../../test/daemon";
import { OutputSections } from "./OutputSections";

const runId = "epics:jira:PAY-40";
const viewPath = `/api/v1/views/run/${encodeURIComponent(runId)}`;
const runPath = `/api/v1/runs/${encodeURIComponent(runId)}`;
const title = /Split invoicing into its own service$/;
const viewOf = (): RunViewResponse => JSON.parse(goldenText("views-run.json")) as RunViewResponse;

/** The daemon's Sessions read for a run with no sessions, unless a test supplies its own. */
const emptySessions = {
  runId,
  rows: [],
  selection: { stageId: "plan-epic" },
  emptyMessage: "No sessions yet. The first one starts when Plan the epic picks this issue up.",
  ghostty: { enabled: false, reason: "This run has no session to open in Ghostty." }
};

/** The run page's daemon: the run view, sessions, outputs and commands. */
function stubDaemon(routes: Record<string, Handler | object> = {}) {
  const view = routes[viewPath] ?? fixtureRoute("views-run.json");
  return daemonRoutes({
    [viewPath]: view as Handler,
    [`${viewPath}/sessions*`]: () => json(emptySessions),
    [`${runPath}/attempts*`]: request => request.path.includes("/output") ? json(goldenText("run-attempt-output.json"))
      : json(goldenText("run-attempt-artifacts.json")),
    "POST /api/v1/artifacts/link": fixtureRoute("artifact-link.json"),
    "POST /api/v1/desktop/vscode": fixtureRoute("desktop-ok.json"),
    [`POST ${runPath}/answer`]: fixtureRoute("run-answer.json"),
    [`POST ${runPath}/resume`]: fixtureRoute("run-resume.json"),
    ...routes
  });
}

function renderRun(search = "") {
  return render(<AppProviders><MemoryRouter initialEntries={[`/runs/${encodeURIComponent(runId)}${search}`]}><AppRoutes /></MemoryRouter></AppProviders>);
}

/** Measured bounds for the workspace and the main column, and a ResizeObserver a test can fire by hand. */
function measureLayout(bounds: { workspace: number }) {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const width = this.classList.contains("run-workspace") ? bounds.workspace : 800;
    return { x: 0, y: 0, top: 0, left: 0, right: width, bottom: 0, width, height: 0, toJSON: () => ({}) } as DOMRect;
  });
  const observers: Array<() => void> = [];
  class FakeResizeObserver {
    constructor(callback: () => void) { observers.push(callback); }
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  return { fireResize: () => observers.forEach(callback => callback()) };
}

const sidebarWidth = () => document.querySelector<HTMLElement>('[data-column="sessions"]')!.style.width;
const heading = () => screen.findByRole("heading", { level: 1, name: title });

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  runDraftStore.clearAll();
});

describe("Run page", () => {
  it("keeps the advanced stage when the run is reopened", async () => {
    const user = userEvent.setup();
    const current = viewOf();
    const old = { ...current, run: { ...current.run, stageId: "plan-epic" } };
    let reads = 0;
    stubDaemon({ [viewPath]: () => { reads += 1; return json(reads === 1 ? old : current); } });
    function Navigation() {
      const navigate = useNavigate();
      return <>
        <button onClick={() => navigate("/work")}>Leave run</button>
        <button onClick={() => navigate(`/runs/${encodeURIComponent(runId)}`)}>Reopen run</button>
      </>;
    }
    render(<AppProviders><MemoryRouter initialEntries={[`/runs/${encodeURIComponent(runId)}`]}><Navigation /><AppRoutes /></MemoryRouter></AppProviders>);
    await screen.findByRole("region", { name: "Stage 1: Plan the epic" });
    await user.click(screen.getByRole("button", { name: "Leave run" }));
    await user.click(screen.getByRole("button", { name: "Reopen run" }));
    await screen.findByRole("region", { name: "Stage 5: Watch the ticket" });
    expect(screen.queryByRole("region", { name: "Stage 1: Plan the epic" })).toBeNull();
  });

  it("does not keep the previous run on screen when navigating to another", async () => {
    const user = userEvent.setup();
    stubDaemon({ "/api/v1/views/run/another-run*": () => new Promise<Response>(() => {}) });
    function SwitchRun() {
      const navigate = useNavigate();
      return <button onClick={() => navigate("/runs/another-run")}>Open another run</button>;
    }
    render(<AppProviders><MemoryRouter initialEntries={[`/runs/${encodeURIComponent(runId)}`]}><SwitchRun /><AppRoutes /></MemoryRouter></AppProviders>);
    const headingNode = await heading();
    await user.click(screen.getByRole("button", { name: "Open another run" }));
    expect(await screen.findByText("Loading run…")).toBeTruthy();
    expect(headingNode.isConnected).toBe(false);
  });

  it("renders the header, stage line, selected panel, output and artifacts from the daemon, and signs a fresh link on every Open", async () => {
    const user = userEvent.setup();
    const daemon = stubDaemon();
    const opened = { location: { href: "" }, opener: "unset" as unknown, close: vi.fn() };
    const openSpy = vi.spyOn(window, "open").mockReturnValue(opened as unknown as Window);
    const { container } = renderRun("?stage=plan-epic&attempt=plan-epic-1");
    await heading();
    expect(screen.getByText("Belt v1 · 11 stages")).toBeTruthy();
    expect(screen.getByText("COST $0.11 · partial")).toBeTruthy();
    expect(container.querySelectorAll(".stage-box").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: /^Plan the epic/ }).getAttribute("data-stage")).toBe("plan-epic");
    const panel = await screen.findByRole("region", { name: "Stage 1: Plan the epic" });
    expect(within(panel).getByText("Contract stage output")).toBeTruthy();
    expect(within(panel).getByText("contract.html")).toBeTruthy();
    const openButtons = await within(panel).findAllByRole("button", { name: "Open in browser" });
    await user.click(openButtons[0]!);
    await waitFor(() => expect(daemon.posts("/api/v1/artifacts/link")).toHaveLength(1));
    await user.click(openButtons[0]!);
    await waitFor(() => expect(daemon.posts("/api/v1/artifacts/link")).toHaveLength(2));
    expect(daemon.posts("/api/v1/artifacts/link")[0]!.body).toEqual({ runId, attemptId: "plan-epic-1", name: "contract.html" });
    expect(openSpy).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(opened.location.href).toContain("/artifacts/"));
  });

  it("falls back to the current stage for an invalid selection, and keeps an explicitly selected historical attempt", async () => {
    stubDaemon();
    const first = renderRun("?stage=not-a-stage&attempt=not-an-attempt");
    await heading();
    expect(await screen.findByRole("region", { name: "Stage 5: Watch the ticket" })).toBeTruthy();
    first.unmount();

    const view = viewOf();
    const panel = view.stages.find(stage => stage.id === "plan-epic")!;
    panel.attempts = [
      { ...panel.attempts[0]!, id: "plan-epic-2", number: 2, caption: "Attempt 2 of 2" },
      { ...panel.attempts[0]!, id: "plan-epic-1", number: 1, caption: "Attempt 1 of 2" }
    ];
    panel.defaultAttempt = "plan-epic-2";
    const daemon = stubDaemon({ [viewPath]: () => json(view) });
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panelRegion = await screen.findByRole("region", { name: "Stage 1: Plan the epic" });
    expect((within(panelRegion).getByLabelText("Select attempt of Plan the epic") as HTMLSelectElement).value).toBe("plan-epic-1");
    await waitFor(() => expect(daemon.requests.some(request => request.path.includes("/attempts/plan-epic-1/output"))).toBe(true));
    expect(daemon.requests.some(request => request.path.includes("/attempts/plan-epic-2/output"))).toBe(false);
  });

  it("sends the daemon's worktree id to the VS Code desktop action", async () => {
    const user = userEvent.setup();
    const daemon = stubDaemon();
    renderRun();
    await heading();
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    await user.click(await screen.findByRole("button", { name: /Open in VS Code/ }));
    await waitFor(() => expect(daemon.posts("/api/v1/desktop/vscode")).toHaveLength(1));
    expect(daemon.posts("/api/v1/desktop/vscode")[0]!.body).toEqual({ runId, targetId: "main" });
  });

  it("shows a Done run with the daemon's Done caption, and no AFK control", async () => {
    const done = viewOf();
    done.run = { ...done.run, state: "done" } as RunViewResponse["run"];
    done.active = false;
    done.boxes = { ...done.boxes, caption: "Done" };
    stubDaemon({ [viewPath]: () => json(JSON.stringify(done)) });
    const { container } = renderRun();
    await heading();
    await waitFor(() => expect(container.querySelector(".run-line-header")?.textContent).toContain("Done"));
    expect(container.querySelector(".run-line-header")?.textContent).not.toContain("Now at");
    expect(screen.queryByRole("button", { name: "Run AFK mode" })).toBeNull();
  });

  it("shows only the shortcut buttons the daemon lists, and the daemon's live watch notes in the selected watch panel", async () => {
    const view = viewOf();
    view.quickActions = ["start-now"];
    view.watchNotes = [{ id: "acme/web#1", reference: "acme/web#1", note: "checks are failing" }];
    stubDaemon({ [viewPath]: () => json(view) });
    renderRun();
    expect(await screen.findByRole("button", { name: "Start now" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(within(await screen.findByRole("region", { name: "Stage 5: Watch the ticket" })).getByText("checks are failing")).toBeTruthy();
  });

  it("shows the daemon's 422 problems beside the answer and keeps the entered answer", async () => {
    const user = userEvent.setup();
    const view = viewOf() as RunViewResponse & { actions: unknown[] };
    view.run.state = "held";
    view.actions.push({ id: "answer", enabled: true, stage: "watch", attempt: "watch-1" });
    stubDaemon({ [viewPath]: () => json(view), [`POST ${runPath}/answer`]: refusal("Answer refused", ["answer is required", "stage changed"]) });
    renderRun();
    const textarea = await screen.findByLabelText("Answer Watch the ticket");
    await user.type(textarea, "  keep this text  ");
    await user.click(screen.getByRole("button", { name: "Send answer" }));
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Answer refused")).toBeTruthy();
    expect(within(alert).getAllByRole("listitem").map(item => item.textContent)).toEqual(["answer is required", "stage changed"]);
    expect((textarea as HTMLTextAreaElement).value).toBe("  keep this text  ");
  });

  it("holds the run's controls first, then the open links after a divider, in one menu that dismisses on Escape and an outside click", async () => {
    const user = userEvent.setup();
    stubDaemon();
    renderRun();
    await heading();
    const trigger = screen.getByRole("button", { name: "Run actions" });
    await user.click(trigger);
    const menu = screen.getByLabelText("Run actions menu");
    const rows = within(menu).getAllByRole("button").map(row => row.textContent);
    expect(rows.indexOf("Drop…")).toBeLessThan(rows.indexOf("Open in VS Code"));
    const link = within(menu).getByRole("link", { name: /Open Ticket PAY-40/ });
    expect(link.previousElementSibling?.classList.contains("run-menu-divider")).toBe(true);
    await user.keyboard("{Escape}");
    expect(screen.queryByLabelText("Run actions menu")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Run actions" }));
    await user.click(trigger);
    await user.click(document.querySelector(".run-menu-scrim")!);
    expect(screen.queryByLabelText("Run actions menu")).toBeNull();
    expect(screen.getByRole("region", { name: "Stage 5: Watch the ticket" })).toBeTruthy();
  });

  it("keeps the Send to sheet open with every problem when the daemon refuses, and closes it only once the daemon accepts", async () => {
    const user = userEvent.setup();
    const view = viewOf();
    view.run.state = "held";
    view.actions = view.actions.map(action => action.id === "resume" ? { ...action, enabled: true, reason: undefined } : action);
    const daemon = stubDaemon({ [viewPath]: () => json(view), [`POST ${runPath}/resume`]: refusal("Continue refused", ["holds are not answerable", "stage changed"]) });
    renderRun();
    await heading();
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    await user.click(screen.getByRole("button", { name: "Send to…" }));
    await user.type(await screen.findByLabelText("Message (optional)"), "please carry on");
    await user.click(screen.getByRole("button", { name: "Send" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(within(dialog).getByText("Continue refused")).toBeTruthy());
    expect(within(dialog).getAllByRole("listitem").map(item => item.textContent)).toEqual(["holds are not answerable", "stage changed"]);
    expect((within(dialog).getByLabelText("Message (optional)") as HTMLTextAreaElement).value).toBe("please carry on");
    expect(daemon.posts().some(request => request.path.endsWith("/resume"))).toBe(true);
  });
});

describe("Run page Sessions sidebar", () => {
  it("toggles open and hidden, keeping the selection, and reopens on the same daemon selection", async () => {
    const user = userEvent.setup();
    const daemon = stubDaemon();
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const toggle = await screen.findByRole("button", { name: "Sessions" });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    await user.click(toggle);
    await waitFor(() => expect(document.querySelector('[data-column="sessions"][data-ready="true"]')).not.toBeNull());
    expect(daemon.requests.some(request => request.path.startsWith(`${viewPath}/sessions?`) && request.path.includes("stage=plan-epic") && request.path.includes("attempt=plan-epic-1"))).toBe(true);
    await user.click(screen.getByRole("button", { name: "Hide ›" }));
    expect(document.querySelector('[data-column="sessions"]')).toBeNull();
    expect(screen.getByRole("region", { name: "Stage 1: Plan the epic" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Sessions" }));
    await waitFor(() => expect(document.querySelector('[data-column="sessions"][data-ready="true"]')).not.toBeNull());
    expect(daemon.requests.filter(request => request.path.startsWith(`${viewPath}/sessions?`)).every(request => request.path.includes("attempt=plan-epic-1"))).toBe(true);
  });

  it("selects a session's stage and attempt through the same URL selection the stage line uses", async () => {
    const user = userEvent.setup();
    const sessions = {
      runId,
      rows: [
        { attemptId: "plan-epic-1", stageId: "plan-epic", title: "01 Plan the epic · attempt 1", caption: "2m 0s · cost not tracked", status: "done", session: "bl-demo-PAY-40-plan-epic", selected: true },
        { attemptId: "open-branches-1", stageId: "open-branches", title: "03 Open the feature branch · attempt 1", caption: "2m 0s · $0.11", status: "done", session: "bl-demo-PAY-40-open-branches", selected: false }
      ],
      selection: { stageId: "plan-epic", attemptId: "plan-epic-1" },
      ghostty: { enabled: false, reason: "No terminal is running for this session." },
      log: "No log was saved for this session."
    };
    const daemon = stubDaemon({ [`${viewPath}/sessions*`]: () => json(sessions) });
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    await user.click(await screen.findByRole("button", { name: "Sessions" }));
    await user.click(await screen.findByRole("button", { name: /03 Open the feature branch/ }));
    await waitFor(() => expect(daemon.requests.some(request => request.path.includes("stage=open-branches") && request.path.includes("attempt=open-branches-1"))).toBe(true));
    expect(await screen.findByRole("region", { name: "Stage 3: Open the feature branch" })).toBeTruthy();
  });

  it("restores a chosen width after reload and shrinks it to keep a 420 px main column, without overwriting the choice", async () => {
    localStorage.setItem(preferencesStorageKey, JSON.stringify({ ...defaultPreferences, sessionsOpen: true, sessionsWidth: 800 }));
    const layout = measureLayout({ workspace: 1000 });
    stubDaemon();
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    await waitFor(() => expect(sidebarWidth()).toBe("580px"));
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).sessionsWidth).toBe(800);
    vi.mocked(HTMLElement.prototype.getBoundingClientRect).mockRestore();
    measureLayout({ workspace: 1400 });
    layout.fireResize();
    await waitFor(() => expect(sidebarWidth()).toBe("800px"));
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).sessionsWidth).toBe(800);
  });

  it("drags the divider to save a clamped width, and double-clicking resets to half the workspace", async () => {
    localStorage.setItem(preferencesStorageKey, JSON.stringify({ ...defaultPreferences, sessionsOpen: true }));
    measureLayout({ workspace: 1200 });
    stubDaemon();
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const divider = await screen.findByRole("separator", { name: "Resize Sessions" });
    expect(sidebarWidth()).toBe("600px");
    fireEvent.pointerDown(divider, { clientX: 600, pointerId: 1 });
    fireEvent.pointerMove(divider, { clientX: 500, pointerId: 1 });
    fireEvent.pointerUp(divider, { clientX: 500, pointerId: 1 });
    await waitFor(() => expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).sessionsWidth).toBe(700));
    expect(sidebarWidth()).toBe("700px");
    fireEvent.pointerDown(divider, { clientX: 500, pointerId: 1 });
    fireEvent.pointerMove(divider, { clientX: 0, pointerId: 1 });
    fireEvent.pointerUp(divider, { clientX: 0, pointerId: 1 });
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).sessionsWidth).toBe(900);
    fireEvent.doubleClick(divider);
    await waitFor(() => expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).sessionsWidth).toBeNull());
    expect(sidebarWidth()).toBe("600px");
  });
});

/** The agent output the daemon sends for a section test: a title, a preamble, and sections of every shape. */
const para = (text: string): MarkdownBlock => ({ type: "paragraph", text });
const headingBlock = (level: number, text: string): MarkdownBlock => ({ type: "heading", level, text });
const sectionsOfSpec = (): AttemptOutputSection[] => [
  { id: "preamble", label: "Output", level: 0, blocks: [para("Read the goal first.")] },
  { id: "heading:2:1:Goal", label: "Goal", level: 2, blocks: [para("Split **invoicing** out. See [the docs](https://example.com/docs) and [a bad link](javascript:alert(1)).")] },
  { id: "heading:3:1:Details", label: "Details", level: 3, blocks: [{ type: "item", marker: "•", text: "first detail" }, { type: "item", marker: "1.", text: "second detail" }] },
  { id: "heading:2:1:Testing", label: "Testing", level: 2, blocks: [{ type: "code", text: "const ok = true" }] },
  { id: "heading:2:2:Goal", label: "Goal", level: 2, blocks: [para("A second goal with the same label.")] },
  { id: "heading:2:1:Empty", label: "Empty", level: 2, blocks: [] }
];
const titleOfSpec = headingBlock(1, "Invoicing split");
function outputOf(sections: AttemptOutputSection[], title?: MarkdownBlock): AttemptOutputResponse {
  const flat = [...(title ? [title] : []), ...sections.flatMap(section => section.level > 0 ? [headingBlock(section.level, section.label), ...section.blocks] : section.blocks)];
  return { runId, attemptId: "plan-epic-1", availability: "available", blocks: flat, title, sections };
}

/** The run page's daemon answering the attempt's output with `body`, and its artifacts as usual. */
function stubOutput(body: string | Handler) {
  return stubDaemon({
    [`${runPath}/attempts*`]: request => request.path.includes("/output")
      ? (typeof body === "string" ? new Response(body, { status: 200, headers: { "content-type": "application/json" } }) : body(request))
      : json(goldenText("run-attempt-artifacts.json"))
  });
}

/** The output section controls inside a stage panel, in the order the page shows them. */
function sectionControls(scope: HTMLElement): HTMLButtonElement[] {
  return within(scope).getAllByRole("button").filter((button): button is HTMLButtonElement => button.classList.contains("output-section-toggle"));
}

/** The stage panel once the attempt's output has loaded, so its section controls can be queried. */
async function outputPanel(): Promise<HTMLElement> {
  const panel = await screen.findByRole("region", { name: "Stage 1: Plan the epic" });
  await waitFor(() => expect(panel.querySelector(".run-output")).not.toBeNull());
  return panel;
}
const sectionBody = (control: HTMLElement) => document.getElementById(control.getAttribute("aria-controls")!)!;

describe("Run page agent output sections", () => {
  it("shows the title and every section control in order, with a subsection's control there while its parent is closed", async () => {
    stubOutput(JSON.stringify(outputOf(sectionsOfSpec(), titleOfSpec)));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panel = await outputPanel();
    expect(await within(panel).findByRole("heading", { level: 1, name: "Invoicing split" })).toBeTruthy();
    const controls = sectionControls(panel);
    expect(controls.map(control => control.querySelector(".output-section-label")?.textContent)).toEqual(["Output", "Goal", "Details", "Testing", "Goal", "Empty"]);
    expect(controls.every(control => control.getAttribute("aria-expanded") === "false")).toBe(true);
    expect(within(panel).getByRole("heading", { level: 3, name: "Details" })).toBeTruthy();
    expect(sectionBody(within(panel).getByRole("button", { name: "Details" })).hidden).toBe(true);
  });

  it("opens and closes each section on its own, and keeps focus on the toggle that was pressed", async () => {
    const user = userEvent.setup();
    stubOutput(JSON.stringify(outputOf(sectionsOfSpec(), titleOfSpec)));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panel = await outputPanel();
    const [goal, details] = [within(panel).getAllByRole("button", { name: "Goal" })[0]!, within(panel).getByRole("button", { name: "Details" })];
    await user.click(goal);
    expect(goal.getAttribute("aria-expanded")).toBe("true");
    expect(sectionBody(goal).hidden).toBe(false);
    expect(within(sectionBody(goal)).getByText(/Split/)).toBeTruthy();
    expect(details.getAttribute("aria-expanded")).toBe("false");
    await user.click(details);
    expect(sectionBody(details).hidden).toBe(false);
    expect(within(sectionBody(details)).getByText("first detail")).toBeTruthy();
    await user.click(goal);
    expect(goal.getAttribute("aria-expanded")).toBe("false");
    expect(sectionBody(goal).hidden).toBe(true);
    expect(details.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(goal);
  });

  it("toggles a focused section with Enter and with Space, keeping focus on it", async () => {
    const user = userEvent.setup();
    stubOutput(JSON.stringify(outputOf(sectionsOfSpec(), titleOfSpec)));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panel = await outputPanel();
    const testing = within(panel).getByRole("button", { name: "Testing" });
    testing.focus();
    await user.keyboard("{Enter}");
    expect(testing.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(testing);
    await user.keyboard(" ");
    expect(testing.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(testing);
  });

  it("skips a link in a collapsed body when tabbing, and reaches it once the section is open", async () => {
    const user = userEvent.setup();
    stubOutput(JSON.stringify(outputOf(sectionsOfSpec(), titleOfSpec)));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panel = await outputPanel();
    const goal = within(panel).getAllByRole("button", { name: "Goal" })[0]!;
    const details = within(panel).getByRole("button", { name: "Details" });
    goal.focus();
    await user.tab();
    expect(document.activeElement).toBe(details);
    await user.click(goal);
    expect(goal.getAttribute("aria-expanded")).toBe("true");
    goal.focus();
    await user.tab();
    expect(document.activeElement).toBe(within(panel).getByRole("link", { name: "the docs" }));
  });

  it("renders the expanded body's paragraphs, lists, code and only safe links, and folds neither the result nor the artifacts", async () => {
    const user = userEvent.setup();
    stubOutput(JSON.stringify(outputOf(sectionsOfSpec(), titleOfSpec)));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panel = await outputPanel();
    await user.click(within(panel).getAllByRole("button", { name: "Goal" })[0]!);
    const body = sectionBody(within(panel).getAllByRole("button", { name: "Goal" })[0]!);
    expect(body.querySelector("strong")?.textContent).toBe("invoicing");
    expect(within(body).getByRole("link", { name: "the docs" }).getAttribute("href")).toBe("https://example.com/docs");
    expect(within(body).queryByRole("link", { name: "a bad link" })).toBeNull();
    expect(within(body).getByText("a bad link").tagName).toBe("SPAN");
    await user.click(within(panel).getByRole("button", { name: "Testing" }));
    expect(within(sectionBody(within(panel).getByRole("button", { name: "Testing" }))).getByText("const ok = true").tagName).toBe("CODE");
    await user.click(within(panel).getByRole("button", { name: "Details" }));
    const details = sectionBody(within(panel).getByRole("button", { name: "Details" }));
    expect(Array.from(details.querySelectorAll(".markdown-marker")).map(marker => marker.textContent)).toEqual(["•", "1."]);
    expect(within(details).getByText("second detail")).toBeTruthy();
    expect(within(panel).getByRole("button", { name: "Open in browser" })).toBeTruthy();
    expect(within(panel).getByRole("button", { name: "Show in Finder" })).toBeTruthy();
    expect(within(panel).getByText("contract.html")).toBeTruthy();
    await user.click(within(panel).getByRole("button", { name: "Testing" }));
    expect(within(panel).getByRole("button", { name: "Open in browser" })).toBeTruthy();
  });

  it("expands the long-output golden's section with its paragraph, list and code blocks in order, beside the title", async () => {
    const user = userEvent.setup();
    stubOutput(goldenText("run-attempt-output-long.json"));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panel = await outputPanel();
    expect(within(panel).getByRole("heading", { level: 1, name: "Release notes" })).toBeTruthy();
    const control = within(panel).getByRole("button", { name: "Output" });
    await user.click(control);
    const body = sectionBody(control);
    expect(Array.from(body.querySelectorAll(".markdown-blocks > *")).map(node => node.tagName)).toEqual(["P", "DIV", "DIV", "DIV", "PRE", "PRE"]);
    expect(within(body).getByText(/reflow around the output column/)).toBeTruthy();
    expect(Array.from(body.querySelectorAll(".markdown-item-text")).map(item => item.textContent)).toEqual(["first item", "second item with a wrapped\ncontinuation that remains part of the same item", "numbered item"]);
    expect(Array.from(body.querySelectorAll("pre code")).map(code => code.textContent)).toEqual(['let greeting = "hello  "\nprint(greeting)', "unclosed code keeps trailing spaces"]);
  });

  it("gives an unheaded output one Output section with its content, and no controls for empty or title-only output", async () => {
    stubOutput(JSON.stringify(outputOf([{ id: "preamble", label: "Output", level: 0, blocks: [para("Plain text only.")] }])));
    const first = renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const panel = await outputPanel();
    const control = within(panel).getByRole("button", { name: "Output" });
    expect(control.getAttribute("aria-expanded")).toBe("false");
    expect(sectionBody(control).hidden).toBe(true);
    expect(within(sectionBody(control)).getByText("Plain text only.")).toBeTruthy();
    first.unmount();

    stubOutput(JSON.stringify(outputOf([], headingBlock(1, "Only a title"))));
    const second = renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const titled = await outputPanel();
    expect(await within(titled).findByRole("heading", { level: 1, name: "Only a title" })).toBeTruthy();
    expect(sectionControls(titled)).toEqual([]);
    second.unmount();

    stubOutput(JSON.stringify(outputOf([])));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const empty = await outputPanel();
    expect(sectionControls(empty)).toEqual([]);
  });

  it("restores a section's choice after the reader leaves and returns, and keeps other attempts' choices apart", async () => {
    const user = userEvent.setup();
    stubOutput(JSON.stringify(outputOf(sectionsOfSpec(), titleOfSpec)));
    const first = renderRun("?stage=plan-epic&attempt=plan-epic-1");
    let panel = await outputPanel();
    await user.click(within(panel).getByRole("button", { name: "Details" }));
    first.unmount();

    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    panel = await outputPanel();
    expect(within(panel).getByRole("button", { name: "Details" }).getAttribute("aria-expanded")).toBe("true");
    expect(within(panel).getAllByRole("button", { name: "Goal" })[0]!.getAttribute("aria-expanded")).toBe("false");
  });

  it("reads a choice for another attempt as closed, and reads the choice made for it back", async () => {
    const user = userEvent.setup();
    const view = viewOf();
    const panelOf = view.stages.find(stage => stage.id === "plan-epic")!;
    panelOf.attempts = [
      { ...panelOf.attempts[0]!, id: "plan-epic-2", number: 2, caption: "Attempt 2 of 2" },
      { ...panelOf.attempts[0]!, id: "plan-epic-1", number: 1, caption: "Attempt 1 of 2" }
    ];
    panelOf.defaultAttempt = "plan-epic-2";
    stubDaemon({
      [viewPath]: () => json(view),
      [`${runPath}/attempts*`]: request => request.path.includes("/output")
        ? json(JSON.stringify({ ...outputOf(sectionsOfSpec(), titleOfSpec), attemptId: request.path.split("/attempts/")[1]!.split("/")[0] }))
        : json(goldenText("run-attempt-artifacts.json"))
    });
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    let panel = await outputPanel();
    await user.click(within(panel).getByRole("button", { name: "Details" }));
    expect(within(panel).getByRole("button", { name: "Details" }).getAttribute("aria-expanded")).toBe("true");
    await user.selectOptions(within(panel).getByLabelText("Select attempt of Plan the epic"), "plan-epic-2");
    await waitFor(() => expect(within(panel).getByRole("button", { name: "Details" }).getAttribute("aria-expanded")).toBe("false"));
    await user.click(within(panel).getByRole("button", { name: "Details" }));
    await user.selectOptions(within(panel).getByLabelText("Select attempt of Plan the epic"), "plan-epic-1");
    await waitFor(() => expect(within(panel).getByRole("button", { name: "Details" }).getAttribute("aria-expanded")).toBe("true"));
    panel = await outputPanel();
    expect(within(panel).getByRole("button", { name: "Details" }).getAttribute("aria-expanded")).toBe("true");
  });
});

describe("Agent output sections around the output", () => {
  it("shows loading, missing and rejected output as before, with no section controls", async () => {
    stubOutput(() => new Promise<Response>(() => {}));
    const loading = renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const loadingPanel = await screen.findByRole("region", { name: "Stage 1: Plan the epic" });
    expect(within(loadingPanel).getByRole("status").textContent).toBe("Loading output…");
    expect(sectionControls(loadingPanel)).toEqual([]);
    loading.unmount();

    stubOutput(() => json({ runId, attemptId: "plan-epic-1", availability: "missing", blocks: [], sections: [], message: "This attempt wrote no plan.md." }));
    const missing = renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const missingPanel = await screen.findByRole("region", { name: "Stage 1: Plan the epic" });
    expect(await within(missingPanel).findByText("This attempt wrote no plan.md.")).toBeTruthy();
    expect(sectionControls(missingPanel)).toEqual([]);
    missing.unmount();

    stubOutput(refusal("Output unavailable", ["the file cannot be read"]));
    renderRun("?stage=plan-epic&attempt=plan-epic-1");
    const rejectedPanel = await screen.findByRole("region", { name: "Stage 1: Plan the epic" });
    const alert = await within(rejectedPanel).findByRole("alert");
    expect(within(alert).getByText("Output unavailable")).toBeTruthy();
    expect(within(alert).getByText("the file cannot be read")).toBeTruthy();
    expect(sectionControls(rejectedPanel)).toEqual([]);
  });

  it("leaves an approval document fully rendered, with no section controls", async () => {
    const view = viewOf();
    const gate = view.stages.find(stage => stage.id === "approve-plan")!;
    view.run.state = "waiting";
    view.run.stageId = gate.id;
    gate.step = "waiting";
    gate.attempts = [{ id: "approve-plan-1", number: 1, caption: "Attempt 1 of 1", content: "decision", isOpen: true }];
    stubDaemon({ [viewPath]: () => json(view) });
    renderRun("?stage=approve-plan&attempt=approve-plan-1");
    const panel = await screen.findByRole("region", { name: "Stage 2: Approve the plan" });
    await waitFor(() => expect(panel.querySelector(".stage-approval-doc .markdown-blocks")).not.toBeNull());
    expect(panel.querySelectorAll(".stage-approval-doc .markdown-blocks > *").length).toBeGreaterThan(0);
    expect(sectionControls(panel)).toEqual([]);
    expect(panel.querySelector(".output-section-toggle")).toBeNull();
  });
});

describe("Agent output section storage and refreshes", () => {
  const renderSections = (props: { runId: string; attemptId: string; sections: AttemptOutputSection[]; title?: MarkdownBlock }) =>
    render(<OutputSections {...props} />);

  it("keeps the same run and attempt's choices across a body-only refresh, and starts a new or renamed heading closed", async () => {
    const user = userEvent.setup();
    const first = renderSections({ runId, attemptId: "a1", sections: sectionsOfSpec() });
    await user.click(screen.getByRole("button", { name: "Testing" }));
    const refreshed = sectionsOfSpec().map(section => section.id === "heading:2:1:Testing" ? { ...section, blocks: [{ type: "code" as const, text: "changed" }] } : section);
    first.rerender(<OutputSections runId={runId} attemptId="a1" sections={refreshed} />);
    expect(screen.getByRole("button", { name: "Testing" }).getAttribute("aria-expanded")).toBe("true");
    const renamed = sectionsOfSpec().map(section => section.id === "heading:2:1:Testing" ? { ...section, id: "heading:2:1:Checks", label: "Checks" } : section);
    first.rerender(<OutputSections runId={runId} attemptId="a1" sections={renamed} />);
    expect(screen.getByRole("button", { name: "Checks" }).getAttribute("aria-expanded")).toBe("false");
    first.unmount();
  });

  it("keeps two runs' choices apart when both have the same attempt ID", async () => {
    const user = userEvent.setup();
    const first = renderSections({ runId: "run-a", attemptId: "same", sections: sectionsOfSpec() });
    await user.click(screen.getByRole("button", { name: "Testing" }));
    first.unmount();
    const other = renderSections({ runId: "run-b", attemptId: "same", sections: sectionsOfSpec() });
    expect(screen.getByRole("button", { name: "Testing" }).getAttribute("aria-expanded")).toBe("false");
    other.unmount();
    renderSections({ runId: "run-a", attemptId: "same", sections: sectionsOfSpec() });
    expect(screen.getByRole("button", { name: "Testing" }).getAttribute("aria-expanded")).toBe("true");
  });

  it("opens each of two repeated labels on its own", async () => {
    const user = userEvent.setup();
    renderSections({ runId, attemptId: "a1", sections: sectionsOfSpec() });
    const [firstGoal, secondGoal] = screen.getAllByRole("button", { name: "Goal" });
    await user.click(secondGoal!);
    expect(secondGoal!.getAttribute("aria-expanded")).toBe("true");
    expect(firstGoal!.getAttribute("aria-expanded")).toBe("false");
    expect(within(sectionBody(secondGoal!)).getByText("A second goal with the same label.")).toBeTruthy();
  });

  it("treats a refused or unreadable storage as no saved choices, and keeps toggles working for the mounted view", async () => {
    const user = userEvent.setup();
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("denied"); });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("denied"); });
    const view = renderSections({ runId, attemptId: "a1", sections: sectionsOfSpec() });
    const testing = screen.getByRole("button", { name: "Testing" });
    expect(testing.getAttribute("aria-expanded")).toBe("false");
    await user.click(testing);
    expect(testing.getAttribute("aria-expanded")).toBe("true");
    view.unmount();
    vi.restoreAllMocks();
  });

  it("ignores malformed saved JSON and replaces it with a valid record on the next toggle", async () => {
    const user = userEvent.setup();
    localStorage.setItem(outputFoldsStorageKey, "{not json");
    renderSections({ runId, attemptId: "a1", sections: sectionsOfSpec() });
    const testing = screen.getByRole("button", { name: "Testing" });
    expect(testing.getAttribute("aria-expanded")).toBe("false");
    await user.click(testing);
    expect(JSON.parse(localStorage.getItem(outputFoldsStorageKey)!)).toEqual({ [runId]: { a1: { "heading:2:1:Testing": true } } });
  });

  it("drops a stored choice that is not a boolean and never reads one for a heading that is gone", () => {
    localStorage.setItem(outputFoldsStorageKey, JSON.stringify({ [runId]: { a1: { "heading:2:1:Testing": "yes", "heading:2:1:Gone": true } } }));
    renderSections({ runId, attemptId: "a1", sections: sectionsOfSpec() });
    expect(screen.getByRole("button", { name: "Testing" }).getAttribute("aria-expanded")).toBe("false");
  });
});
