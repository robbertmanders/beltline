import { useState } from "react";
import type { AfkDecision, AfkOverruleOffer, RunResponse } from "../api/wire.gen";
import { runCommandRequest } from "../api/runCommands";
import { useCommand } from "../api/useCommand";
import { ErrorProblems } from "../components/ProblemList";
import { Sheet } from "../components/Sheet";

/**
 * Rejects an applied AFK decision, with comments, and restarts an agent stage, as the app's Overrule sheet did.
 * Where it may restart and the warnings are the daemon's offer; the browser only holds the unsent form.
 */
export function OverruleSheet({ decision, offer, onClose }: { decision: AfkDecision; offer: AfkOverruleOffer; onClose: () => void }) {
  const command = useCommand();
  const [choice, setChoice] = useState<"previous" | "chosen">(offer.previous ? "previous" : "chosen");
  const [chosen, setChosen] = useState("");
  const [comments, setComments] = useState("");
  const [accepted, setAccepted] = useState(false);
  const target = choice === "previous" ? offer.previous : offer.choices.find(item => item.stageId === chosen);
  const canSubmit = accepted && comments.trim() !== "" && target !== undefined && !command.pending;
  const submit = () => {
    if (!canSubmit || !target) return;
    command.submit({
      ...runCommandRequest(decision.runId, "overrule-afk", { decision: decision.id, stage: target.stageId, comments, acceptedWarning: accepted }),
      affects: ["runs", "afk", "library", "settings", "state"],
      publish: (queryClient, result: RunResponse) => { runCommandRequest(decision.runId, "overrule-afk").publish(queryClient, result); onClose(); }
    });
  };
  const footer = <>
    <button type="button" className="btn" disabled={command.pending} onClick={onClose}>Cancel</button>
    <button type="button" className="btn fill" disabled={!canSubmit} onClick={submit}>Overrule and restart</button>
  </>;
  return <Sheet title={decision.subjectRef} tag="Overrule" width={620} open onClose={onClose} footer={footer}>
    <p className="overrule-title">{decision.subjectTitle}</p>
    <dl className="overrule-facts">
      <dt>Original decision</dt><dd>{decision.headline}</dd>
      <dt>Stage</dt><dd>{offer.decisionStage ?? decision.stageName}</dd>
      <dt>Current stage</dt><dd>{offer.currentStage ?? ""}</dd>
      <dt>Restart</dt><dd>{target?.label ?? "Choose an agent"}</dd>
    </dl>
    <div className="sheet-field">
      <label>Restart at</label>
      <div className="overrule-choice">
        <button type="button" className={`btn sm${choice === "previous" ? " fill" : ""}`} disabled={!offer.previous}
          aria-pressed={choice === "previous"} onClick={() => setChoice("previous")}>Previous agent</button>
        <button type="button" className={`btn sm${choice === "chosen" ? " fill" : ""}`}
          aria-pressed={choice === "chosen"} onClick={() => setChoice("chosen")}>Choose an agent in this belt</button>
      </div>
      {choice === "chosen" && <select className="paper-native-select" aria-label="Agent stage" value={chosen} onChange={event => setChosen(event.currentTarget.value)}>
        <option value="">Choose an agent</option>
        {offer.choices.map(item => <option key={item.stageId} value={item.stageId}>{item.label}</option>)}
      </select>}
    </div>
    <div className="sheet-field">
      <label htmlFor={`overrule-${decision.id}`}>Comments</label>
      <textarea id={`overrule-${decision.id}`} className="paper-field overrule-comments" value={comments} onChange={event => setComments(event.currentTarget.value)} />
      <p className="sheet-help">{offer.commentsHelp}</p>
    </div>
    <p className="overrule-warning">{offer.warning}</p>
    <label className="tg"><input type="checkbox" checked={accepted} onChange={event => setAccepted(event.currentTarget.checked)} /><span className="t" aria-hidden="true" />I understand, restart anyway</label>
    <p className="sheet-help">{offer.notice}</p>
    {command.error ? <ErrorProblems error={command.error} /> : null}
  </Sheet>;
}
