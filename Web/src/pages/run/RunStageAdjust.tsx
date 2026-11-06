import { useState } from "react";
import { apiRequest } from "../../api/client";
import type { CommandController } from "../../api/useCommand";
import type { RunAction, RunAdjustSwitchResponse, RunStage } from "../../api/wire.gen";
import { ModelPicker } from "../../components/ModelPicker";
import { ErrorProblems } from "../../components/ProblemList";
import { Sheet } from "../../components/Sheet";
import type { RunCommandSubmit } from "./RunControls";

interface RunStageAdjustProps {
  runId: string;
  stageId: string;
  help: NonNullable<RunStage["adjust"]>;
  action: RunAction | undefined;
  enabled: boolean;
  command: CommandController;
  failed: boolean;
  submit: RunCommandSubmit;
  onClose: () => void;
}

/** Mounted only while open, once the daemon has supplied the selected stage's settings. */
export function RunStageAdjust({ runId, stageId, help, action, enabled, command, failed, submit, onClose }: RunStageAdjustProps) {
  const [adjust, setAdjust] = useState({
    runner: help.runner, model: help.model, thinking: help.thinking ?? "", brief: help.brief
  });
  const [switchError, setSwitchError] = useState<string | null>(null);
  const runnerChoice = help.runners.find(item => item.runner === adjust.runner);
  const models = runnerChoice?.models ?? [];
  const switchRunner = async (runner: string) => {
    setSwitchError(null);
    try {
      const next = await apiRequest<RunAdjustSwitchResponse>(`/api/v1/runs/${encodeURIComponent(runId)}/adjust/switch`, {
        method: "POST", body: { stage: stageId, runner, model: adjust.model, thinking: adjust.thinking || undefined }
      });
      setAdjust(previous => ({ ...previous, runner: next.runner, model: next.model, thinking: next.thinking ?? "" }));
    } catch (error) {
      setSwitchError(error instanceof Error ? error.message : "The runner could not be changed.");
    }
  };

  return <Sheet title="Adjust stage" className="adjust-sheet" open onClose={onClose} footer={<>
    <button type="button" className="btn" onClick={onClose}>Cancel</button>
    <button type="button" className="btn fill" disabled={!enabled} title={action?.reason ?? undefined}
      onClick={() => submit("adjust-stage", { stage: stageId, model: adjust.model, thinking: adjust.thinking || undefined, brief: adjust.brief, runner: adjust.runner }, onClose)}>Apply</button>
  </>}>
    <div className="sheet-field">
      <label htmlFor="adjust-runner">Harness</label>
      <select id="adjust-runner" value={adjust.runner} onChange={event => void switchRunner(event.currentTarget.value)}>
        {help.runners.map(item => <option key={item.runner} value={item.runner}>{item.label}</option>)}
      </select>
    </div>
    <ModelPicker id="adjust-model" label="Model" runnerLabel={runnerChoice?.label ?? adjust.runner} choices={models} value={adjust.model}
      onChange={model => setAdjust(previous => ({ ...previous, model }))} />
    {action?.help && <p className="muted-copy">{action.help}</p>}
    {help.repairNote && <p className="muted-copy">{help.repairNote}</p>}
    {models.length === 0 && runnerChoice?.emptyNote && <p className="muted-copy">{runnerChoice.emptyNote}</p>}
    <div className="sheet-field">
      <label htmlFor="adjust-thinking">Effort</label>
      <select id="adjust-thinking" value={adjust.thinking} disabled={(runnerChoice?.efforts ?? []).length === 0}
        onChange={event => { const thinking = event.currentTarget.value; setAdjust(previous => ({ ...previous, thinking })); }}>
        <option value="">Runner default</option>
        {(runnerChoice?.efforts ?? []).map(option => <option key={option} value={option}>{option}</option>)}
      </select>
      {runnerChoice?.thinkingNote && <p className="muted-copy">{runnerChoice.thinkingNote}</p>}
    </div>
    <div className="sheet-field adjust-brief">
      <label htmlFor="adjust-brief">Brief</label>
      <p className="muted-copy">The agent's brief can be changed for this run only. The issue, any note, and the outcome footer are added automatically.</p>
      {help.agentBrief != null && <button type="button" className="link" onClick={() => setAdjust(previous => ({ ...previous, brief: help.agentBrief ?? "" }))}>Use the agent's brief</button>}
      <textarea id="adjust-brief" className="paper-field agent-brief" value={adjust.brief} onChange={event => { const brief = event.currentTarget.value; setAdjust(previous => ({ ...previous, brief })); }} />
    </div>
    <section className="adjust-placeholders" aria-label="Placeholders">
      <span className="paper-caps">Placeholders</span>
      <ul>{help.placeholders.entries.map(entry => <li key={entry.name}><code>{`{${entry.name}}`}</code> — {entry.description}</li>)}</ul>
      <p className="muted-copy">{help.note}</p>
    </section>
    <p className="muted-copy">{help.freshSessionNote}</p>
    {switchError && <p className="run-menu-error">{switchError}</p>}
    {failed && <ErrorProblems error={command.error} />}
  </Sheet>;
}
