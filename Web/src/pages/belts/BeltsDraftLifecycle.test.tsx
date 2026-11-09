import { screen, waitFor, within } from "@testing-library/react";
import type { BeltRecord } from "./beltRecords";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { contextFor, contextWithBases, contextRoute, firstLive, firstName, firstStage, fixture, mockDaemon, nameInput, renderAt, beltsList, type DaemonRequest } from "./beltsTestKit";
import { history, libraryChanged } from "../../test/daemon";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";

beforeEach(() => {
  localStorage.clear();
});

describe("draft lifecycle", () => {
  it("discarding a copy returns to its source, and the source keeps its own unsent edits", async () => {
    const user = userEvent.setup();
    const copy = JSON.parse(goldenText("belt-copy.json")) as { draft: { name: string } };
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/copy": () => json({ ...copy, draft: { ...copy.draft, name: `${firstName} copy` } })
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " src");
    await user.click(screen.getByRole("button", { name: "Copy belt" }));
    expect(await screen.findByRole("heading", { level: 2, name: `${firstName} copy` })).toBeTruthy();
    await history(user, "Go to belts");
    await user.click(await screen.findByRole("button", { name: `${firstName} copy Draft` }));
    await screen.findByRole("heading", { level: 2, name: `${firstName} copy` });
    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect(await screen.findByRole("heading", { level: 2, name: `${firstName} src` })).toBeTruthy();
    expect(nameInput().value).toBe(`${firstName} src`);
    expect(daemon.posts("/save")).toHaveLength(0);
  });

  it("a live refresh while a draft is dirty updates the saved status and keeps the typed name", async () => {
    const user = userEvent.setup();
    let reads = 0;
    const archivedList = fixture.belts.map(record => record.belt.id === firstLive.belt.id
      ? { ...record, archived: true, actions: record.actions.map(action => action.id === "archive"
        ? { ...action, enabled: false, reason: "This belt is already archived." }
        : action.id === "unarchive" ? { ...action, enabled: true, reason: undefined } : action) }
      : record);
    const daemon = mockDaemon({
      "/api/v1/belts": () => json(reads++ === 0 ? fixture : beltsList(archivedList)),
      [`/api/v1/belts/${firstLive.belt.id}/archive`]: () => json(goldenText("belt-archive.json"))
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " kept");
    await user.click(screen.getByRole("button", { name: "Archive" }));
    expect(await screen.findByRole("button", { name: "Restore" })).toBeTruthy();
    expect(nameInput().value).toBe(`${firstName} kept`);
    expect(daemon.posts("/save")).toHaveLength(0);
    expect(daemon.posts(`/${firstLive.belt.id}/archive`)[0]!.body).toEqual({});
  });

  it("typing while a save is pending keeps that input after the save succeeds", async () => {
    const user = userEvent.setup();
    let release: () => void = () => undefined;
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/save": (request: DaemonRequest) => new Promise<Response>(resolve => {
        release = () => resolve(json({ resource: { ...firstLive, belt: { ...firstLive.belt, name: request.body.belt.name } }, warnings: [] }));
      })
    });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " sent");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(daemon.posts("/save")).toHaveLength(1));
    await user.type(nameInput(), " later");
    release();
    await waitFor(() => expect(nameInput().value).toBe(`${firstName} sent later`));
    expect(screen.queryByText("No unsaved changes to save.")).toBeNull();
    expect(within(screen.getByRole("region", { name: "Belt editor" })).getByText(/Unsaved changes/)).toBeTruthy();
  });

  it("shows a saved belt's warnings from the library read, and says a warned paused belt cannot resume", async () => {
    const warned = fixture.belts.map(record => record.belt.id === firstLive.belt.id
      ? { ...record, paused: true, warnings: ["Stage two has no agent", "Trigger has no repositories"] }
      : record);
    mockDaemon({
      "/api/v1/belts": () => json(beltsList(warned)),
      "/api/v1/belts/context": request => json({ ...contextFor(request.body.draft), notes: { whileEditing: "Saving will pause this belt until its warnings are resolved.", whenSaved: "This belt is paused. Resolve its warnings, save, then resume it." } })
    });
    renderAt("/belts");
    const region = await screen.findByRole("list", { name: "Warnings" });
    expect(within(region).getAllByRole("listitem").map(item => item.textContent)).toEqual(["Stage two has no agent", "Trigger has no repositories"]);
    expect(await screen.findByText("This belt is paused. Resolve its warnings, save, then resume it.")).toBeTruthy();
  });
});

describe("task sheet", () => {
  const manualBelt = fixture.belts.find(record => record.belt.triggers.some(entry => entry.trigger.kind === "manual") && !record.archived)!;

  it("offers each manual trigger by its position, and a second trigger brings its own first repository", async () => {
    const user = userEvent.setup();
    const second = { id: "second", repos: ["acme/second"], trigger: { kind: "manual" } } as BeltRecord["belt"]["triggers"][number];
    const withTwo = fixture.belts.map(record => record.belt.id === manualBelt.belt.id
      ? { ...record, belt: { ...record.belt, triggers: [...record.belt.triggers, second] } }
      : record);
    const daemon = mockDaemon({ "/api/v1/belts": () => json(beltsList(withTwo)), "/api/v1/tasks": () => json(goldenText("run-decide.json")) });
    renderAt(`/belts?belt=${manualBelt.belt.id}`);
    await screen.findByRole("heading", { level: 2, name: manualBelt.belt.name });
    await user.click(screen.getByRole("button", { name: "Start a task…" }));
    const sheet = await screen.findByRole("dialog", { name: "Start a task" });
    await user.selectOptions(within(sheet).getByLabelText("Trigger"), "second");
    expect((within(sheet).getByLabelText("Repository") as HTMLSelectElement).value).toBe("acme/second");
    await user.type(within(sheet).getByLabelText("Title"), "Second");
    await user.click(within(sheet).getByRole("button", { name: "Start" }));
    await waitFor(() => expect(daemon.posts("/api/v1/tasks")).toHaveLength(1));
    expect(daemon.posts("/api/v1/tasks")[0]!.body).toMatchObject({ entry: "second", repo: "acme/second", title: "Second" });
  });

  it("keeps the sheet and its input when the start is refused, with the daemon's message, and explains a belt with no eligible repository", async () => {
    const user = userEvent.setup();
    mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/tasks": () => json({ message: "That repository was removed", problems: ["acme/gone is not on the trigger"] }, 422)
    });
    renderAt(`/belts?belt=${manualBelt.belt.id}`);
    await screen.findByRole("heading", { level: 2, name: manualBelt.belt.name });
    await user.click(screen.getByRole("button", { name: "Start a task…" }));
    const sheet = await screen.findByRole("dialog", { name: "Start a task" });
    await user.type(within(sheet).getByLabelText("Title"), "Keep me");
    await user.click(within(sheet).getByRole("button", { name: "Start" }));
    expect(await within(sheet).findByText("That repository was removed")).toBeTruthy();
    expect((within(sheet).getByLabelText("Title") as HTMLInputElement).value).toBe("Keep me");
  });

});

describe("repository base branches", () => {
  /** The snapshot with the belt changed: the editor opens its draft, the saved belt is the same. */
  const withBelt = (record: BeltRecord, change: Partial<BeltRecord["belt"]>): BeltRecord => ({ ...record, belt: { ...record.belt, ...change }, draft: { ...record.draft, ...change } });

  /** The demo belt with a shared base and a second repository on its first trigger, so two overrides can be made. */
  const withTwoRepositories = (overrides?: Record<string, string>): BeltRecord[] => fixture.belts.map(record => record.belt.id === firstLive.belt.id
    ? withBelt(record, { base: "develop", repositoryBases: overrides, triggers: record.belt.triggers.map((entry, index) => index === 0 ? { ...entry, repos: [...entry.repos, "acme/web"] } : entry) })
    : record);

  const saveRoute = (belts: BeltRecord[]) => ({
    "/api/v1/belts": () => json(beltsList(belts)),
    "/api/v1/belts/context": (request: DaemonRequest) => json(contextWithBases(request.body.draft)),
    "/api/v1/belts/save": (request: DaemonRequest) => json({ resource: { ...belts[0]!, belt: request.body.belt }, warnings: [] })
  });

  it("types one override per repository, sends them only on Save, and clearing a field removes its override", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon(saveRoute(withTwoRepositories({ "acme/web": "release/web" })));
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(await screen.findByLabelText("acme/platform"), "release/app");
    await user.clear(screen.getByLabelText("acme/web"));
    expect((screen.getByLabelText("Base branch") as HTMLInputElement).value).toBe("develop");
    expect(screen.getByText("Repository base branches")).toBeTruthy();
    expect(daemon.posts("/save")).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(daemon.posts("/save")).toHaveLength(1));
    expect(daemon.posts("/save")[0]!.body.belt.repositoryBases).toEqual({ "acme/platform": "release/app" });
  });

  it("reopening a saved belt shows its overrides with the shared-base placeholder and help", async () => {
    mockDaemon(saveRoute(withTwoRepositories({ "acme/platform": "release/app" })));
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    expect((await screen.findByLabelText("acme/platform")).getAttribute("placeholder")).toBe("Use shared base branch");
    expect((screen.getByLabelText("acme/platform") as HTMLInputElement).value).toBe("release/app");
    expect((screen.getByLabelText("acme/web") as HTMLInputElement).value).toBe("");
    expect(screen.getByText(/An empty field starts that repository from the shared base branch/)).toBeTruthy();
  });

});

/** The belts list as the daemon would send it after `edit`, which changes the fixture's belts (and their browser cards) in place. */
function listing(edit: (belts: BeltRecord[], cards: Array<{ id: string; name: string; version: number }>) => void): string {
  const copy = structuredClone(fixture) as unknown as { belts: BeltRecord[]; browser: { items: Array<{ id: string; name: string; version: number }> } };
  edit(copy.belts, copy.browser.items);
  return JSON.stringify(copy);
}

/** Renames the first live belt and gives it its next version, as a configuration change does. */
function renamedFirstLive(name: string): string {
  return listing((belts, cards) => {
    const record = belts.find(item => item.belt.id === firstLive.belt.id)!;
    record.belt = { ...record.belt, name, version: record.belt.version + 1 };
    record.draft = { ...record.draft, name, version: record.draft.version + 1 };
    cards.find(card => card.id === firstLive.belt.id)!.name = name;
    cards.find(card => card.id === firstLive.belt.id)!.version = record.belt.version;
  });
}

/** The list without the first live belt, as after it was removed from the data folder. */
function listingWithoutFirstLive(): string {
  return listing((belts, cards) => {
    belts.splice(belts.findIndex(item => item.belt.id === firstLive.belt.id), 1);
    cards.splice(cards.findIndex(card => card.id === firstLive.belt.id), 1);
  });
}

/** The list without the first stage of the first live belt, as after a configurator edit removed it. */
function withoutFirstStage(): string {
  return listing((belts, cards) => {
    const record = belts.find(item => item.belt.id === firstLive.belt.id)!;
    const stages = record.belt.stages.filter(stage => stage.id !== firstStage.id);
    record.belt = { ...record.belt, stages, version: record.belt.version + 1 };
    record.draft = { ...record.draft, stages, version: record.draft.version + 1 };
    cards.find(card => card.id === firstLive.belt.id)!.version = record.belt.version;
  });
}

/** Opens the first stage's settings from the stage line, and waits until its menu has loaded. */
async function openFirstStage(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: firstStage.name, pressed: false }));
  const form = await screen.findByRole("region", { name: `Stage ${firstStage.name} settings` });
  await waitFor(() => expect(form.getAttribute("data-ready")).toBe("true"));
  return form;
}

describe("saved belt refresh", () => {
  it("shows a configured belt's new name when it is reopened in the mounted page, and a clean open belt follows a library change", async () => {
    const user = userEvent.setup();
    let list = goldenText("belts.json");
    const daemon = mockDaemon({ "/api/v1/belts": () => json(list) });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    list = renamedFirstLive("Configured belt");
    libraryChanged(2);
    await screen.findByRole("heading", { level: 2, name: "Configured belt" });
    expect(nameInput().value).toBe("Configured belt");
    list = renamedFirstLive("Reopened belt");
    await history(user, "Go to belts");
    await user.click(await screen.findByRole("button", { name: "Open Reopened belt" }));
    await screen.findByRole("heading", { level: 2, name: "Reopened belt" });
    expect(nameInput().value).toBe("Reopened belt");
    expect(daemon.posts().length).toBe(0);
  });

  it("keeps unsent input through refreshes and navigation, then Discard shows the newest saved belt and sends nothing", async () => {
    const user = userEvent.setup();
    let list = goldenText("belts.json");
    const daemon = mockDaemon({ "/api/v1/belts": () => json(list), "/api/v1/belts/context": contextRoute() });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " local");
    await user.type(await screen.findByLabelText("Has label for trigger 1"), "needs-review");
    const typedTrigger = (screen.getByLabelText("Has label for trigger 1") as HTMLInputElement).value;
    list = renamedFirstLive("Configured belt");
    libraryChanged(2);
    await waitFor(() => expect(daemon.requests.filter(request => request.path === "/api/v1/belts").length).toBeGreaterThanOrEqual(2));
    expect(nameInput().value).toBe(`${firstName} local`);
    expect((screen.getByLabelText("Has label for trigger 1") as HTMLInputElement).value).toBe(typedTrigger);
    expect(daemon.posts("trigger-update").length).toBe(0);
    await history(user, "Go to belts");
    await user.click(await screen.findByRole("button", { name: `${firstName} local Draft` }));
    expect(nameInput().value).toBe(`${firstName} local`);
    await user.click(screen.getByRole("button", { name: "Discard" }));
    await screen.findByRole("heading", { level: 2, name: "Configured belt" });
    expect(nameInput().value).toBe("Configured belt");
    expect(daemon.posts("/api/v1/belts/save").length).toBe(0);
  });

  it("gives a draft that is back to its saved belt the newer saved version", async () => {
    const user = userEvent.setup();
    let list = goldenText("belts.json");
    mockDaemon({ "/api/v1/belts": () => json(list) });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), "x{backspace}");
    expect(nameInput().value).toBe(firstName);
    list = renamedFirstLive("Configured belt");
    libraryChanged(2);
    await screen.findByRole("heading", { level: 2, name: "Configured belt" });
    expect(nameInput().value).toBe("Configured belt");
  });

  it("shows the missing-belt state for a belt a read no longer lists, and keeps unsent input with an explanation until Discard", async () => {
    const user = userEvent.setup();
    let list = goldenText("belts.json");
    const daemon = mockDaemon({ "/api/v1/belts": () => json(list), "/api/v1/belts/context": contextRoute() });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " unsent");
    list = listingWithoutFirstLive();
    libraryChanged(2);
    expect(await screen.findByText("This belt is no longer in the library")).toBeTruthy();
    expect(nameInput().value).toBe(`${firstName} unsent`);
    await user.click(screen.getByRole("button", { name: "Discard changes" }));
    expect(await screen.findByRole("heading", { level: 2, name: "This belt is missing" })).toBeTruthy();
    expect(screen.queryByText(/no longer in the library/)).toBeNull();
    expect(daemon.posts("/api/v1/belts/save").length).toBe(0);
  });

  it("keeps a stage's unapplied input through a refresh that removes the stage, navigation and Discard, and sends no save", async () => {
    const user = userEvent.setup();
    let list = goldenText("belts.json");
    const daemon = mockDaemon({ "/api/v1/belts": () => json(list), "/api/v1/belts/context": contextRoute() });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    const form = await openFirstStage(user);
    await user.type(within(form).getByLabelText("Brief"), " Unsent.");
    const typed = (screen.getByLabelText("Brief") as HTMLTextAreaElement).value;
    list = withoutFirstStage();
    libraryChanged(2);
    await waitFor(() => expect(daemon.requests.filter(request => request.path === "/api/v1/belts").length).toBeGreaterThanOrEqual(2));
    expect((screen.getByLabelText("Brief") as HTMLTextAreaElement).value).toBe(typed);
    await history(user, "Go to belts");
    await user.click(await screen.findByRole("button", { name: `${firstName} Draft` }));
    await screen.findByRole("region", { name: `Stage ${firstStage.name} settings` });
    expect((screen.getByLabelText("Brief") as HTMLTextAreaElement).value).toBe(typed);
    await user.click(screen.getByRole("button", { name: "Discard" }));
    await waitFor(() => expect(screen.queryByRole("region", { name: `Stage ${firstStage.name} settings` })).toBeNull());
    expect(daemon.posts("/api/v1/belts/save")).toHaveLength(0);
  });

  it("keeps the open belt and its unsent input when a refresh fails, with the error and no missing explanation", async () => {
    const user = userEvent.setup();
    let failing = false;
    mockDaemon({ "/api/v1/belts": () => failing ? json({ message: "Unavailable", problems: [] }, 503) : json(goldenText("belts.json")) });
    renderAt("/belts");
    await screen.findByRole("heading", { level: 2, name: firstName });
    await user.type(nameInput(), " typed");
    failing = true;
    libraryChanged(2);
    expect(await screen.findByText("Showing the last loaded belts; refresh failed.")).toBeTruthy();
    expect(nameInput().value).toBe(`${firstName} typed`);
    expect(screen.queryByText(/no longer in the library/)).toBeNull();
    expect(screen.queryByText("This belt is missing")).toBeNull();
  });
});
