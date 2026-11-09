import { render, screen, waitFor, within } from "@testing-library/react";
import type { BeltRecord } from "./beltRecords";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { AppProviders, AppRoutes } from "../../app/App";
import { contextFor, firstLive, firstName, firstStage, fixture, mockDaemon, nameInput, renderAt, type DaemonRequest } from "./beltsTestKit";
import type { Belt } from "../../api/wire.gen";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";

function withBrowser(records: BeltRecord[]) {
  const cards = records.map(record => ({
    id: record.belt.id, name: record.belt.name || "Unnamed belt", version: record.belt.version,
    summary: record.detail, detail: record.detail,
    status: record.archived ? "Archived" : record.paused ? "Paused" : "Running",
    statusTone: record.archived || record.paused ? "grey" : "good",
    stages: []
  }));
  return { belts: records, liveIds: records.filter(item => !item.archived).map(item => item.belt.id), browser: {
    items: cards, totalCount: records.length, matchCount: records.length,
    filters: [
      { id: "all", label: "All", count: records.length },
      { id: "running", label: "Running", count: records.filter(item => !item.archived && !item.paused).length },
      { id: "paused", label: "Paused", count: records.filter(item => !item.archived && item.paused).length },
      { id: "archived", label: "Archived", count: records.filter(item => item.archived).length }
    ],
    actions: [{ id: "new", label: "+ New belt", enabled: true }]
  } };
}

function renderGrid(path = "/belts") {
  return render(<AppProviders><MemoryRouter initialEntries={[path]}><AppRoutes /></MemoryRouter></AppProviders>);
}

const saveButton = () => screen.getByRole("button", { name: "Save" });

beforeEach(() => {
  localStorage.clear();
});

describe("Belts list", () => {
  it("opens on the daemon-backed browser and opens a card as an explicit detail route", async () => {
    const user = userEvent.setup();
    const body = JSON.stringify(withBrowser(fixture.belts));
    const daemon = mockDaemon({ "/api/v1/belts": body });
    renderGrid();
    await waitFor(() => expect(document.querySelector(".library-browser")).not.toBeNull());
    await within(document.querySelector(".library-browser")!).findByRole("heading", { level: 1, name: "Belts" });
    expect(await screen.findByRole("button", { name: `Open ${firstName}` })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: `Open ${firstName}` }));
    expect(await screen.findByRole("heading", { level: 2, name: firstName })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Back to Belts/ })).toBeNull();
    expect(daemon.requests.some(request => request.path.includes("?search=&filter=all"))).toBe(true);
  });

  it("keeps search and filter state in the URL and requests the matching daemon view", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": JSON.stringify(withBrowser(fixture.belts)) });
    renderGrid("/belts?filter=paused");
    await waitFor(() => expect(document.querySelector(".library-browser")).not.toBeNull());
    await within(document.querySelector(".library-browser")!).findByRole("heading", { level: 1, name: "Belts" });
    const search = screen.getByRole("textbox", { name: "Search belts" });
    await user.type(search, "nightly");
    expect((search as HTMLInputElement).value).toBe("nightly");
    await waitFor(() => expect(daemon.requests.some(request => request.path.includes("filter=paused") && request.path.includes("search=nightly"))).toBe(true));
  });

  it("shows the empty state when there are no belts", async () => {
    mockDaemon({ "/api/v1/belts": JSON.stringify(withBrowser([])) });
    renderGrid();
    expect(await screen.findByRole("heading", { name: "No belts yet" })).toBeTruthy();
  });

  it("names a missing selection and offers the first belt", async () => {
    const user = userEvent.setup();
    mockDaemon({ "/api/v1/belts": JSON.stringify(withBrowser(fixture.belts)) });
    renderAt("/belts?belt=no-such-belt");
    expect(await screen.findByRole("heading", { name: "This belt is missing" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Show the first belt" }));
    expect(await screen.findByRole("heading", { level: 2, name: firstName })).toBeTruthy();
  });

  it("reports an initial read failure as an error", async () => {
    mockDaemon({ "/api/v1/belts": () => json({ message: "The library is unavailable", problems: ["try again"] }, 500) });
    renderAt("/belts");
    expect(await screen.findByRole("heading", { name: "Unable to load belts" })).toBeTruthy();
    expect(screen.getByText("The library is unavailable")).toBeTruthy();
  });
});

describe("Belt editor", () => {
  it("types locally and sends nothing until a command", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json") });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " draft");
    expect(nameInput().value).toBe(`${firstName} draft`);
    expect(daemon.posts()).toHaveLength(0);
  });

  it("offers Save and Discard only while something is unsent, as the app did", async () => {
    const user = userEvent.setup();
    mockDaemon({ "/api/v1/belts": goldenText("belts.json") });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    await user.type(nameInput(), "!");
    expect(saveButton()).toBeTruthy();
    expect(screen.getByRole("button", { name: "Discard" })).toBeTruthy();
  });

  it("sends each structural edit with the whole current draft and shows the returned draft", async () => {
    const user = userEvent.setup();
    const returned = JSON.parse(goldenText("belt-draft-stage-remove.json")) as { draft: Belt; warnings: string[] };
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/draft/stage-remove": () => json(returned)
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " typed");
    await user.click(screen.getByRole("button", { name: `Remove ${firstStage.name}` }));

    await waitFor(() => expect(daemon.posts()).toHaveLength(1));
    const [request] = daemon.posts();
    expect(request!.path).toBe("/api/v1/belts/draft/stage-remove");
    expect(request!.body.op).toBe("stage-remove");
    expect(request!.body.stageId).toBe(firstStage.id);
    expect(request!.body.draft.name).toBe(`${firstName} typed`);
    expect(request!.body.draft.stages).toHaveLength(firstLive.belt.stages.length);

    const stages = await screen.findByRole("region", { name: "Stages" });
    const shown = Array.from(stages.querySelectorAll(".stage-block-wrap[data-stage]")).map(item => item.getAttribute("data-stage"));
    expect(shown).toEqual(returned.draft.stages.map(stage => stage.id));
  });

  it("keeps the input and shows the daemon's problems when a save is rejected with 422", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/save": () => json({ message: "The belt is not valid", problems: ["Name is taken", "Stage two has no agent"] }, 422)
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " changed");
    await user.click(saveButton());

    expect(await screen.findByText("The belt is not valid")).toBeTruthy();
    const alert = screen.getByRole("alert");
    expect(within(alert).getAllByRole("listitem").map(item => item.firstElementChild?.textContent)).toEqual(["Name is taken", "Stage two has no agent"]);
    expect(nameInput().value).toBe(`${firstName} changed`);
    expect(daemon.posts()).toHaveLength(1);
    expect(daemon.posts()[0]!.body.belt.name).toBe(`${firstName} changed`);
  });

  it("saves the whole draft, then returns to no unsent changes", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/save": (request: DaemonRequest) => json({ resource: { ...firstLive, belt: { ...firstLive.belt, name: request.body.belt.name } }, warnings: [] })
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), "!");
    await user.click(saveButton());
    await waitFor(() => expect(daemon.posts()).toHaveLength(1));
    expect(daemon.posts()[0]!.body.belt.name).toBe(`${firstName}!`);
    await waitFor(() => expect(screen.queryByRole("button", { name: "Save" })).toBeNull());
  });

  it("discards unsent edits back to the saved belt without a request", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json") });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " gone");
    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect(nameInput().value).toBe(firstName);
    expect(daemon.posts()).toHaveLength(0);
  });

  it("sends an AFK change as the belt's own command, not as an edit to the draft", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      [`/api/v1/belts/${firstLive.belt.id}/afk`]: (request: DaemonRequest) => json({ resource: { ...firstLive, belt: { ...firstLive.belt, afk: request.body.afk } }, warnings: [] })
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    // The AFK button moves to the next mode: Global, then Always.
    await user.click(screen.getByRole("button", { name: "AFK mode for new runs" }));
    await waitFor(() => expect(daemon.posts()).toHaveLength(1));
    expect(daemon.posts()[0]!.path).toBe(`/api/v1/belts/${firstLive.belt.id}/afk`);
    expect(daemon.posts()[0]!.body).toEqual({ afk: "always" });
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("shows warnings in order, and says saving will pause a warned belt", async () => {
    const user = userEvent.setup();
    const returned = JSON.parse(goldenText("belt-draft-stage-remove.json")) as Record<string, unknown>;
    mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/draft/stage-remove": () => json({ ...returned, warnings: ["Stage one has no route", "Trigger has no repositories"] }),
      "/api/v1/belts/context": request => json({ ...contextFor(request.body.draft), notes: { whileEditing: "Saving will pause this belt until its warnings are resolved.", whenSaved: null } })
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.click(screen.getByRole("button", { name: `Remove ${firstStage.name}` }));
    const warnings = await screen.findByRole("list", { name: "Warnings" });
    expect(within(warnings).getAllByRole("listitem").map(item => item.textContent)).toEqual(["Stage one has no route", "Trigger has no repositories"]);
    expect(await screen.findByText("Saving will pause this belt until its warnings are resolved.")).toBeTruthy();
  });

  it("keeps unsent input across navigation to another page and back", async () => {
    const user = userEvent.setup();
    mockDaemon({ "/api/v1/belts": goldenText("belts.json") });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " kept");
    await user.click(screen.getByRole("link", { name: "Agents" }));
    await screen.findByRole("heading", { level: 1, name: "Agents" });
    await user.click(screen.getByRole("link", { name: "Belts" }));
    await user.click(await screen.findByRole("button", { name: `${firstName} kept Draft` }));
    await screen.findByRole("heading", { level: 2, name: `${firstName} kept` });
    expect(nameInput().value).toBe(`${firstName} kept`);
  });

  it("starts a new belt as an unsaved row whose Save sends the new draft", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({
      "/api/v1/belts": JSON.stringify(withBrowser(fixture.belts)),
      "/api/v1/belts/new": goldenText("belt-new.json"),
      "/api/v1/belts/save": goldenText("belt-save.json")
    });
    renderGrid();
    await user.click(await screen.findByRole("button", { name: /\+ New belt/ }));
    expect(await screen.findByLabelText("Belt name")).toBeTruthy();
    await user.click(saveButton());
    await waitFor(() => expect(daemon.posts().some(request => request.path === "/api/v1/belts/save")).toBe(true));
  });

  it("archives through the daemon and keeps the last list when the refresh fails", async () => {
    const user = userEvent.setup();
    let listReads = 0;
    const daemon = mockDaemon({
      "/api/v1/belts": () => {
        listReads += 1;
        return listReads === 1 ? json(goldenText("belts.json")) : json({ message: "Refresh failed", problems: [] }, 500);
      },
      [`/api/v1/belts/${firstLive.belt.id}/archive`]: goldenText("belt-archive.json")
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.click(screen.getByRole("button", { name: "Archive" }));
    expect(await screen.findByText("Showing the last loaded belts; refresh failed.")).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: firstName })).toBeTruthy();
    expect(daemon.posts().filter(request => request.path.endsWith("/archive"))).toHaveLength(1);
  });
});
