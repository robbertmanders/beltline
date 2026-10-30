import { useState } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/errors";
import { emptyStageDraft, type StageDraft } from "../../state/runDrafts";
import type { CommandController } from "../../api/useCommand";
import type { PromptFacts, RunViewResponse, SourceSharing } from "../../api/wire.gen";
import { RunControls, type RunCommandSubmit } from "./RunControls";
import { RunInlineControls } from "./RunInlineControls";
import { golden } from "../../test/golden";

function controller(overrides: Partial<CommandController> = {}): CommandController {
  return { submit: vi.fn(() => true), retry: vi.fn(), clearError: vi.fn(), pending: false, error: null, lastRequest: null, ...overrides };
}

function Harness({ view, stageId, command, submit = vi.fn(), adjustOpen }: { view: RunViewResponse; stageId: string; command: CommandController; submit?: RunCommandSubmit; adjustOpen?: boolean }) {
  const [draft, setDraft] = useState<StageDraft>({ ...emptyStageDraft });
  const [open, setOpen] = useState(false);
  const panel = view.stages.find(item => item.id === stageId)!;
  const stage = view.run.stages.find(item => item.id === stageId)!;
  const props = {
    runId: view.run.id, run: view.run, view, stage, panel, selectedAttemptId: panel.defaultAttempt ?? null, draft,
    onAnswerChange: (value: string) => setDraft(current => ({ ...current, answer: value })),
    onDecisionNoteChange: (outcomeId: string, value: string) => setDraft(current => ({ ...current, notes: { ...current.notes, [outcomeId]: value } })),
    onPendingOutcomeChange: (value: string | null) => setDraft(current => ({ ...current, pendingOutcome: value })),
    onPromptChange: (value: string) => setDraft(current => ({ ...current, prompt: value })),
    onSourceChange: (value: SourceSharing) => setDraft(current => ({ ...current, source: value })),
    onSendMessageChange: (value: string) => setDraft(current => ({ ...current, sendMessage: value })),
    answerVersion: () => 0, command, submit, canRead: true, connection: "connected" as const, onErased: vi.fn(),
    adjustOpen
  };
  return <>
    <RunControls {...props} open={open} onOpenChange={setOpen} />
    <RunInlineControls {...props} />
  </>;
}

/** The daemon's view of a run held at its Merge stage (`watch`). */
function heldView(): RunViewResponse {
  return golden<RunViewResponse>("views-run-held.json");
}

/** views-run.json's run as if held at its ticket watch, for the inline answer box (not the menu, which the daemon builds for its state). */
function heldAtTicketView(): RunViewResponse {
  const view = golden<RunViewResponse>("views-run.json");
  view.run.state = "held";
  return view;
}
function gateView(): RunViewResponse {
  const view = golden<RunViewResponse>("views-run.json");
  view.run.state = "waiting";
  view.run.stageId = "approve-plan";
  const decisions = view.run.stages.find(stage => stage.id === "approve-plan")?.outcomes ?? [];
  view.actions = [
    ...view.actions.filter(action => action.id !== "decide"),
    // The daemon says which outcome asks for a note and what the note is for, as its decide actions do.
    ...decisions.map(outcome => outcome.name === "sent back"
      ? { id: "decide", enabled: true, stage: "approve-plan", outcome: outcome.name, needsNote: true, noteLabel: "Note for Plan the epic", thenHelp: "Then: Back to Plan the epic with the output" }
      : { id: "decide", enabled: true, stage: "approve-plan", outcome: outcome.name, needsNote: false, noteLabel: "Note for the next run", thenHelp: `Then: ${outcome.name}` })
  ];
  return view;
}

/** gateView's approval as a prompt stage, as the daemon sends one: its editor in place of the document. */
function promptView(prompt: Partial<PromptFacts> = {}): RunViewResponse {
  const view = gateView();
  const panel = view.stages.find(stage => stage.id === "approve-plan")!;
  const approval = panel.facts!.approval!;
  approval.outcomes = [
    { id: "started", name: "started", label: "Start", tone: "good" },
    { id: "parked", name: "parked", label: "Park it", tone: "neutral" }
  ];
  approval.primary = "started";
  approval.prompt = {
    label: "Your prompt", placeholder: "Describe your task.", help: "Write the run's task.",
    contextLabel: "Source context", context: [{ type: "paragraph", text: "Ticket acceptance criteria" }],
    sourceLabel: "Include source context for agents", sourceHelp: "Unchecked, agents get the reference only.", source: "withheld",
    ...prompt
  };
  view.actions = [
    ...view.actions.filter(action => action.id !== "decide"),
    { id: "decide", enabled: true, stage: "approve-plan", outcome: "started", needsNote: true },
    { id: "decide", enabled: true, stage: "approve-plan", outcome: "parked", needsNote: false }
  ];
  return view;
}

describe("prompt stage", () => {
  it("sends the prompt and the source choice with an outcome that moves on, and parks without them", async () => {
    const user = userEvent.setup();
    const submit = vi.fn<RunCommandSubmit>();
    render(<Harness view={promptView()} stageId="approve-plan" command={controller()} submit={submit} />);
    const start = screen.getByRole("button", { name: "Start" });
    expect(start.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("Ticket acceptance criteria")).toBeTruthy();
    const source = screen.getByRole("checkbox", { name: "Include source context for agents" }) as HTMLInputElement;
    expect(source.checked).toBe(false);
    await user.click(screen.getByRole("button", { name: "Park it" }));
    expect(submit).toHaveBeenLastCalledWith("decide", { outcome: "parked", note: "", stage: "approve-plan" });
    await user.type(screen.getByRole("textbox", { name: "Your prompt" }), "My task\nwith constraints");
    await user.click(start);
    expect(submit).toHaveBeenLastCalledWith("decide", { outcome: "started", note: "My task\nwith constraints", stage: "approve-plan", source: "withheld" });
    await user.click(source);
    await user.click(start);
    expect(submit).toHaveBeenLastCalledWith("decide", { outcome: "started", note: "My task\nwith constraints", stage: "approve-plan", source: "shared" });
  });

  it("starts a visit back from the run's task and choice, and keeps the draft after a refusal", async () => {
    const user = userEvent.setup();
    const view = promptView({ task: "Earlier task", source: "shared" });
    const submit = vi.fn<RunCommandSubmit>();
    const { rerender } = render(<Harness view={view} stageId="approve-plan" command={controller()} submit={submit} />);
    const text = screen.getByRole("textbox", { name: "Your prompt" }) as HTMLTextAreaElement;
    expect(text.value).toBe("Earlier task");
    expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(true);
    await user.type(text, " and more");
    await user.click(screen.getByRole("button", { name: "Start" }));
    rerender(<Harness view={view} stageId="approve-plan" submit={submit} command={controller({
      error: new ApiError("Decision refused", 422), lastRequest: { path: "/api/v1/runs/run/decide", body: {}, affects: [], publish: vi.fn() }
    })} />);
    expect((screen.getByRole("textbox", { name: "Your prompt" }) as HTMLTextAreaElement).value).toBe("Earlier task and more");
    expect(screen.getByText("Decision refused")).toBeTruthy();
  });
});

describe("run controls", () => {
  it("waits for the selected stage's settings instead of retaining an earlier stage's defaults", async () => {
    const view = golden<RunViewResponse>("views-run-forked.json");
    const agent = view.stages.find(item => item.adjust)!;
    const help = agent.adjust!;
    const command = controller();
    const submit = vi.fn<RunCommandSubmit>();
    const { rerender } = render(<Harness view={view} stageId="watch" command={command} />);
    const loading = structuredClone(view);
    delete loading.stages.find(item => item.id === agent.id)!.adjust;
    rerender(<Harness view={loading} stageId={agent.id} command={command} submit={submit} adjustOpen />);
    expect(screen.getByText("Loading stage settings…")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Apply" })).toBeNull();
    expect(screen.queryByLabelText("Brief")).toBeNull();
    view.actions = [...view.actions.filter(action => action.id !== "adjust-stage"), { id: "adjust-stage", enabled: true, stage: agent.id }];
    rerender(<Harness view={view} stageId={agent.id} command={command} submit={submit} adjustOpen />);
    expect((screen.getByLabelText("Brief") as HTMLTextAreaElement).value).toBe(help.brief);
    expect((screen.getByLabelText("Harness") as HTMLSelectElement).value).toBe(help.runner);
    expect((screen.getByLabelText("Effort") as HTMLSelectElement).value).toBe(help.thinking ?? "");
    await userEvent.setup().click(screen.getByRole("button", { name: "Apply" }));
    expect(submit).toHaveBeenCalledWith("adjust-stage", {
      stage: agent.id, runner: help.runner, model: help.model, thinking: help.thinking || undefined, brief: help.brief
    }, expect.any(Function));
  });

  it("isolates adjustments by stage and run, preserves typing during refresh, and reloads saved settings on reopening", async () => {
    const user = userEvent.setup();
    const view = golden<RunViewResponse>("views-run-forked.json");
    const first = view.stages.find(item => item.adjust)!;
    const second = view.stages.find(item => item.id === "triage")!;
    second.adjust = { ...first.adjust!, runner: "copilot", model: "second-model", thinking: "high", brief: "Second stage brief" };
    const command = controller();
    const { rerender } = render(<Harness view={view} stageId={first.id} command={command} adjustOpen />);
    rerender(<Harness view={view} stageId={second.id} command={command} adjustOpen />);
    expect((screen.getByLabelText("Brief") as HTMLTextAreaElement).value).toBe("Second stage brief");
    expect((screen.getByLabelText("Harness") as HTMLSelectElement).value).toBe("copilot");
    expect((screen.getByLabelText("Effort") as HTMLSelectElement).value).toBe("high");
    await user.clear(screen.getByLabelText("Brief"));
    await user.type(screen.getByLabelText("Brief"), "Unsent edits");
    const refreshed = structuredClone(view);
    refreshed.stages.find(item => item.id === second.id)!.adjust!.brief = "Updated saved brief";
    rerender(<Harness view={refreshed} stageId={second.id} command={command} adjustOpen />);
    expect((screen.getByLabelText("Brief") as HTMLTextAreaElement).value).toBe("Unsent edits");
    rerender(<Harness view={refreshed} stageId={second.id} command={command} adjustOpen={false} />);
    rerender(<Harness view={refreshed} stageId={second.id} command={command} adjustOpen />);
    expect((screen.getByLabelText("Brief") as HTMLTextAreaElement).value).toBe("Updated saved brief");
    const otherRun = structuredClone(refreshed);
    otherRun.run.id = "another-run";
    otherRun.stages.find(item => item.id === second.id)!.adjust!.brief = "Other run brief";
    rerender(<Harness view={otherRun} stageId={second.id} command={command} adjustOpen />);
    expect((screen.getByLabelText("Brief") as HTMLTextAreaElement).value).toBe("Other run brief");
  });

  it("adjusts a stage's brief beside its placeholders and can restore the agent's brief", async () => {
    const user = userEvent.setup();
    // The run page sends Adjust help for the stage it shows; the forked run is one whose current stage is an agent.
    const view = golden<RunViewResponse>("views-run-forked.json");
    const agentStage = view.stages.find(item => item.adjust)!;
    const adjust = agentStage.adjust!;
    view.actions = [...view.actions.filter(action => action.id !== "adjust-stage"), { id: "adjust-stage", enabled: true, stage: agentStage.id }];
    // The stage panel's Adjust button opens the sheet, as in the app.
    render(<Harness view={view} stageId={agentStage.id} command={controller()} adjustOpen />);
    const sheet = await screen.findByRole("dialog", { name: "Adjust stage" });
    expect(within(sheet).getByLabelText("Harness")).toBeTruthy();
    expect(within(sheet).queryByLabelText("Runner")).toBeNull();
    const brief = within(sheet).getByLabelText("Brief") as HTMLTextAreaElement;
    await user.clear(brief);
    await user.type(brief, "New brief");
    expect(brief.value).toBe("New brief");
    const placeholders = within(sheet).getByRole("region", { name: "Placeholders" });
    for (const entry of adjust.placeholders.entries) expect(within(placeholders).getByText(`{${entry.name}}`)).toBeTruthy();
    expect(within(placeholders).getByText(adjust.note)).toBeTruthy();
    await user.click(within(sheet).getByRole("button", { name: "Use the agent's brief" }));
    expect(brief.value).toBe(adjust.agentBrief);
  });

  it("leaves overruling an AFK decision to the AFK page, as the Mac app did", async () => {
    const user = userEvent.setup();
    const view = golden<RunViewResponse>("views-run.json");
    view.actions = [...view.actions, { id: "overrule-afk", enabled: true, decision: "decision-1" }];
    render(<Harness view={view} stageId={view.run.stageId!} command={controller()} submit={vi.fn<RunCommandSubmit>()} />);
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    expect(screen.queryByRole("button", { name: /Overrule/ })).toBeNull();
  });

  it("decides an outcome without a note at once, and keeps the note of one that asks for it", async () => {
    const user = userEvent.setup();
    const submit = vi.fn();
    render(<Harness view={gateView()} stageId="approve-plan" command={controller()} submit={submit} />);
    await user.click(screen.getByRole("button", { name: "Approve" }));
    expect(submit).toHaveBeenCalledWith("decide", { outcome: "approved", note: "", stage: "approve-plan" });
    await user.click(screen.getByRole("button", { name: "Send back with a note" }));
    const note = await screen.findByLabelText(/Note for Plan the epic/);
    await user.type(note, "tighten the scope");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Send back with a note" }));
    expect((await screen.findByLabelText(/Note for Plan the epic/) as HTMLTextAreaElement).value).toBe("tighten the scope");
  });

  it("preserves the Send to message when the destination changes", async () => {
    const user = userEvent.setup();
    render(<Harness view={heldView()} stageId="watch" command={controller()} />);
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    await user.click(screen.getByRole("button", { name: "Send to…" }));
    const message = await screen.findByLabelText("Message (optional)");
    await user.type(message, "please carry on");
    const destination = screen.getByLabelText("Stage") as HTMLSelectElement;
    const before = destination.value;
    const other = Array.from(destination.options).find(option => option.value !== before)!.value;
    await user.selectOptions(destination, other);
    expect((screen.getByLabelText("Message (optional)") as HTMLTextAreaElement).value).toBe("please carry on");
  });

  it("offers Run again separately from Send to and targets the current stage", async () => {
    const user = userEvent.setup();
    render(<Harness view={heldView()} stageId="watch" command={controller()} />);
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    await user.click(screen.getByRole("button", { name: "Run Merge again" }));
    // As in the app, running the held stage again names it and offers no other stage.
    const sheet = await screen.findByRole("dialog", { name: "Merge" });
    expect(within(sheet).queryByLabelText("Stage")).toBeNull();
    expect(within(sheet).getByText("Run this stage again.")).toBeTruthy();
  });

  it("keeps the Send to sheet open and shows every problem when the daemon refuses it", async () => {
    const user = userEvent.setup();
    const view = heldView();
    const submit = vi.fn();
    const { rerender } = render(<Harness view={view} stageId="watch" command={controller()} submit={submit} />);
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    await user.click(screen.getByRole("button", { name: "Send to…" }));
    await user.type(await screen.findByLabelText("Message (optional)"), "please carry on");
    await user.click(screen.getByRole("button", { name: "Send" }));
    expect(submit).toHaveBeenCalledTimes(1);
    expect(submit.mock.calls[0]?.[0]).toBe("resume");
    // Nothing accepted it yet, so the form stays open with the entered text.
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByLabelText("Message (optional)")).toBeTruthy();
    rerender(<Harness view={view} stageId="watch" submit={submit}
      command={controller({ error: new ApiError("Continue refused", 422, ["holds are not answerable", "stage changed"]) })} />);
    const refused = screen.getByRole("dialog");
    expect(within(refused).getByText("Continue refused")).toBeTruthy();
    expect(within(refused).getAllByRole("listitem").map(item => item.textContent)).toEqual(["holds are not answerable", "stage changed"]);
    expect((within(refused).getByLabelText("Message (optional)") as HTMLTextAreaElement).value).toBe("please carry on");
  });

  it("keeps the confirmation open until the daemon accepts the command", async () => {
    const user = userEvent.setup();
    const view = heldView();
    const submit = vi.fn();
    render(<Harness view={view} stageId="watch" command={controller()} submit={submit} />);
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    await user.click(within(screen.getByLabelText("Run actions menu")).getByRole("button", { name: "Drop…" }));
    const dialog = await screen.findByRole("dialog", { name: `Drop ${view.reference}?` });
    await user.click(within(dialog).getByRole("button", { name: "Drop the run" }));
    expect(submit.mock.calls[0]?.[0]).toBe("drop");
    expect(screen.getByRole("dialog", { name: `Drop ${view.reference}?` })).toBeTruthy();
  });

  it("draws the daemon's menu rows, each enabled from its own record", async () => {
    const user = userEvent.setup();
    render(<Harness view={golden<RunViewResponse>("views-run.json")} stageId="watch" command={controller()} />);
    await user.click(screen.getByRole("button", { name: "Run actions" }));
    const menu = within(screen.getByLabelText("Run actions menu"));
    expect(menu.getAllByRole("button").map(button => button.textContent)).toEqual(["Pause", "Park", "Send to…", "Abort…", "Drop…"]);
    expect((menu.getByRole("button", { name: "Send to…" }) as HTMLButtonElement).disabled).toBe(false);
    expect((menu.getByRole("button", { name: "Drop…" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("shows an inline answer refusal beside the answer box", async () => {
    const user = userEvent.setup();
    const view = heldAtTicketView();
    view.actions.push({ id: "answer", enabled: true, stage: "watch", attempt: "watch-1" });
    const refused = controller({
      error: new ApiError("Answer refused", 422, ["answer is required"]),
      lastRequest: { path: "/api/v1/runs/epics%3Ajira%3APAY-40/answer", body: {}, affects: [], publish: () => {} }
    });
    render(<Harness view={view} stageId="watch" command={refused} />);
    await user.type(screen.getByLabelText("Answer Watch the ticket"), "keep this");
    const alert = screen.getByRole("alert");
    expect(within(alert).getByText("Answer refused")).toBeTruthy();
    expect(screen.getByLabelText("Answer Watch the ticket")).toBeTruthy();
  });

});
