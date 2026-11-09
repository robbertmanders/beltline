import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { answer, contextRoute, docs, echo, firstLive, firstName, firstStage, fixture, heading, mockDaemon, nameInput, renderAt, selectBelt } from "./beltsTestKit";
import { history } from "../../test/daemon";
import { json } from "../../test/response";
import { goldenText } from "../../test/golden";

/** The Belts page's behaviour rules 2 to 4: unsent input, structural edits and their answers, triggers and repositories. */

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  Object.defineProperty(window.navigator, "onLine", { configurable: true, value: true });
  window.dispatchEvent(new Event("online"));
});

describe("rule 2: unsent input survives selection, pages and history", () => {
  it("keeps each belt's typed name, typed trigger settings and open stage through another belt, another page, Back and Forward", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": contextRoute(), "/api/v1/models/pi": goldenText("models.json") });
    renderAt("/belts");
    await heading(firstName);
    await user.type(nameInput(), " one");
    await user.clear(screen.getByLabelText("Has label for trigger 1"));
    await user.type(screen.getByLabelText("Has label for trigger 1"), "typed-label");
    await user.click(screen.getByRole("button", { name: firstStage.name, pressed: false }));
    const form = await screen.findByRole("region", { name: `Stage ${firstStage.name} settings` });
    await user.type(within(form).getByLabelText("Name"), " renamed");
    await user.click(screen.getByRole("button", { name: "Trigger" }));

    await selectBelt(user, docs.belt.name);
    await heading(docs.belt.name);
    await user.type(nameInput(), " two");
    await history(user, "Go to agents");
    await screen.findByRole("heading", { level: 1, name: "Agents" });
    await history(user, "Back");
    await heading(`${docs.belt.name} two`);
    await history(user, "Go to belts");
    await selectBelt(user, firstName);
    await heading(`${firstName} one`);
    expect((screen.getByLabelText("Has label for trigger 1") as HTMLInputElement).value).toBe("typed-label");
    await user.click(screen.getByRole("button", { name: firstStage.name, pressed: false }));
    const reopened = await screen.findByRole("region", { name: `Stage ${firstStage.name} settings` });
    expect((within(reopened).getByLabelText("Name") as HTMLInputElement).value).toBe(`${firstStage.name} renamed`);
    await user.click(screen.getByRole("button", { name: "Trigger" }));
    await history(user, "Go to belts");
    await selectBelt(user, docs.belt.name);
    await heading(`${docs.belt.name} two`);
    await history(user, "Back");
    await screen.findByRole("heading", { level: 1, name: "Belts" });
    await history(user, "Forward");
    await heading(`${docs.belt.name} two`);
    // Typing, selecting and navigating saved nothing. Leaving the label field applied it to the draft, once.
    expect(daemon.posts().map(request => request.path)).toEqual(["/api/v1/belts/draft/trigger-update"]);
    expect((daemon.posts()[0]!.body as any).trigger.label).toBe("typed-label");
  }, 20_000);

  it("keeps a trigger's kind stash when another belt is selected, and sends it when the kind is switched back", async () => {
    const user = userEvent.setup();
    const issues = firstLive.belt.triggers[0]!;
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/context": contextRoute(),
      "/api/v1/belts/draft/trigger-kind": request => json({
        ...answer("belt-draft-trigger-kind.json"),
        draft: { ...request.body.draft, triggers: request.body.draft.triggers.map((entry: any) => entry.id === request.body.entryId
          ? { ...entry, trigger: request.body.stash?.triggers?.[request.body.kind] ?? { kind: request.body.kind, jql: "", poll: 60 } } : entry) }
      })
    });
    renderAt("/belts");
    await heading(firstName);
    await user.selectOptions(screen.getByRole("combobox", { name: "Kind of trigger 1" }), "jira");
    await waitFor(() => expect(daemon.posts("/draft/trigger-kind")).toHaveLength(1));
    await waitFor(() => expect((screen.getByRole("combobox", { name: "Kind of trigger 1" }) as HTMLSelectElement).value).toBe("jira"));
    await selectBelt(user, docs.belt.name);
    await heading(docs.belt.name);
    await selectBelt(user, firstName);
    await heading(firstName);
    await user.selectOptions(screen.getByRole("combobox", { name: "Kind of trigger 1" }), "issues");
    await waitFor(() => expect(daemon.posts("/draft/trigger-kind")).toHaveLength(2));
    expect(daemon.posts("/draft/trigger-kind")[1]!.body.stash).toMatchObject({ entry: issues.id, triggers: { issues: issues.trigger, jira: { kind: "jira" } } });
  });

  it("saves typed trigger settings with the belt: Save applies them first, and is never shown unusable", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": contextRoute(),
      "/api/v1/belts/draft/trigger-update": request => json({
        draft: { ...request.body.draft, triggers: request.body.draft.triggers.map((entry: any) => entry.id === request.body.entryId ? { ...entry, trigger: request.body.trigger } : entry) },
        warnings: [], actions: []
      }),
      "/api/v1/belts/save": request => json({ resource: { ...firstLive, belt: request.body.belt }, warnings: [] })
    });
    renderAt("/belts");
    await heading(firstName);
    await user.clear(screen.getByLabelText("Has label for trigger 1"));
    await user.type(screen.getByLabelText("Has label for trigger 1"), "typed-label");
    const save = screen.getByRole("button", { name: "Save" }) as HTMLButtonElement;
    expect(save.disabled).toBe(false);
    await user.click(save);
    await waitFor(() => expect(daemon.posts("/save")).toHaveLength(1));
    expect(daemon.posts().map(request => request.path)).toEqual(["/api/v1/belts/draft/trigger-update", "/api/v1/belts/save"]);
    expect((daemon.posts("/save")[0]!.body.belt.triggers[0].trigger as any).label).toBe("typed-label");
  });

  it("drops typed trigger settings with the rest on Discard", async () => {
    const user = userEvent.setup();
    mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": contextRoute() });
    renderAt("/belts");
    await heading(firstName);
    await user.clear(screen.getByLabelText("Has label for trigger 1"));
    await user.type(screen.getByLabelText("Has label for trigger 1"), "typed-label");
    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect((screen.getByLabelText("Has label for trigger 1") as HTMLInputElement).value).toBe(String((firstLive.belt.triggers[0]!.trigger as any).label));
  });
});

describe("rule 3: structural edits and late answers", () => {
  it("applies a late answer to the belt it was sent for, never to the belt on screen", async () => {
    const user = userEvent.setup();
    let release: () => void = () => undefined;
    mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/context": contextRoute(),
      "/api/v1/belts/draft/stage-remove": request => new Promise<Response>(resolve => {
        release = () => resolve(json({ ...answer("belt-draft-stage-remove.json"), draft: { ...request.body.draft, name: "Answered", stages: request.body.draft.stages.slice(1) } }));
      })
    });
    renderAt("/belts");
    await heading(firstName);
    await user.click(screen.getByRole("button", { name: `Remove ${firstStage.name}` }));
    await selectBelt(user, docs.belt.name);
    await heading(docs.belt.name);
    release();
    // The answer went to the belt it was sent for: the belt on screen stays clean.
    await waitFor(() => expect(screen.queryByRole("button", { name: "Save" })).toBeNull());
    expect(nameInput().value).toBe(docs.belt.name);
    expect(screen.getAllByRole("button", { name: docs.belt.stages[0]!.name }).length).toBeGreaterThan(0);
    await selectBelt(user, firstName);
    await heading("Answered");
    expect(screen.queryByRole("button", { name: `Remove ${firstStage.name}` })).toBeNull();
  });

  it("sends one structural edit while one is pending, however fast the clicks come", async () => {
    const user = userEvent.setup();
    let release: () => void = () => undefined;
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/context": contextRoute(),
      "/api/v1/belts/draft/stage-remove": request => new Promise<Response>(resolve => { release = () => resolve(json({ ...answer("belt-draft-stage-remove.json"), draft: request.body.draft })); })
    });
    renderAt("/belts");
    await heading(firstName);
    await user.click(screen.getByRole("button", { name: `Remove ${firstStage.name}` }));
    // While it is pending the line is read only, and the other commands wait.
    expect(screen.queryByRole("button", { name: `Remove ${firstStage.name}` })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Copy belt" }));
    expect(daemon.posts()).toHaveLength(1);
    expect(screen.getAllByText("A belt command is already running.").length).toBeGreaterThan(0);
    release();
    await waitFor(() => expect(screen.queryAllByText("A belt command is already running.")).toHaveLength(0));
    expect(daemon.posts()).toHaveLength(1);
  });
});

describe("rule 3: drag and nested forks", () => {
  /** A minimal DataTransfer, which jsdom lacks. */
  function transfer() {
    const data = new Map<string, string>();
    return { setData: (type: string, value: string) => data.set(type, value), getData: (type: string) => data.get(type) ?? "", get types() { return [...data.keys()]; }, effectAllowed: "", dropEffect: "" };
  }

  it("moves a stage dropped onto another to that stage's place with stage-move and a target", async () => {
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": contextRoute(), "/api/v1/belts/draft/stage-move": echo("belt-draft-stage-move.json") });
    renderAt("/belts");
    await heading(firstName);
    const items = document.querySelectorAll<HTMLElement>(".stage-block-wrap");
    const dataTransfer = transfer();
    fireEvent.dragStart(items[0]!, { dataTransfer });
    fireEvent.dragOver(items[2]!, { dataTransfer });
    fireEvent.drop(items[2]!, { dataTransfer });
    await waitFor(() => expect(daemon.posts("/draft/stage-move")).toHaveLength(1));
    expect(daemon.posts("/draft/stage-move")[0]!.body).toMatchObject({ stageId: firstLive.belt.stages[0]!.id, targetId: firstLive.belt.stages[2]!.id });
    expect(daemon.posts("/draft/stage-move")[0]!.body.step).toBeUndefined();
  });

  it("removes a nested fork by its own id and shows the daemon's answer", async () => {
    const user = userEvent.setup();
    const nested = fixture.belts.find(record => record.belt.id === "nested")!;
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/context": contextRoute(),
      "/api/v1/belts/draft/fork-remove": request => json({ ...answer("belt-draft-fork-remove.json"), draft: {
        ...request.body.draft,
        forks: request.body.draft.forks.filter((fork: any) => fork.id !== "pace"),
        stages: request.body.draft.stages.map((stage: any) => stage.route === "quick" || stage.route === "deep" ? { ...stage, route: "build" } : stage)
      } })
    });
    renderAt("/belts?belt=nested");
    await heading(nested.belt.name);
    await user.click(screen.getByRole("button", { name: "Remove fork pace" }));
    await waitFor(() => expect(daemon.posts("/draft/fork-remove")).toHaveLength(1));
    expect(daemon.posts("/draft/fork-remove")[0]!.body).toMatchObject({ forkId: "pace", draft: { id: "nested" } });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Remove fork pace" })).toBeNull());
    expect(screen.getByRole("button", { name: "Remove fork wave" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove fork later" })).toBeTruthy();
  });
});

describe("rule 4: repositories", () => {
  it("keeps input the daemon did not add, says so, and clears it once a repository is added", async () => {
    const user = userEvent.setup();
    let take = false;
    const daemon = mockDaemon({
      "/api/v1/belts": goldenText("belts.json"),
      "/api/v1/belts/context": contextRoute(),
      "/api/v1/belts/draft/repository-add": request => json({
        ...answer("belt-draft-repository-add.json"),
        draft: take ? { ...request.body.draft, triggers: request.body.draft.triggers.map((entry: any) => ({ ...entry, repos: [...entry.repos, "acme/new"] })) } : request.body.draft
      })
    });
    renderAt("/belts");
    await heading(firstName);
    const input = screen.getByLabelText("Add a repository to trigger 1") as HTMLInputElement;
    await user.type(input, "not a repository{Enter}");
    await waitFor(() => expect(daemon.posts("/draft/repository-add")).toHaveLength(1));
    expect(daemon.posts("/draft/repository-add")[0]!.body.repository).toBe("not a repository");
    expect(await screen.findByText("Not added: this is not owner/repository, or it is already listed.")).toBeTruthy();
    expect(input.value).toBe("not a repository");
    take = true;
    await user.clear(input);
    expect(screen.queryByText(/^Not added/)).toBeNull();
    await user.type(input, "https://github.com/acme/new{Enter}");
    await waitFor(() => expect(daemon.posts("/draft/repository-add")).toHaveLength(2));
    expect(daemon.posts("/draft/repository-add")[1]!.body.repository).toBe("https://github.com/acme/new");
    await waitFor(() => expect((screen.getByLabelText("Add a repository to trigger 1") as HTMLInputElement).value).toBe(""));
    expect(screen.getByRole("button", { name: "Remove acme/new from trigger 1" })).toBeTruthy();
  });

  it("uses the daemon's choices for polling and assignee, in seconds", async () => {
    const user = userEvent.setup();
    const daemon = mockDaemon({ "/api/v1/belts": goldenText("belts.json"), "/api/v1/belts/context": contextRoute(), "/api/v1/belts/draft/trigger-update": echo("belt-draft-trigger-update.json") });
    renderAt("/belts");
    await heading(firstName);
    const poll = await screen.findByLabelText("Check every for trigger 1") as HTMLSelectElement;
    expect([...poll.options].map(option => option.textContent)).toEqual(["30 seconds", "1 minute", "5 minutes", "15 minutes"]);
    // As in the app, a pick applies at once.
    await user.selectOptions(poll, "300");
    await waitFor(() => expect(daemon.posts("/draft/trigger-update")).toHaveLength(1));
    expect(daemon.posts("/draft/trigger-update")[0]!.body.trigger).toMatchObject({ kind: "issues", poll: 300 });
  });
});
