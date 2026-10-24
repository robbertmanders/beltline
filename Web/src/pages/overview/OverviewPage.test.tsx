import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppProviders, AppRoutes } from "../../app/App";
import { defaultPreferences, preferencesStorageKey, savePreferences } from "../../state/preferences";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";
import { AppSocket } from "../../test/setup";
import { stubDaemon } from "../../test/daemon";

const overviewPath = "/api/v1/views/overview";

type OverviewBody = Record<string, any>;
/** The demo's Overview, as the daemon sends it. */
const overviewGolden = (): OverviewBody => JSON.parse(goldenText("views-overview.json")) as OverviewBody;

/** The overview's daemon: each read of the overview is answered by `answer`, which gets the read's number. */
function stubOverview(answer: (call: number) => Response | Promise<Response>) {
  let reads = 0;
  const daemon = stubDaemon({ [overviewPath]: () => { reads += 1; return answer(reads); } });
  return { ...daemon, reads: () => reads };
}

function renderOverview(entry = "/overview") {
  return render(<AppProviders><MemoryRouter initialEntries={[entry]}><AppRoutes /></MemoryRouter></AppProviders>);
}

/** Waits until the page has its answer: a drawn view, or an error. */
const answered = () => waitFor(() => expect(document.querySelector(".overview-summary, .overview-floor, .error-panel")).not.toBeNull());
const modeButton = (name: string) => screen.getByRole("button", { name });
/** A belt card's pin on the Floor. */
const pinButton = (belt: string) => document.querySelector<HTMLButtonElement>(`.box-belt[data-belt='${belt}'] .box-belt-pin`)!;
/** Stores a full preferences record, since the stored shape is strict and a partial one reads as the defaults. */
const storePin = (floorPinnedBeltId: string | null) => savePreferences({ ...defaultPreferences, floorPinnedBeltId });

beforeEach(() => {
  localStorage.clear();
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(new Date(2026, 9, 9, 9, 30));
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("Overview page", () => {
  it("greets by the fixed clock, shows the caption and the three controls, and lands on Summary", async () => {
    stubOverview(() => json(goldenText("views-overview.json")));
    renderOverview();
    await answered();
    expect(screen.getByRole("heading", { level: 1, name: "Overview" })).toBeTruthy();
    expect(screen.getByText("Good morning.")).toBeTruthy();
    expect(screen.getByText(overviewGolden().caption)).toBeTruthy();
    expect(modeButton("Summary").getAttribute("aria-pressed")).toBe("true");
    expect(modeButton("Floor").getAttribute("aria-pressed")).toBe("false");
  });

  it("renders all six Summary sections with the daemon's limits and order", async () => {
    stubOverview(() => json(goldenText("views-overview.json")));
    renderOverview();
    await answered();
    for (const name of ["Spend", "Sessions", "Running now", "Shipped", "In progress", "Needs you"]) {
      expect(screen.getByRole("region", { name })).toBeTruthy();
    }
    const body = overviewGolden();
    const inProgress = within(screen.getByRole("region", { name: "In progress" })).queryAllByRole("button");
    expect(inProgress.length).toBe(Math.min(body.today.inProgress.rows.length, 6));
    const needs = within(screen.getByRole("region", { name: "Needs you" })).queryAllByRole("button");
    expect(needs.length).toBe(body.today.needsYou.rows.length);
    expect(within(screen.getByRole("region", { name: "Sessions" })).getByTestId("session-grid").children.length).toBe(56);
  });

  it("shows the daemon's empty copy for Running, In progress and Needs you", async () => {
    stubOverview(() => json(goldenText("views-overview-empty.json")));
    renderOverview();
    await answered();
    expect(screen.getByText("Nothing is running.")).toBeTruthy();
    expect(screen.getByText("Nothing in progress.")).toBeTruthy();
    expect(screen.getByText("Nothing needs you.")).toBeTruthy();
  });

  it("switches between Summary and Floor without a new request and keeps the saved belt pin", async () => {
    storePin("issues");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const overview = stubOverview(() => json(goldenText("views-overview.json")));
    renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    expect(document.querySelector(".overview-floor")).not.toBeNull();
    expect(modeButton("Floor").getAttribute("aria-pressed")).toBe("true");
    expect(pinButton("issues").getAttribute("aria-pressed")).toBe("true");
    await user.click(modeButton("Summary"));
    expect(document.querySelector(".overview-summary")).not.toBeNull();
    expect(overview.reads()).toBe(1);
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).floorPinnedBeltId).toBe("issues");
  });

  it("pins a belt from the Floor, saves it, and restores it after a remount", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubOverview(() => json(goldenText("views-overview.json")));
    const first = renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    await user.click(pinButton("issues"));
    expect(JSON.parse(localStorage.getItem(preferencesStorageKey)!).floorPinnedBeltId).toBe("issues");
    first.unmount();
    renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    expect(pinButton("issues").getAttribute("aria-pressed")).toBe("true");
  });

  it("ignores a saved pin for a belt the floor no longer has, with no dimming", async () => {
    storePin("gone");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubOverview(() => json(goldenText("views-overview.json")));
    renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    expect(document.querySelectorAll(".box-belt.is-pinned").length).toBe(0);
    expect(document.querySelectorAll("[style*='opacity: 0.35']").length).toBe(0);
  });

  it("refetches on a runs change and redraws the Summary, keeping the pin", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
        const changed = { ...overviewGolden(), caption: "9 running · 0 need you · 0 shipped today" };
    const overview = stubOverview(call => json(JSON.stringify(call === 1 ? overviewGolden() : changed)));
    renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    await user.click(pinButton("issues"));
    await act(async () => {
      AppSocket.instances[0]!.send({ type: "hello", apiVersion: 1 });
      AppSocket.instances[0]!.send({ type: "changed", resource: "runs", revision: 2 });
    });
    expect(await screen.findByText(changed.caption)).toBeTruthy();
    expect(overview.reads()).toBeGreaterThanOrEqual(2);
    expect(pinButton("issues").getAttribute("aria-pressed")).toBe("true");
  });

  it("shows the pending state, then the first-read error", async () => {
    stubOverview(() => new Promise<Response>(() => {}));
    renderOverview();
    expect(await screen.findByText("Loading Overview…")).toBeTruthy();
    cleanup();
    stubOverview(() => json(goldenText("error-not-found.json"), 500));
    renderOverview();
    expect(await screen.findByRole("heading", { name: "Unable to load Overview" })).toBeTruthy();
  });

  it("keeps the last valid view with a refresh-failed message when a refetch fails", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
        stubOverview(call => call === 1 ? json(goldenText("views-overview.json")) : json(goldenText("error-not-found.json"), 500));
    renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    const drawn = document.querySelectorAll(".floor-box").length;
    await act(async () => {
      AppSocket.instances[0]!.send({ type: "hello", apiVersion: 1 });
      AppSocket.instances[0]!.send({ type: "changed", resource: "runs", revision: 2 });
    });
    expect(await screen.findByText("Showing the last loaded view; refresh failed.")).toBeTruthy();
    expect(document.querySelectorAll(".floor-box").length).toBe(drawn);
  });

  it("refetches on a library change, and falls back to no focus when the pinned belt is removed", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
        storePin("issues");
    const removed = overviewGolden();
    removed.floor.belts = removed.floor.belts.filter((belt: { id: string }) => belt.id !== "issues");
    removed.floor.boxes = removed.floor.boxes.map((box: { belts: string[] }) => ({ ...box, belts: box.belts.filter(id => id !== "issues") }));
    const overview = stubOverview(call => json(JSON.stringify(call === 1 ? overviewGolden() : removed)));
    renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    expect(pinButton("issues").getAttribute("aria-pressed")).toBe("true");
    await act(async () => {
      AppSocket.instances[0]!.send({ type: "hello", apiVersion: 1 });
      AppSocket.instances[0]!.send({ type: "changed", resource: "library", revision: 2 });
    });
    await waitFor(() => expect(overview.reads()).toBeGreaterThanOrEqual(2));
    expect(document.querySelector(".box-belt[data-belt='issues']")).toBeNull();
    expect(document.querySelectorAll(".box-belt.is-pinned").length).toBe(0);
    expect(document.querySelectorAll(".factory-track-focus").length).toBe(0);
  });

  it("stays usable when browser storage throws on read and write", async () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("denied"); });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("denied"); });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubOverview(() => json(goldenText("views-overview.json")));
    renderOverview();
    await answered();
    await user.click(modeButton("Floor"));
    expect(document.querySelector(".box-floor")).not.toBeNull();
  });
});
