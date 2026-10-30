import { findRunAction } from "../../api/runCommands";
import type { RunAction } from "../../api/wire.gen";
import { CommandFailure, isInlineCommand, type RunControlsProps } from "./RunControls";
import { RunPromptEntry } from "./RunPromptEntry";

export type RunInlineControlsProps = Omit<RunControlsProps, "open" | "onOpenChange" | "adjustOpen" | "onAdjustOpenChange">;

/** The answer box and the approval's decision, or the prompt stage's editor, for the selected stage. They stay in the panel while a command is refused. */
export function RunInlineControls(props: RunInlineControlsProps & { part?: "answer" | "decision" }) {
  const { view, stage, panel, draft, command, submit, canRead, connection, part } = props;
  if (part === undefined) return <><RunInlineControls {...props} part="answer" /><RunInlineControls {...props} part="decision" /></>;
  const pending = command.pending;
  const online = connection === "connected" && canRead;
  const actions: readonly RunAction[] = view.actions;
  const actionFor = (id: string, selectors = {}) => findRunAction(actions, id, selectors);
  const answerAction = actions.find(action => action.id === "answer" && typeof action.stage === "string" && typeof action.attempt === "string");
  const failed = command.error !== null && isInlineCommand(command.lastRequest?.path);
  const failure = failed && <CommandFailure command={command} />;

  if (part === "answer") {
    if (answerAction?.stage !== stage.id) return null;
    const empty = draft.answer.trim() === "";
    return <div className="run-answer">
      <label className="paper-caps" htmlFor="run-answer-text">Answer {stage.name}</label>
      <textarea id="run-answer-text" className="paper-field" rows={3} placeholder="Your answer to the questions below" value={draft.answer} disabled={!online || pending}
        onChange={event => props.onAnswerChange(event.currentTarget.value)}
        onKeyDown={event => {
          if (event.key === "Enter" && !event.altKey && !event.shiftKey && !empty && answerAction.enabled) {
            event.preventDefault();
            submit("answer", { answer: draft.answer, stage: answerAction.stage, attempt: answerAction.attempt });
          }
        }} />
      <div className="run-answer-actions">
        <button type="button" className="action-button small slate" disabled={!online || pending || empty || !answerAction.enabled}
          title={answerAction.enabled ? "Return sends it; Option-Return starts a new line" : answerAction.reason ?? undefined}
          onClick={() => submit("answer", { answer: draft.answer, stage: answerAction.stage, attempt: answerAction.attempt })}>Send answer</button>
        <button type="button" className="action-button small" disabled={!online || pending || !answerAction.enabled}
          title={answerAction.reason ?? view.heldAnswer?.buttonHelp ?? undefined}
          onClick={() => submit("answer", { answer: null, stage: answerAction.stage, attempt: answerAction.attempt })}>
          {view.heldAnswer?.buttonTitle ?? "I answered in the session"}
        </button>
      </div>
      {view.heldAnswer && <p className="run-muted">{view.heldAnswer.note}</p>}
      {failure}
    </div>;
  }

  const prompt = panel.facts?.approval?.prompt;
  if (prompt) return <RunPromptEntry {...props} prompt={prompt} failure={failure} />;
  // The daemon lists the outcome buttons and names the good one (the wide button); the others sit under it.
  const outcomes = panel.facts?.approval?.outcomes ?? [];
  if (outcomes.length === 0) return null;
  const primaryId = panel.facts?.approval?.primary;
  const primary = outcomes.find(outcome => outcome.id === primaryId);
  const deciding = outcomes.find(outcome => outcome.id === draft.pendingOutcome);
  const decideAction = (outcome: (typeof outcomes)[number]) => actionFor("decide", { stage: stage.id, outcome: outcome.name });
  const decideButton = (outcome: (typeof outcomes)[number], kind: string) => {
    const action = decideAction(outcome);
    return <button key={outcome.id} type="button" className={`action-button ${kind}`} disabled={!online || pending || !action?.enabled}
      title={action?.reason ?? action?.thenHelp ?? undefined}
      onClick={() => {
        if (action?.needsNote) props.onPendingOutcomeChange(outcome.id);
        else submit("decide", { outcome: outcome.name, note: "", stage: stage.id });
      }}>{outcome.label}</button>;
  };
  return <div className="run-decide">
    {deciding ? <>
      <label className="paper-caps" htmlFor="run-decision-note">{decideAction(deciding)?.noteLabel ?? "Note for the next run"}</label>
      <textarea id="run-decision-note" className="paper-field" rows={3} placeholder="What should change" value={draft.notes[deciding.id] ?? ""} disabled={!online || pending}
        onChange={event => props.onDecisionNoteChange(deciding.id, event.currentTarget.value)} />
      <div className="run-decide-actions">
        <button type="button" className="action-button danger" disabled={!online || pending || (draft.notes[deciding.id] ?? "").trim() === ""}
          onClick={() => submit("decide", { outcome: deciding.name, note: draft.notes[deciding.id] ?? "", stage: stage.id })}>{deciding.label}</button>
        <button type="button" className="action-button" disabled={pending} onClick={() => props.onPendingOutcomeChange(null)}>Cancel</button>
      </div>
    </> : <>
      {primary && <div className="run-decide-primary">{decideButton(primary, "slate wide")}</div>}
      <div className="run-outcome-row">
        {outcomes.filter(outcome => outcome.id !== primary?.id).map(outcome => decideButton(outcome, outcome.tone === "problem" ? "danger" : ""))}
      </div>
      <p className="run-muted">{panel.facts?.approval?.outcomesHelp}</p>
    </>}
    {failure}
  </div>;
}
