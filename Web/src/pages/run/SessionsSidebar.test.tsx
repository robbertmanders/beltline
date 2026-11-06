import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AppProviders, AppRoutes } from "../../app/App";
import { defaultPreferences, preferencesStorageKey } from "../../state/preferences";
import { FakeSocket, MockTerminal, resetTerminalDoubles, stubTerminalSockets } from "../../test/terminalDoubles";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";
import { fixtureRoute, stubDaemon as daemonRoutes } from "../../test/daemon";

vi.mock("@xterm/xterm", async () => ({ Terminal: (await import("../../test/terminalDoubles")).MockTerminal }));
vi.mock("@xterm/addon-fit", async () => ({ FitAddon: (await import("../../test/terminalDoubles")).MockFit }));

const runId = "epics:jira:PAY-40";
const viewPath = `/api/v1/views/run/${encodeURIComponent(runId)}`;
const runPath = `/api/v1/runs/${encodeURIComponent(runId)}`;
const liveTerminal = "bl-demo-PAY-40-open-branches";

const rowLive = { attemptId: "open-branches-1", stageId: "open-branches", title: "03 Open the feature branch · attempt 1", caption: "live", status: "live", session: liveTerminal };
const rowDone = { attemptId: "file-wave-1", stageId: "file-wave", title: "04 File the wave · attempt 1", caption: "2m 0s · cost not tracked", status: "done", session: "bl-demo-PAY-40-file-wave" };
const noGhostty = { enabled: false, reason: "No terminal is running for this session." };

/** The daemon's Sessions read for the run, with the chosen attempt selected. */
function sessionsBody(selected: "live" | "done", overrides: Record<string, unknown> = {}) {
  const chosen = selected === "live" ? rowLive : rowDone;
  return {
    runId,
    rows: [rowLive, rowDone].map(row => ({ ...row, selected: row.attemptId === chosen.attemptId })),
    selection: { stageId: chosen.stageId, attemptId: chosen.attemptId },
    ghostty: noGhostty,
    ...overrides
  };
}

const liveWithTerminal = (overrides: Record<string, unknown> = {}) => sessionsBody("live", {
  terminal: { target: liveTerminal, readOnly: true, continued: false },
  takeOver: { enabled: true },
  ghostty: { enabled: true },
  ...overrides
});

interface DaemonOptions {
  sessions: (attempt: string | null) => unknown;
  continueResponse?: () => Response;
}

/** The sessions daemon: the run, its sessions for the attempt asked, outputs, and the Continue and Ghostty commands. */
function stubDaemon(options: DaemonOptions) {
  const daemon = daemonRoutes({
    [viewPath]: fixtureRoute("views-run.json"),
    [`${viewPath}/sessions*`]: request => json(options.sessions(new URL(request.path, "http://127.0.0.1").searchParams.get("attempt"))),
    [`${runPath}/attempts*`]: request => request.path.includes("/output") ? json(goldenText("run-attempt-output.json")) : json(goldenText("run-attempt-artifacts.json")),
    [`POST ${runPath}/continue-session`]: options.continueResponse ?? fixtureRoute("run-continue-session.json"),
    "POST /api/v1/desktop/ghostty": fixtureRoute("desktop-ok.json")
  });
  const pathOf = (request: { path: string }) => request.path.split("?")[0];
  return {
    requests: daemon.requests,
    posts: (path: string) => daemon.requests.filter(request => request.method === "POST" && pathOf(request) === path),
    sessionRequests: () => daemon.requests.filter(request => pathOf(request) === `${viewPath}/sessions`)
  };
}

function renderRun(search: string) {
  localStorage.setItem(preferencesStorageKey, JSON.stringify({ ...defaultPreferences, sessionsOpen: true }));
  return render(<AppProviders><MemoryRouter initialEntries={[`/runs/${encodeURIComponent(runId)}${search}`]}><AppRoutes /></MemoryRouter></AppProviders>);
}

function latestTerminal(): MockTerminal {
  const current = MockTerminal.instances[MockTerminal.instances.length - 1];
  if (!current) throw new Error("no terminal was created");
  return current;
}

beforeEach(() => {
  localStorage.clear();
  resetTerminalDoubles();
  stubTerminalSockets();
});

describe("Sessions sidebar", () => {
  it("shows the daemon's rows with status, caption and selection, and names the fallback and empty copy from the read", async () => {
    stubDaemon({ sessions: () => sessionsBody("done", { selection: { stageId: "watch", attemptId: "file-wave-1", fallback: "Watch the ticket runs no agent. Showing the latest session." }, log: "saved log" }) });
    renderRun("?stage=watch&attempt=watch-1");
    const sidebar = await waitFor(() => {
      const element = document.querySelector<HTMLElement>('[data-column="sessions"][data-ready="true"]');
      expect(element).not.toBeNull();
      return element!;
    });
    expect(within(sidebar).getByLabelText("Done").textContent).toBe("✓");
    expect(within(sidebar).getByText("04 File the wave · attempt 1")).toBeTruthy();
    expect(within(sidebar).getByText("2m 0s · cost not tracked")).toBeTruthy();
    expect(within(sidebar).getByRole("button", { name: /04 File the wave/ }).getAttribute("aria-pressed")).toBe("true");
    expect(within(sidebar).getByText("Watch the ticket runs no agent. Showing the latest session.")).toBeTruthy();
    expect(within(sidebar).getByText("saved log")).toBeTruthy();
  });

  it("shows the daemon's empty copy for a run with no sessions", async () => {
    stubDaemon({ sessions: () => ({ runId, rows: [], selection: { stageId: "plan-epic" }, emptyMessage: "No sessions yet. The first one starts when Plan the epic picks this issue up.", ghostty: noGhostty }) });
    renderRun("");
    expect(await screen.findByText(/^No sessions yet\. The first one starts when Plan the epic/)).toBeTruthy();
  });

  it("selects a row's stage and attempt by asking the daemon for that selection", async () => {
    const user = userEvent.setup();
    const { sessionRequests } = stubDaemon({ sessions: attempt => sessionsBody(attempt === "file-wave-1" ? "done" : "live") });
    renderRun("?stage=open-branches&attempt=open-branches-1");
    await user.click(await screen.findByRole("button", { name: /04 File the wave/ }));
    await waitFor(() => expect(sessionRequests().some(request => new URL(request.path, "http://127.0.0.1").searchParams.get("attempt") === "file-wave-1" && new URL(request.path, "http://127.0.0.1").searchParams.get("stage") === "file-wave")).toBe(true));
    expect(within(screen.getByRole("region", { name: "Selected session" })).getByText("tmux · bl-demo-PAY-40-file-wave")).toBeTruthy();
  });

  it("watch attaches read-only to the daemon's target with no takeOver, and withholds input until writable", async () => {
    const { posts } = stubDaemon({ sessions: () => liveWithTerminal() });
    renderRun("?stage=open-branches&attempt=open-branches-1");
    await screen.findByRole("button", { name: "Take over" });
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(1));
    expect(FakeSocket.instances[0]!.url).toMatch(/\/api\/v1\/terminals\?session=bl-demo-PAY-40-open-branches$/);
    FakeSocket.instances[0]!.open();
    latestTerminal().type("typed while watching");
    expect(FakeSocket.instances[0]!.sentText()).toEqual([]);
    expect(posts("/api/v1/desktop/ghostty")).toHaveLength(0);
  });

  it("take over reconnects with takeOver=1, closes the watch socket, and withholds input until writable", async () => {
    const user = userEvent.setup();
    stubDaemon({ sessions: () => liveWithTerminal() });
    renderRun("?stage=open-branches&attempt=open-branches-1");
    await user.click(await screen.findByRole("button", { name: "Take over" }));
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    expect(FakeSocket.instances[0]!.closed).toEqual([[1000, "terminal closed"]]);
    expect(FakeSocket.instances[1]!.url).toMatch(/\?session=bl-demo-PAY-40-open-branches&takeOver=1$/);
    FakeSocket.instances[1]!.open();
    latestTerminal().type("before permission");
    expect(FakeSocket.instances[1]!.sentText()).toEqual([]);
    FakeSocket.instances[1]!.receiveText(JSON.stringify({ type: "mode", readOnly: false }));
    latestTerminal().type("after permission");
    expect(FakeSocket.instances[1]!.sentText()).toEqual(["after permission"]);
    expect(screen.getByRole("button", { name: "Watch only" })).toBeTruthy();
  });

  it("a Claude stage, once headless, is now watched read-only, taken over with paste, and handed back to watching", async () => {
    const user = userEvent.setup();
    const claude = "bl-claude-acme-142-build-main-1-00000000-0000-4000-8000-000000000001";
    stubDaemon({ sessions: () => liveWithTerminal({ terminal: { target: claude, readOnly: true, continued: false } }) });
    renderRun("?stage=open-branches&attempt=open-branches-1");
    expect(screen.queryByText(/runs headless/)).toBeNull();
    await user.click(await screen.findByRole("button", { name: "Take over" }));
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    FakeSocket.instances[0]!.open();
    expect(FakeSocket.instances[1]!.url).toContain(`?session=${claude}&takeOver=1`);
    FakeSocket.instances[1]!.open();
    expect(await screen.findByText("Watching · read only")).toBeTruthy();
    latestTerminal().type("pasted before permission");
    expect(FakeSocket.instances[1]!.sentText()).toEqual([]);
    FakeSocket.instances[1]!.receiveText(JSON.stringify({ type: "mode", readOnly: false }));
    expect(await screen.findByText("Connected · input on")).toBeTruthy();
    latestTerminal().type("take the left branch\r");
    latestTerminal().type("a pasted\nparagraph");
    expect(FakeSocket.instances[1]!.sentText()).toEqual(["take the left branch\r", "a pasted\nparagraph"]);
    await user.click(screen.getByRole("button", { name: "Watch only" }));
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(3));
    expect(FakeSocket.instances[2]!.url).not.toContain("takeOver");
    FakeSocket.instances[2]!.open();
    latestTerminal().type("after watch only");
    expect(FakeSocket.instances[2]!.sentText()).toEqual([]);
    expect(await screen.findByText("Watching · read only")).toBeTruthy();
  });

  it("a failed take-over shows a connection error and leaves input disabled", async () => {
    const user = userEvent.setup();
    stubDaemon({ sessions: () => liveWithTerminal() });
    renderRun("?stage=open-branches&attempt=open-branches-1");
    await user.click(await screen.findByRole("button", { name: "Take over" }));
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    FakeSocket.instances[1]!.fail();
    expect(await screen.findByText("Terminal offline. Input is off.")).toBeTruthy();
    latestTerminal().type("refused");
    expect(FakeSocket.instances[1]!.sentText()).toEqual([]);
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });

  it("choosing another session clears the take-over choice", async () => {
    const user = userEvent.setup();
    stubDaemon({ sessions: attempt => attempt === "file-wave-1" ? sessionsBody("done") : liveWithTerminal() });
    renderRun("?stage=open-branches&attempt=open-branches-1");
    await user.click(await screen.findByRole("button", { name: "Take over" }));
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    await user.click(screen.getByRole("button", { name: /04 File the wave/ }));
    await user.click(await screen.findByRole("button", { name: /03 Open the feature branch/ }));
    expect(await screen.findByRole("button", { name: "Take over" })).toBeTruthy();
    await waitFor(() => expect(FakeSocket.instances.length).toBeGreaterThanOrEqual(3));
    expect(FakeSocket.instances.at(-1)!.url).not.toContain("takeOver");
  });

  it("Continue attaches to the target the daemon returns, with no takeOver", async () => {
    const user = userEvent.setup();
    const { posts } = stubDaemon({ sessions: () => sessionsBody("done", { continueSession: { enabled: true }, log: "saved" }) });
    renderRun("?stage=file-wave&attempt=file-wave-1");
    await user.click(await screen.findByRole("button", { name: "Continue" }));
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(1));
    expect(FakeSocket.instances[0]!.url).toMatch(/\?session=bl-demo-151-triage-continue$/);
    expect(posts(`/api/v1/runs/${encodeURIComponent(runId)}/continue-session`)).toHaveLength(1);
    expect(posts(`/api/v1/runs/${encodeURIComponent(runId)}/continue-session`)[0]!.body).toEqual({ attempt: "file-wave-1" });
  });

  it("a failed Continue shows the error and never enables input", async () => {
    const user = userEvent.setup();
    stubDaemon({
      sessions: () => sessionsBody("done", { continueSession: { enabled: true }, log: "saved" }),
      continueResponse: () => json('{"message":"This session is already being continued.","problems":[]}', 422)
    });
    renderRun("?stage=file-wave&attempt=file-wave-1");
    await user.click(await screen.findByRole("button", { name: "Continue" }));
    expect(await screen.findByText("This session is already being continued.")).toBeTruthy();
    expect(FakeSocket.instances).toHaveLength(0);
    expect(screen.getByText("saved")).toBeTruthy();
  });

  it("Ghostty sends its typed desktop request for the selected attempt", async () => {
    const user = userEvent.setup();
    const { posts } = stubDaemon({ sessions: () => liveWithTerminal() });
    renderRun("?stage=open-branches&attempt=open-branches-1");
    await user.click(await screen.findByRole("button", { name: "Attach in Ghostty" }));
    await waitFor(() => expect(posts("/api/v1/desktop/ghostty")).toHaveLength(1));
    expect(posts("/api/v1/desktop/ghostty")[0]!.body).toEqual({ runId, attemptId: "open-branches-1" });
  });

  it("reopening Sessions reconnects to the daemon's continuation as the same writable conversation", async () => {
    const continuation = "bl-demo-PAY-40-file-wave-continue";
    stubDaemon({ sessions: () => sessionsBody("done", { terminal: { target: continuation, readOnly: false, continued: true }, continueSession: { enabled: true } }) });
    renderRun("?stage=file-wave&attempt=file-wave-1");
    await waitFor(() => expect(FakeSocket.instances).toHaveLength(1));
    expect(FakeSocket.instances[0]!.url).toMatch(new RegExp(`\\?session=${continuation}$`));
    FakeSocket.instances[0]!.open();
    FakeSocket.instances[0]!.receiveText(JSON.stringify({ type: "mode", readOnly: false }));
    latestTerminal().type("continued");
    expect(FakeSocket.instances[0]!.sentText()).toEqual(["continued"]);
  });
});
