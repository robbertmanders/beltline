import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { AppProviders, AppRoutes } from "../../app/App";
import { preferencesStorageKey } from "../../state/preferences";
import { json } from "../../test/response";
import { stubDaemon } from "../../test/daemon";
import { goldenText } from "../../test/golden";

const runPath = "/api/v1/views/run/issues%3Aacme%2Fweb%2361";

/** The daemon's Sessions read for this run: three finished attempts, the last one selected. */
const sessionsBody = JSON.stringify({
  runId: "issues:acme/web#61",
  rows: [
    { attemptId: "triage-1", stageId: "triage", title: "01 Triage · attempt 1", caption: "3m 0s · $0.04", status: "done", session: "bl-demo-61-triage", selected: false },
    { attemptId: "quick-build-1", stageId: "quick-build", title: "02 Quick build · attempt 1", caption: "9m 0s · cost not tracked", status: "done", session: "bl-demo-61-quick-build", selected: false },
    { attemptId: "wrap-up-1", stageId: "wrap-up", title: "05 Wrap up · attempt 1", caption: "1m 0s · cost not tracked", status: "problem", session: "bl-demo-61-wrap-up", selected: true }
  ],
  selection: { stageId: "wrap-up", attemptId: "wrap-up-1" },
  ghostty: { enabled: false, reason: "No terminal is running for this session." },
  log: "Saved transcript",
  continueSession: { enabled: false, reason: "the run has left Wrap up" }
});

const emptySessionsBody = JSON.stringify({
  runId: "issues:acme/web#61",
  rows: [],
  selection: { stageId: "triage" },
  emptyMessage: "No sessions yet. The first one starts when Triage picks this issue up.",
  ghostty: { enabled: false, reason: "This run has no session to open in Ghostty." }
});

/** The run page's daemon: the run (a golden, or a response the test makes) and its sessions. */
function stubRun(run: string | (() => Response), sessions: string = sessionsBody) {
  const daemon = stubDaemon({
    [runPath]: typeof run === "string" ? fixtureRouteText(run) : run,
    [`${runPath}?*`]: typeof run === "string" ? fixtureRouteText(run) : run,
    [`${runPath}/sessions*`]: () => json(sessions)
  });
  return { requests: daemon.requests, posts: () => daemon.requests.filter(request => request.method === "POST") };
}

function renderRun() {
  return render(<AppProviders><MemoryRouter initialEntries={["/runs/issues%3Aacme%2Fweb%2361"]}><AppRoutes /></MemoryRouter></AppProviders>);
}

const sessionsButton = () => screen.getByRole("button", { name: "Sessions" });

beforeEach(() => {
  localStorage.clear();
});

describe("run page Sessions route", () => {
  it("starts closed, then opens a read-only sidebar that lists the daemon's session rows", async () => {
    const user = userEvent.setup();
    const { posts } = stubRun(goldenText("views-run-forked.json"));
    renderRun();
    await screen.findByRole("heading", { level: 1, name: /Remember the last export format$/ });
    expect(sessionsButton().getAttribute("aria-expanded")).toBe("false");
    expect(document.querySelector('[data-column="sessions"]')).toBeNull();
    await user.click(sessionsButton());
    const panel = await waitFor(() => {
      const element = document.querySelector<HTMLElement>('[data-column="sessions"][data-ready="true"]');
      expect(element).not.toBeNull();
      return element!;
    });
    expect(within(panel).getByRole("heading", { level: 2, name: "Sessions" })).toBeTruthy();
    expect(within(panel).getByText(/read only/)).toBeTruthy();
    expect(within(panel).getByText("01 Triage · attempt 1")).toBeTruthy();
    expect(within(panel).getByRole("button", { name: /01 Triage · attempt 1/ }).getAttribute("title")).toBe("bl-demo-61-triage");
    expect(within(panel).getByText("3m 0s · $0.04")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Sessions" })).toBeNull();
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).sessionsOpen).toBe(true);
    expect(posts()).toHaveLength(0);
  });

  it("shows the daemon's empty copy when no attempt ran in a session", async () => {
    const user = userEvent.setup();
    stubRun(goldenText("views-run-forked.json"), emptySessionsBody);
    renderRun();
    await screen.findByRole("heading", { level: 1, name: /Remember the last export format$/ });
    await user.click(sessionsButton());
    expect(await screen.findByText(/^No sessions yet\./)).toBeTruthy();
  });

  it("hides from the sidebar's own control, storing the preference and sending nothing", async () => {
    const user = userEvent.setup();
    const { posts } = stubRun(goldenText("views-run-forked.json"));
    renderRun();
    await user.click(await screen.findByRole("button", { name: "Sessions" }));
    await user.click(await screen.findByRole("button", { name: "Hide ›" }));
    expect(document.querySelector('[data-column="sessions"]')).toBeNull();
    expect(sessionsButton().getAttribute("aria-expanded")).toBe("false");
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).sessionsOpen).toBe(false);
    expect(posts()).toHaveLength(0);
  });

  it("keeps the open preference across remounting the run page", async () => {
    const user = userEvent.setup();
    stubRun(goldenText("views-run-forked.json"));
    const first = renderRun();
    await user.click(await screen.findByRole("button", { name: "Sessions" }));
    await screen.findByText("01 Triage · attempt 1");
    first.unmount();
    renderRun();
    expect(await screen.findByText("01 Triage · attempt 1")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Sessions" })).toBeNull();
  });

  it("shows a failed run read with the daemon's problems through the shared list", async () => {
    stubRun(() => json('{"message":"Run view failed","problems":["stage Build is missing","stage Build is missing"]}', 500));
    renderRun();
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Run view failed")).toBeTruthy();
    expect(within(alert).getAllByRole("listitem").map(item => item.textContent)).toEqual(["stage Build is missing", "stage Build is missing"]);
  });
});

/** A route that answers with the given text, as a golden does. */
const fixtureRouteText = (text: string) => () => json(text);
