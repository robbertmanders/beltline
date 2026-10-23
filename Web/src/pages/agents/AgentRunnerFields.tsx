import { useQuery } from "@tanstack/react-query";
import { apiRequest } from "../../api/client";
import { describeError } from "../../api/errors";
import { apiQueryKey } from "../../api/resources";
import { backendValues } from "../../api/wire.gen";
import type { Backend, ModelRunnerResource, Thinking } from "../../api/wire.gen";

import { useRuntime } from "../../app/runtime";
import { backendLabels } from "../../components/backendLabels";
import { ModelPicker, type ModelPickerState } from "../../components/ModelPicker";

/** The harness, model and thinking fields one row shows, from the daemon only. The caller places them in a row. */
export function RunnerFields({ idPrefix, runner, model, thinking, onModel, onThinking, onChangeRunner, runnerDisabled }: {
  idPrefix: string;
  runner: Backend;
  model: string;
  thinking: Thinking | null | undefined;
  onModel: (model: string) => void;
  onThinking: (thinking: Thinking | null) => void;
  onChangeRunner: (runner: Backend) => void;
  runnerDisabled: boolean;
}) {
  const runtime = useRuntime();
  const query = useQuery({
    queryKey: apiQueryKey(`/api/v1/models/${runner}`),
    queryFn: ({ signal }) => apiRequest<ModelRunnerResource>(`/api/v1/models/${runner}`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: ["models", "settings"] }
  });
  const models: ModelRunnerResource | undefined = query.data;
  const state: ModelPickerState = query.isPending ? "loading" : query.isError ? "failed" : "ready";
  const thinkingChoices = models?.thinking ?? [];
  const staleThinking = thinking && !thinkingChoices.some(choice => choice.id === thinking);
  // The visible headings sit once above the rows (ModelColumnHeadings), so each control keeps its name in a hidden label.
  return <>
    <label className="agent-field">
      <select className="paper-field" aria-label="Harness" value={runner} disabled={runnerDisabled}
        title={runnerDisabled ? "Connect to the daemon to change the runner." : undefined} onChange={event => onChangeRunner(event.currentTarget.value as Backend)}>
        {backendValues.map(name => <option key={name} value={name}>{backendLabels[name]}</option>)}
      </select>
    </label>
    <ModelPicker id={`${idPrefix}-model`} label="Model" labelHidden runnerLabel={backendLabels[runner]}
      choices={models?.choices ?? []} allowedModels={models?.allowedModels} value={model}
      state={state} errorMessage={query.isError ? describeError(query.error).message : null}
      onChange={onModel} />
    <label className="agent-field">
      <select className="paper-field" aria-label="Thinking" value={thinking ?? ""}
        disabled={thinkingChoices.length === 0 && !staleThinking}
        onChange={event => onThinking(event.currentTarget.value ? event.currentTarget.value as Thinking : null)}>
        <option value="">The runner&apos;s default</option>
        {staleThinking && <option value={thinking!} disabled>{`${thinking} (no longer offered)`}</option>}
        {thinkingChoices.map(choice => <option key={choice.id} value={choice.id}>{choice.label}</option>)}
      </select>
      {models?.thinkingNote && <span className="agent-hint">{models.thinkingNote}</span>}
      {state === "loading" && <span className="agent-hint" role="status">Loading thinking choices…</span>}
      {state === "failed" && <span className="agent-hint agent-error" role="alert">Thinking choices could not be loaded.</span>}
      {state === "ready" && thinkingChoices.length === 0 && <span className="agent-hint">No thinking choices reported for {backendLabels[runner]}.</span>}
      {staleThinking && <span className="agent-hint agent-stale">This level is no longer offered; pick one of the runner&apos;s choices.</span>}
    </label>
  </>;
}
