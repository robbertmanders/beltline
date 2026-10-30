import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { draftGolden, listGolden, posts, renderAgents, resetAgentsEnvironment, stubAgents, type Record_ } from "./agentsTestKit";
import { json } from "../test/response";
import { golden } from "../test/golden";

beforeEach(resetAgentsEnvironment);

describe("saving a draft", () => {
  it("edits form fields and outcome controls from the keyboard", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    const name = await screen.findByLabelText("Name");
    name.focus();
    await user.clear(name);
    await user.type(name, "Keyboard Triage");
    expect((name as HTMLInputElement).value).toBe("Keyboard Triage");
    const addOutcome = screen.getByRole("button", { name: "+ Add outcome" });
    addOutcome.focus();
    await user.keyboard("{Enter}");
    const newOutcome = screen.getByLabelText("Outcome 5 name");
    newOutcome.focus();
    await user.type(newOutcome, "Keyboard outcome");
    expect((newOutcome as HTMLInputElement).value).toBe("Keyboard outcome");
    expect(posts(requests).filter(request => request.path === "/api/v1/agents/save" || request.path === "/api/v1/agents/new")).toHaveLength(0);
  });

  it("keeps edits made while a save is in flight", async () => {
    const user = userEvent.setup();
    let finishSave!: (response: Response) => void;
    const { requests } = stubAgents({ save: () => new Promise(resolve => { finishSave = resolve; }) });
    renderAgents();
    const name = await screen.findByLabelText("Name") as HTMLInputElement;
    fireEvent.change(name, { target: { value: "Submitted name" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Newer unsaved edit" } });
    await waitFor(() => expect(finishSave).toBeTypeOf("function"));
    const submitted = posts(requests).find(request => request.path === "/api/v1/agents/save")!.body!.agent as Record<string, unknown>;
    finishSave(json({ agent: { ...submitted, version: Number(submitted.version) + 1 }, archived: false, glyph: "funnel", detail: `v${Number(submitted.version) + 1}` }));
    expect(await screen.findByDisplayValue("Newer unsaved edit")).toBeTruthy();
    // The status line under a dirty draft is the daemon's, from the draft presentation.
    expect(await screen.findByText(draftGolden().presentation.status as string)).toBeTruthy();
  });

  it("applies only the most recently requested runner switch", async () => {
    const replies: Array<() => void> = [];
    stubAgents({ switchRunner: body => new Promise(resolve => {
      replies.push(() => resolve({ ...draftGolden(), agent: { ...body.agent, runner: body.runner, model: "" } }));
    }) });
    renderAgents();
    const runner = (await screen.findAllByRole("combobox", { name: "Harness" }))[0] as HTMLSelectElement;
    await waitFor(() => expect(runner.disabled).toBe(false));
    fireEvent.change(runner, { target: { value: "codex" } });
    fireEvent.change(runner, { target: { value: "claude" } });
    await waitFor(() => expect(replies).toHaveLength(2));
    await act(async () => replies[0]!());
    expect(runner.value).toBe("pi");
    await act(async () => replies[1]!());
    await waitFor(() => expect(runner.value).toBe("claude"));
  });

  it("keeps unrelated typing while a runner switch is in flight", async () => {
    let finishSwitch!: () => void;
    stubAgents({ switchRunner: body => new Promise(resolve => {
      finishSwitch = () => resolve({ ...draftGolden(), agent: { ...body.agent, runner: body.runner, model: "" } });
    }) });
    renderAgents();
    const runner = (await screen.findAllByRole("combobox", { name: "Harness" }))[0] as HTMLSelectElement;
    await waitFor(() => expect(runner.disabled).toBe(false));
    fireEvent.change(runner, { target: { value: "codex" } });
    await waitFor(() => expect(finishSwitch).toBeTypeOf("function"));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Newer typed name" } });
    await act(async () => finishSwitch());
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Newer typed name");
    expect(runner.value).toBe("codex");
  });

  it("does not recreate an old draft when a runner reply arrives after a newer edit was saved", async () => {
    const user = userEvent.setup();
    const list = listGolden();
    let finishSwitch!: () => void;
    stubAgents({ list, switchRunner: body => new Promise(resolve => {
      finishSwitch = () => resolve({ ...draftGolden(), agent: { ...body.agent, runner: body.runner, model: "" } });
    }), save: body => {
      const saved = { ...body.agent, version: Number(body.agent.version) + 1 } as Record_["agent"];
      const record = list.agents.find(record => record.agent.id === saved.id)!;
      record.agent = saved;
      return Promise.resolve(json({ ...record, glyph: record.glyph ?? "funnel" }));
    } });
    renderAgents();
    const runner = (await screen.findAllByRole("combobox", { name: "Harness" }))[0] as HTMLSelectElement;
    await waitFor(() => expect(runner.disabled).toBe(false));
    fireEvent.change(runner, { target: { value: "codex" } });
    await waitFor(() => expect(finishSwitch).toBeTypeOf("function"));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Saved newer name" } });
    const save = await screen.findByRole("button", { name: "Save" });
    await waitFor(() => expect(save.hasAttribute("disabled")).toBe(false));
    await user.click(save);
    await waitFor(() => expect(screen.queryByText(/Unsaved changes/)).toBeNull());
    await act(async () => finishSwitch());
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Saved newer name");
    expect(runner.value).toBe("pi");
    expect(screen.queryByText(/Unsaved changes/)).toBeNull();
  });

  it("edits every saved field and posts the exact API agent shape with ordered outcomes and conditional Then values", async () => {
    const user = userEvent.setup();
    const initial = listGolden().agents.find(record => record.agent.id === "triage")!.agent as Record<string, unknown>;
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await screen.findByLabelText("Name");
    const change = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });
    change("Name", "Triage edited");
    change("Workspace", "worktreeReadOnly");
    change("Runs at once", "4");
    change("Colour", "violet");
    change("Brief", "A deliberately long edited brief that stays in the textarea.");
    fireEvent.change(screen.getAllByLabelText("Model")[0]!, { target: { value: "fixture-model" } });
    fireEvent.change(screen.getAllByLabelText("Thinking")[0]!, { target: { value: "max" } });

    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    fireEvent.change(screen.getAllByLabelText("Model")[1]!, { target: { value: "opus-5.5-1m" } });
    fireEvent.change(screen.getAllByLabelText("Thinking")[1]!, { target: { value: "high" } });
    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    await user.click(screen.getByRole("button", { name: "Remove alternate 2" }));

    const setOutcome = (index: number, field: string, value: string) => change(`Outcome ${index + 1} ${field}`, value);
    setOutcome(0, "name", "route to build");
    setOutcome(0, "when", "The work needs a build.");
    change("Outcome 1 tone", "problem");
    change("Outcome 1 then", "goto");
    await user.click(screen.getByRole("button", { name: "Outcome 1 agents" }));
    await user.click(screen.getByLabelText("Build"));
    const outcomeOne = screen.getByLabelText("Outcome 1 then").closest("tr") as HTMLElement;
    await user.click(within(outcomeOne).getByLabelText("pass the output along"));
    change("Outcome 1 at most", "3");
    setOutcome(1, "name", "retry this");
    change("Outcome 2 then", "retry");
    change("Outcome 2 at most", "2");
    change("Outcome 3 then", "hold");
    await user.click(screen.getByLabelText("comment on the issue or ticket"));
    change("Outcome 4 then", "next");
    const outcomeFour = screen.getByLabelText("Outcome 4 then").closest("tr") as HTMLElement;
    await user.click(within(outcomeFour).getByLabelText("pass the output along"));
    await user.click(screen.getByRole("button", { name: "+ Add outcome" }));
    setOutcome(4, "name", "temporary last");
    setOutcome(4, "when", "Temporary outcome.");
    await user.click(screen.getByRole("button", { name: "Remove outcome 5" }));
    // Re-add so the final outcome order and the add/remove controls are both covered.
    await user.click(screen.getByRole("button", { name: "+ Add outcome" }));
    setOutcome(4, "name", "ordered last");
    setOutcome(4, "when", "After everything else.");

    const expected = structuredClone(initial);
    Object.assign(expected, {
      name: "Triage edited", workspace: "worktreeReadOnly", slots: 4, model: "fixture-model", thinking: "max",
      tint: "violet", brief: "A deliberately long edited brief that stays in the textarea.",
      alternates: [{ runner: "pi", model: "opus-5.5-1m", thinking: "high" }],
      outcomes: [
        { name: "route to build", when: "The work needs a build.", tone: "problem", then: { do: "goto", withOutput: true, max: 3 }, to: ["build"] },
        { ...((initial.outcomes as Array<Record<string, unknown>>)[1]!), name: "retry this", then: { do: "retry", max: 2 } },
        { ...((initial.outcomes as Array<Record<string, unknown>>)[2]!), then: { do: "hold", comment: false } },
        { ...((initial.outcomes as Array<Record<string, unknown>>)[3]!), then: { do: "next", withOutput: true } },
        { name: "ordered last", when: "After everything else.", tone: "neutral", then: { do: "hold" }, to: [] }
      ]
    });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/save")).toBe(true));
    const request = posts(requests).find(item => item.path === "/api/v1/agents/save")!;
    expect(Object.keys(request.body!).sort()).toEqual(["agent"]);
    expect(request.body).toEqual({ agent: expected });
    expect((screen.getByLabelText("Id") as HTMLOutputElement).textContent).toBe("triage");
  }, 20_000);

  it("posts the exact {agent} body and shows the returned saved version", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    // Select triage so the saved record is known.
    await screen.findByLabelText("Brief");
    await screen.findByLabelText("Name");
    // Edit Name through a single change so the draft is dirty.
    const name = screen.getByLabelText("Name") as HTMLInputElement;
    fireEvent.change(name, { target: { value: "Triage revised" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/save")).toBe(true));
    const saved = posts(requests).find(request => request.path === "/api/v1/agents/save")!;
    const agent = saved.body!.agent as Record<string, unknown>;
    expect(agent.id).toBe("triage");
    expect(agent.name).toBe("Triage revised");
    expect(agent.key).toBe("triage");
    expect(Array.isArray(agent.outcomes)).toBe(true);
    expect(Array.isArray(agent.alternates)).toBe(true);
    // The returned version is shown once the list refetches.
    await waitFor(() => expect(screen.queryByText(/Unsaved changes/)).toBeNull());
    // The save succeeded as a whole: no contract error, and Save's click event was not
    // taken for the save-and-open callback (calling it threw after the save).
    expect(screen.queryByText(/did not match its API contract/)).toBeNull();
    expect(screen.queryByText(/is not a function/)).toBeNull();
  });

  it("saves the icon and colour the editor shows, and clears the icon back to the default", async () => {
    const user = userEvent.setup();
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    await screen.findByLabelText("Icon");
    fireEvent.change(screen.getByLabelText("Icon"), { target: { value: "flag" } });
    fireEvent.change(screen.getByLabelText("Colour"), { target: { value: "teal" } });
    expect(screen.getByLabelText("Icon").closest(".agent-field")?.textContent).toContain("Flag");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/save")).toBe(true));
    expect((posts(requests).find(request => request.path === "/api/v1/agents/save")!.body!.agent as Record<string, unknown>).icon).toBe("flag");
    await waitFor(() => expect(screen.queryByText(/Unsaved changes/)).toBeNull());
    fireEvent.change(screen.getByLabelText("Icon"), { target: { value: "" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).filter(request => request.path === "/api/v1/agents/save")).toHaveLength(2));
    expect((posts(requests).filter(request => request.path === "/api/v1/agents/save")[1]!.body!.agent as Record<string, unknown>).icon).toBeUndefined();
  });

  it("shows a rejected save's ordered 422 problems inline, preserves the draft, then saves after a correction", async () => {
    const user = userEvent.setup();
    let attempts = 0;
    const { requests } = stubAgents({
      save: body => {
        attempts += 1;
        if (attempts === 1) return json({ message: "Fix these first.", problems: ["Give the agent a name.", "Write the agent's brief."] }, 422);
        return json({ agent: { ...body.agent, version: Number(body.agent.version) + 1 }, archived: false, glyph: body.agent.icon ?? "chip", detail: `v${Number(body.agent.version) + 1}` });
      }
    });
    renderAgents();
    await screen.findByLabelText("Brief");
    const name = await screen.findByLabelText("Name") as HTMLInputElement;
    fireEvent.change(name, { target: { value: "Risky" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Fix these first.")).toBeTruthy();
    expect(within(alert).getAllByRole("listitem").map(item => item.textContent)).toEqual(["Give the agent a name.", "Write the agent's brief."]);
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("Risky");
    // Correct and save again.
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Risky fixed" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).filter(request => request.path === "/api/v1/agents/save")).toHaveLength(2));
    await waitFor(() => expect(screen.queryByText(/Unsaved changes/)).toBeNull());
    expect(screen.queryByText(/did not match its API contract/)).toBeNull();
  });

  it("discards a new agent draft and returns to the browser", async () => {
    const user = userEvent.setup();
    stubAgents();
    renderAgents(undefined, "/agents");
    await user.click(await screen.findByRole("button", { name: /New agent/ }));
    await screen.findByLabelText("Id");
    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Agents" })).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Unsent drafts" })).toBeNull();
  });
});

describe("model and thinking choices", () => {
  it("shows loading, failed, and empty API-choice states explicitly", async () => {
    const user = userEvent.setup();
    let finish!: (response: Response) => void;
    stubAgents({ modelResponse: runner => runner === "pi"
      ? new Promise<Response>(resolve => { finish = resolve; })
      : json(golden("models.json")) });
    renderAgents();
    await screen.findByLabelText("Brief");
    expect(await screen.findByText("Loading models…")).toBeTruthy();

    // An alternate added while Pi is still loading shows the same loading state, with both controls disabled.
    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    const alternateRow = () => screen.getAllByLabelText("Harness")[1]!.closest(".agent-form-row") as HTMLElement;
    expect(within(alternateRow()).getByText("Loading models…")).toBeTruthy();
    expect(within(alternateRow()).getByText("Loading thinking choices…")).toBeTruthy();
    expect((within(alternateRow()).getByLabelText("Model") as HTMLSelectElement).disabled).toBe(true);
    expect((within(alternateRow()).getByLabelText("Thinking") as HTMLSelectElement).disabled).toBe(true);
    expect((within(alternateRow()).getByLabelText("Harness") as HTMLSelectElement).disabled).toBe(false);

    finish(json({
      runner: "pi", executable: "pi", catalog: null, usable: true, loading: false, error: null,
      allowedModels: [], choices: [], unavailableModels: [], thinking: [], thinkingNote: null, listing: [],
      actions: { refresh: { available: true, refusalReason: null }, candidates: [] }
    }));
    expect(await screen.findAllByText("No models reported for Pi.")).toHaveLength(2);
    expect(screen.getAllByText("No thinking choices reported for Pi.")).toHaveLength(2);
    expect(within(alternateRow()).queryByText("Loading models…")).toBeNull();
    expect((within(alternateRow()).getByLabelText("Model") as HTMLSelectElement).disabled).toBe(true);
    expect((within(alternateRow()).getByLabelText("Thinking") as HTMLSelectElement).disabled).toBe(true);

    // Moving the alternate to a runner with choices enables its controls; the empty primary row stays empty.
    fireEvent.change(screen.getAllByLabelText("Harness")[1]!, { target: { value: "cursor" } });
    await waitFor(() => expect((screen.getAllByLabelText("Harness")[1] as HTMLSelectElement).value).toBe("cursor"));
    await waitFor(() => expect((screen.getAllByLabelText("Thinking")[1] as HTMLSelectElement).options.length).toBeGreaterThan(1));
    expect(within(alternateRow()).queryByText(/No models reported/)).toBeNull();
    expect(within(alternateRow()).queryByText("Loading models…")).toBeNull();
    expect((within(alternateRow()).getByLabelText("Model") as HTMLSelectElement).disabled).toBe(false);
    expect((within(alternateRow()).getByLabelText("Thinking") as HTMLSelectElement).disabled).toBe(false);
    expect(screen.getAllByText("No models reported for Pi.")).toHaveLength(1);
  });

  it("uses restricted primary and alternate API choices, retains opaque ids, and offers the API default", async () => {
    const user = userEvent.setup();
    const opaqueId = "vendor/runner:model@rev-7";
    const onlyChoice = {
      ...golden<Record<string, unknown>>("models.json"),
      choices: [{ id: opaqueId, name: "Only offered model" }],
      allowedModels: [opaqueId],
      thinking: [{ id: "low", label: "Low (API)" }]
    };
    const alternateId = "cursor/provider:model-v2";
    const alternateChoices = { ...onlyChoice, runner: "cursor", executable: "cursor", choices: [{ id: alternateId, name: "Cursor-only model" }], allowedModels: [alternateId], thinking: [{ id: "high", label: "High (Cursor API)" }] };
    const { requests } = stubAgents({ models: runner => runner === "pi" ? onlyChoice : alternateChoices });
    renderAgents();
    await screen.findByLabelText("Brief");
    const primaryModel = await screen.findByLabelText("Model") as HTMLSelectElement;
    await waitFor(() => expect(Array.from(primaryModel.options).map(option => option.value)).toEqual(["openrouter/stealth/space-bunny-alpha", opaqueId]));
    expect(Array.from(primaryModel.options).filter(option => !option.disabled).map(option => option.value)).toEqual([opaqueId]);
    expect(screen.getAllByText(/not allowed/).length).toBeGreaterThan(0);
    expect(Array.from((screen.getByLabelText("Thinking") as HTMLSelectElement).options).map(option => option.textContent)).toEqual(["The runner's default", "Low (API)"]);
    fireEvent.change(primaryModel, { target: { value: opaqueId } });
    fireEvent.change(screen.getByLabelText("Thinking"), { target: { value: "" } });

    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    fireEvent.change(screen.getAllByLabelText("Harness")[1]!, { target: { value: "cursor" } });
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/draft/switch-runner" && request.body?.alternateIndex === 0)).toBe(true));
    const alternateModel = screen.getAllByLabelText("Model")[1] as HTMLSelectElement;
    await waitFor(() => expect(Array.from(alternateModel.options).some(option => option.value === alternateId)).toBe(true));
    expect(Array.from(alternateModel.options).filter(option => !option.disabled).map(option => option.value)).toEqual([alternateId]);
    fireEvent.change(alternateModel, { target: { value: alternateId } });
    fireEvent.change(screen.getAllByLabelText("Thinking")[1]!, { target: { value: "" } });
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/save")).toBe(true));
    const body = posts(requests).find(request => request.path === "/api/v1/agents/save")!.body!;
    const saved = body.agent as Record<string, unknown>;
    expect(saved.model).toBe(opaqueId);
    expect(saved).not.toHaveProperty("thinking");
    expect(saved.alternates).toEqual([{ runner: "cursor", model: alternateId }]);
  });

  it("offers exactly the daemon's thinking choices and normalizes a runner switch through the daemon", async () => {
    const { requests } = stubAgents();
    renderAgents();
    await screen.findByLabelText("Brief");
    const thinking = await screen.findByLabelText("Thinking") as HTMLSelectElement;
    await waitFor(() => expect(within(thinking).getAllByRole("option").length).toBeGreaterThan(1));
    const labels = Array.from(thinking.options).map(option => option.textContent);
    expect(labels).toContain("Extra high");
    expect(labels).toContain("Max");
    fireEvent.change(screen.getByLabelText("Harness"), { target: { value: "copilot" } });
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/draft/switch-runner")).toBe(true));
    const switched = posts(requests).find(request => request.path === "/api/v1/agents/draft/switch-runner")!;
    expect((switched.body!.runner as string)).toBe("copilot");
    await waitFor(() => expect((screen.getByLabelText("Harness") as HTMLSelectElement).value).toBe("copilot"));
  });

  it("keeps a saved selection no longer offered visible with an explanation", async () => {
    const cursorModel = "cursor/provider:model-v2";
    const cursorModels = { ...golden<Record<string, unknown>>("models.json"), runner: "cursor", executable: "cursor", choices: [{ id: cursorModel, name: "Cursor-only model" }], allowedModels: [cursorModel], thinking: [{ id: "high", label: "High (Cursor API)" }] };
    stubAgents({
      list: { ...listGolden(), agents: listGolden().agents.map(record => record.agent.id === "triage"
        ? { ...record, agent: { ...record.agent, thinking: "max", alternates: [
          { runner: "pi", model: "retired-model", thinking: "max" },
          { runner: "cursor", model: cursorModel, thinking: "high" }
        ] } }
        : record) },
      models: runner => runner === "pi" ? { ...golden<Record<string, unknown>>("models.json"), thinking: [{ id: "low", label: "Low" }] } : cursorModels
    });
    renderAgents();
    await screen.findByLabelText("Brief");
    const thinking = await screen.findAllByLabelText("Thinking") as HTMLSelectElement[];
    await waitFor(() => expect(thinking[2]!.options.length).toBeGreaterThan(1));
    // The stale Pi alternate keeps its saved values and explains them beside its own controls.
    const stale = thinking[1]!;
    const staleRow = stale.closest(".agent-form-row") as HTMLElement;
    expect(stale.value).toBe("max");
    expect(Array.from(stale.options).some(option => option.disabled && option.value === "max")).toBe(true);
    expect(within(staleRow).getByText(/This level is no longer offered/)).toBeTruthy();
    // A model the runner no longer offers stays selected, shown disabled with its reason, and the Model control stays usable.
    const staleModel = within(staleRow).getByLabelText("Model") as HTMLSelectElement;
    expect(staleModel.value).toBe("retired-model");
    expect(Array.from(staleModel.options).some(option => option.disabled && option.value === "retired-model" && /not allowed/.test(option.textContent ?? ""))).toBe(true);
    expect(staleModel.disabled).toBe(false);
    expect(staleRow.querySelector(".model-stale")!.textContent).toBe("not allowed");
    // The healthy Cursor row is unaffected by the Pi row's stale selection.
    const cursorRow = thinking[2]!.closest(".agent-form-row") as HTMLElement;
    expect(thinking[2]!.value).toBe("high");
    expect(thinking[2]!.disabled).toBe(false);
    expect(within(cursorRow).queryByText(/This level is no longer offered/)).toBeNull();
    expect((within(cursorRow).getByLabelText("Model") as HTMLSelectElement).value).toBe(cursorModel);
    // The primary row's saved "max" is stale too, and keeps its value beside its own explanation.
    expect(thinking[0]!.value).toBe("max");
    expect(screen.getAllByText(/This level is no longer offered/)).toHaveLength(2);
  });

  it("keeps the surviving alternates' settings and their order when one is removed before saving", async () => {
    const user = userEvent.setup();
    const cursorModel = "cursor/provider:model-v2";
    const piModels = golden<Record<string, unknown>>("models.json");
    const cursorModels = { ...piModels, runner: "cursor", executable: "cursor", choices: [{ id: cursorModel, name: "Cursor-only model" }], allowedModels: [cursorModel], thinking: [{ id: "high", label: "High (Cursor API)" }] };
    const { requests } = stubAgents({ models: runner => runner === "pi" ? piModels : cursorModels });
    renderAgents();
    await screen.findByLabelText("Brief");
    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    await user.click(screen.getByRole("button", { name: "+ Add a model" }));
    fireEvent.change(screen.getAllByLabelText("Harness")[1]!, { target: { value: "cursor" } });
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/draft/switch-runner" && request.body?.alternateIndex === 0)).toBe(true));
    await waitFor(() => expect(Array.from((screen.getAllByLabelText("Model")[1] as HTMLSelectElement).options).some(option => option.value === cursorModel)).toBe(true));
    fireEvent.change(screen.getAllByLabelText("Model")[1]!, { target: { value: cursorModel } });
    fireEvent.change(screen.getAllByLabelText("Thinking")[1]!, { target: { value: "high" } });
    fireEvent.change(screen.getAllByLabelText("Model")[2]!, { target: { value: "opus-5.5-1m" } });
    fireEvent.change(screen.getAllByLabelText("Thinking")[2]!, { target: { value: "max" } });
    fireEvent.change(screen.getAllByLabelText("Model")[3]!, { target: { value: "fixture-model" } });
    await user.click(screen.getByRole("button", { name: "Remove alternate 2" }));
    expect(screen.getAllByLabelText("Model")).toHaveLength(3);

    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(posts(requests).some(request => request.path === "/api/v1/agents/save")).toBe(true));
    const saved = posts(requests).find(request => request.path === "/api/v1/agents/save")!.body!.agent as Record<string, unknown>;
    expect(saved.model).toBe("openrouter/stealth/space-bunny-alpha");
    expect(saved.alternates).toEqual([
      { runner: "cursor", model: cursorModel, thinking: "high" },
      { runner: "pi", model: "fixture-model" }
    ]);
  });
});
