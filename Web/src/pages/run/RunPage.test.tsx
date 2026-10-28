import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useNavigate } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AppProviders, AppRoutes } from "../../app/App";
import type { RunViewResponse } from "../../api/wire.gen";
import { runDraftStore } from "../../state/runDrafts";
import { defaultPreferences, preferencesStorageKey } from "../../state/preferences";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";
import { fixtureRoute, refusal, stubDaemon as daemonRoutes, type Handler } from "../../test/daemon";

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

  it("uses the main panel for prompt entry and posts the shared text with explicit source consent", async () => {
    const user = userEvent.setup();
    const view = viewOf();
    view.run.state = "waiting";
    view.run.stageId = "approve-plan";
    const panel = view.stages.find(stage => stage.id === "approve-plan")!;
    panel.step = "waiting";
    panel.attempts = [{ id: "approve-plan-1", number: 1, caption: "Attempt 1 of 1", content: "decision", isOpen: true }];
    const approval = panel.facts!.approval!;
    delete approval.blocks;
    approval.outcomes = [
      { id: "refine", name: "refine", label: "Refine then implement", tone: "good" },
      { id: "direct", name: "direct", label: "Implement directly", tone: "good" }
    ];
    approval.primary = "refine";
    approval.prompt = {
      noteKey: "refine", label: "Your prompt", placeholder: "Your task", help: "Write first, then choose.",
      contextLabel: "Source context", context: [{ type: "paragraph", text: "Ticket details for the human" }],
      includeLabel: "Include source context for agents", include: false
    };
    view.actions = approval.outcomes.map(outcome => ({ id: "decide", enabled: true, stage: panel.id, outcome: outcome.name, needsNote: true }));
    const daemon = stubDaemon({
      [viewPath]: () => json(view), [`POST ${runPath}/decide`]: refusal("Decision refused", ["Keep editing"])
    });
    const first = renderRun();
    const prompt = await screen.findByLabelText("Your prompt");
    expect(prompt.closest(".stage-approval-side")).toBeNull();
    expect(screen.queryByText(approval.emptyDocument)).toBeNull();
    expect(screen.getByText("Ticket details for the human")).toBeTruthy();
    await user.type(prompt, "Implement only this task.");
    await user.click(screen.getByRole("checkbox", { name: approval.prompt.includeLabel }));
    first.unmount();
    renderRun();
    expect((await screen.findByLabelText("Your prompt") as HTMLTextAreaElement).value).toBe("Implement only this task.");
    expect((screen.getByRole("checkbox", { name: approval.prompt.includeLabel }) as HTMLInputElement).checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Implement directly" }));
    await screen.findByText("Decision refused");
    expect(daemon.posts(`${runPath}/decide`)[0]!.body).toEqual({
      outcome: "direct", note: "Implement only this task.", includeSubject: true
    });
    expect((screen.getByLabelText("Your prompt") as HTMLTextAreaElement).value).toBe("Implement only this task.");
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
