import { useQuery } from "@tanstack/react-query";
import { apiRequest } from "../../api/client";
import { apiQueryKey } from "../../api/resources";

import { backendValues, fallbackValues } from "../../api/wire.gen";
import type { Backend, ModelRunnerResource, Outcome, Stage, StageMenu, Thinking, Workspace } from "../../api/wire.gen";
import { useRuntime } from "../../app/runtime";
import { ModelColumnHeadings } from "../../components/ModelColumnHeadings";
import { ModelPicker } from "../../components/ModelPicker";
import { OutcomesEditor } from "./OutcomesEditor";
import { useDraftEditor, useTypedForm } from "./draftEditor";

const fallbackLabels: Record<(typeof fallbackValues)[number], string> = {
  hold: "Hold for me",
  retry: "Run this stage again, once",
  drop: "Drop the run"
};

type Alternate = NonNullable<Stage["alternates"]>[number];

/** The models the daemon offers for one runner, read only when a picker for it is shown. */
export function useRunnerModels(runner: string, enabled = true) {
  const runtime = useRuntime();
  const known = (backendValues as readonly string[]).includes(runner);
  const query = useQuery({
    queryKey: apiQueryKey(`/api/v1/models/${runner}`),
    queryFn: ({ signal }) => apiRequest<ModelRunnerResource>(`/api/v1/models/${runner}`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && enabled && known,
    retry: false,
    meta: { resourceDeps: ["models", "settings"] }
  });
  return {
    choices: query.data?.choices ?? [],
    allowedModels: query.data?.allowedModels ?? [],
    state: (query.isError ? "failed" : query.data ? "ready" : "loading") as "failed" | "ready" | "loading",
    errorMessage: query.data?.error ?? (query.error instanceof Error ? query.error.message : null)
  };
}

function ThinkingSelect({ label, runner, value, onChange }: { label: string; runner: StageMenu["runners"][number] | undefined; value: string; onChange: (value: string) => void }) {
  const levels = runner?.thinking ?? [];
  // A saved level the runner does not list stays visible, so it is never replaced without the person choosing.
  const unlisted = value !== "" && !levels.some(level => level.id === value);
  return <>
    <select aria-label={label} value={value} disabled={levels.length === 0 && !unlisted} onChange={event => onChange(event.target.value)}>
      <option value="">The runner's default</option>
      {unlisted && <option value={value}>{`${value} (not taken by ${runner?.label ?? "this runner"})`}</option>}
      {levels.map(level => <option key={level.id} value={level.id}>{level.label}</option>)}
    </select>
    {runner?.thinkingNote && <p className="field-note">{runner.thinkingNote}</p>}
  </>;
}

interface ModelRowProps {
  className: string;
  id: string;
  labels: { harness: string; model: string; thinking: string; remove?: string };
  menu: StageMenu | null;
  runner: string;
  model: string;
  thinking: string;
  staleReason?: string | null;
  onRunner: (runner: string) => void;
  onModel: (model: string) => void;
  onThinking: (thinking: string) => void;
  onRemove?: () => void;
}

/** One row of the model list, laid out as the Agent editor's rows: harness, model and thinking, with Remove beside an alternate. */
function ModelRow({ className, id, labels, menu, runner, model, thinking, staleReason, onRunner, onModel, onThinking, onRemove }: ModelRowProps) {
  const models = useRunnerModels(runner);
  const choice = menu?.runners.find(item => item.id === runner);
  return <div className={`agent-form-row agent-form-row--model stage-model-row ${className}`}>
    <div className="agent-field model-cell" data-heading="Harness">
      <select className="paper-field" aria-label={labels.harness} value={runner} disabled={!menu} onChange={event => onRunner(event.target.value)}>
        {(menu?.runners ?? []).map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
      </select>
    </div>
    <div className="agent-field model-cell" data-heading="Model">
      {runner === "" ? <p className="loading-state">Loading runners…</p>
        : <ModelPicker id={id} label={labels.model} labelHidden runnerLabel={choice?.label ?? runner} choices={models.choices}
          allowedModels={models.allowedModels} value={model} staleReason={staleReason} state={models.state} errorMessage={models.errorMessage} onChange={onModel} />}
    </div>
    <div className="agent-field model-cell" data-heading="Thinking">
      <ThinkingSelect label={labels.thinking} runner={choice} value={thinking} onChange={onThinking} />
    </div>
    {onRemove && <button type="button" className="quiet-button agent-form-remove" aria-label={labels.remove} onClick={onRemove}>Remove</button>}
  </div>;
}

/**
 * One stage's settings, laid out as the Agent editor lays out its cards: Stage settings, then Brief (agent stages
 * only) and Outcomes. Typing is local; Save sends one stage-update with the changed settings and outcomes, and the
 * daemon's answer replaces the draft. Moving the stage to another route and changing what a watch waits on are
 * structural and are sent when chosen. The menus, the pinned agent, its brief and the preview are the daemon's
 * context for this stage; until it arrives the form says so.
 */
export function StageForm({ stage, onEditAgent, onUpdateStage, updateStageEnabled }: {
  stage: Stage;
  /** Opens the stage's agent: its editor, or, for a library belt, the library's read-only view of it. */
  onEditAgent: (agent: { id: string; version: number }) => void;
  onUpdateStage: (stageId: string) => void;
  updateStageEnabled: boolean;
}) {
  const editor = useDraftEditor();
  const readOnly = editor.readOnly === true;
  const kind = stage.type ?? "agent";
  const isAgent = kind === "agent";
  const menu = editor.context?.stage?.stage === stage.id ? editor.context.stage : null;
  const pinned = isAgent && menu ? editor.context?.agent ?? null : null;
  const form = useTypedForm(`stage:${stage.id}`, {
    name: stage.name,
    brief: stage.brief ?? "",
    model: stage.model ?? "",
    runner: (stage.runner ?? "") as Backend | "",
    key: stage.key ?? "",
    slots: stage.slots === undefined ? "" : String(stage.slots),
    workspace: stage.workspace as Workspace | "",
    thinking: (stage.thinking ?? "") as Thinking | "",
    fallback: stage.fallback ?? "hold",
    alternates: (stage.alternates ?? []) as Alternate[],
    outcomes: stage.outcomes as Outcome[]
  });
  const { name, brief, model, runner, key, slots, workspace, thinking, fallback, alternates, outcomes } = form.values;
  const setName = (value: string) => form.set("name", value);
  const setBrief = (value: string) => form.set("brief", value);
  const setModel = (value: string) => form.set("model", value);
  const setRunner = (value: string) => form.set("runner", value as Backend | "");
  const setKey = (value: string) => form.set("key", value);
  const setSlots = (value: string) => form.set("slots", value);
  const setWorkspace = (value: string) => form.set("workspace", value as Workspace | "");
  const setThinking = (value: string) => form.set("thinking", value as Thinking | "");
  const setFallback = (value: string) => form.set("fallback", value as typeof fallback);
  const setAlternates = (value: Alternate[]) => form.set("alternates", value);
  const setAlternate = (index: number, change: Partial<Alternate>) => setAlternates(alternates.map((item, position) => position === index ? { ...item, ...change } : item));
  const setOutcomes = (value: Outcome[]) => form.set("outcomes", value);
  const reason = editor.blocked ?? editor.structuralReason;
  const backend = runner || menu?.defaultRunner || "";
  const placeholders = editor.context?.placeholders?.entries ?? [];
  // The daemon's placeholder prose: the chips' note and the help for the stage's placeholders.
  const prose = editor.context?.placeholders?.prose ?? null;
  // An empty brief runs with the agent's; the field shows that brief until the person changes it.
  const inherited = pinned?.brief ?? "";
  const shownBrief = brief === "" ? inherited : brief;

  const apply = (overrides: Record<string, unknown> = {}) => {
    const update: Record<string, unknown> = { name, key, outcomes, fallback };
    if (isAgent) {
      update.brief = brief;
      update.model = model;
      if (runner !== "") update.runner = runner;
      if (slots !== "") update.slots = Number(slots);
      if (workspace !== "") update.workspace = workspace;
      if (thinking !== "") update.thinking = thinking;
      else update.clearThinking = true;
      update.alternates = alternates;
    }
    editor.operate("stage-update", { stageId: stage.id, update: { ...update, ...overrides } }, form.applies());
  };

  const heading = menu ? `${String(menu.number).padStart(2, "0")} · ${menu.kind}` : kind === "approve" ? "Approval" : kind === "prompt" ? "Prompt" : kind === "watch" ? "Watcher" : "Agent stage";
  const feedAgent = editor.context?.line?.agents.find(agent => agent.id === stage.agent?.id) ?? editor.context?.line?.agents[0];

  return <section className="stage-form" aria-label={`Stage ${stage.name} settings`} data-stage-form={stage.id} data-ready={menu ? "true" : undefined}>
    <section className="agent-card belt-card" aria-label="Stage settings">
      <div className="agent-card-head stage-form-head">
        <div className="form-heading"><span className="paper-tag">{heading}</span><h3>{name || "Unnamed stage"}</h3></div>
        {readOnly ? stage.agent && <button type="button" className="action-button small" onClick={() => onEditAgent(stage.agent!)}>Open agent</button>
          : <div className="stage-form-moves" role="group" aria-label="Move this stage">
          <button type="button" className="action-button small" disabled={reason !== null} aria-label={`Fork at ${stage.name}`}
            onClick={() => editor.operate("fork-add", { stageId: stage.id })}>Fork here</button>
          <button type="button" className="action-button small" disabled={reason !== null || !feedAgent} aria-label={`Feed a new stage into ${stage.name}`}
            onClick={() => { if (feedAgent) void editor.addStage({ type: "agent", agent: { id: feedAgent.id, version: feedAgent.version }, feed: stage.id }); }}>Feed in here</button>
          <button type="button" className="action-button small" disabled={reason !== null || !menu?.canMoveEarlier}
            onClick={() => editor.operate("stage-move", { stageId: stage.id, step: -1 })}>← Earlier</button>
          <button type="button" className="action-button small" disabled={reason !== null || !menu?.canMoveLater}
            onClick={() => editor.operate("stage-move", { stageId: stage.id, step: 1 })}>Later →</button>
          <button type="button" className="action-button small danger" disabled={reason !== null}
            onClick={() => { editor.operate("stage-remove", { stageId: stage.id }); editor.selectStage(null); }}>Remove stage</button>
        </div>}
      </div>
      {!menu && <p className="loading-state" role="status">Loading this stage's choices…</p>}
      <fieldset className="editor-fields" disabled={readOnly}>
      <div className="agent-form">
        <div className="agent-form-row agent-form-row--pair">
          <label className="agent-field"><span className="agent-field-label">Name</span>
            <input className="paper-field" id={`stage-name-${stage.id}`} {...editor.problemTarget("stage", stage.id)} value={name} onChange={event => setName(event.target.value)} />
          </label>
          {isAgent && <div className="agent-field agent-pin" data-agent-pin={stage.agent ? `${stage.agent.id}@${stage.agent.version}` : ""}><span className="agent-field-label">Agent</span>
            <span className="agent-pin-row">
              <span className="agent-pin-name">{pinned?.name ?? stage.agent?.id ?? "No agent"}</span>
              <code>v{stage.agent?.version ?? 0}</code>
              {pinned?.archived && <span className="field-note">Archived: kept for this stage, not offered for new ones.</span>}
              {pinned?.missing && <span className="field-note">This version is missing from the library.</span>}
              {!readOnly && pinned?.latestVersion != null && <button type="button" className="quiet-button" disabled={reason !== null || !updateStageEnabled}
                onClick={() => onUpdateStage(stage.id)}>Update to v{pinned.latestVersion}</button>}
              {!readOnly && stage.agent && <button type="button" className="action-button small agent-pin-edit" onClick={() => onEditAgent(stage.agent!)}>Edit</button>}
            </span>
          </div>}
          {kind === "approve" && <label className="agent-field"><span className="agent-field-label">Decided by</span>
            <select className="paper-field" aria-label="Decided by" value="you" disabled><option value="you">You</option></select>
          </label>}
          {kind === "prompt" && <label className="agent-field"><span className="agent-field-label">Source context</span>
            <select className="paper-field" aria-label="Source context" value={stage.source ?? "withheld"} disabled={reason !== null || !menu}
              onChange={event => editor.operate("stage-update", { stageId: stage.id, update: { source: event.target.value } })}>
              {(menu?.sources ?? []).map(source => <option key={source.id} value={source.id}>{source.label}</option>)}
            </select>
          </label>}
          {kind === "watch" && <label className="agent-field"><span className="agent-field-label">Waits on</span>
            <select className="paper-field" aria-label="Waits on" value={stage.watch ?? "pullRequest"} disabled={reason !== null || !menu}
              onChange={event => editor.operate("stage-update", { stageId: stage.id, update: { watch: event.target.value } })}>
              {(menu?.watchTargets ?? []).map(target => <option key={target.id} value={target.id}>{target.label}</option>)}
            </select>
          </label>}
        </div>
        {menu?.placement && <label className="agent-field"><span className="agent-field-label">Route</span>
          <select className="paper-field" aria-label="Route" {...editor.problemTarget("stage", stage.id, "route")} value={stage.route ?? ""} disabled={reason !== null}
            onChange={event => editor.operate("stage-move", event.target.value === "" ? { stageId: stage.id, detach: true } : { stageId: stage.id, route: event.target.value })}>
            <option value="">{menu.placement.off}</option>
            {menu.placement.routes.map(route => <option key={route.id} value={route.id}>{route.label}</option>)}
          </select>
        </label>}
        {kind === "watch" && menu?.watchNote && <p className="agent-hint watch-note">{menu.watchNote}</p>}
        {isAgent && <div className="agent-form-row agent-form-row--pair">
          <label className="agent-field"><span className="agent-field-label">Workspace</span>
            <select className="paper-field" aria-label="Workspace" value={workspace} disabled={!menu} onChange={event => setWorkspace(event.target.value)}>
              {(menu?.workspaces ?? []).map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
            </select>
          </label>
          <label className="agent-field"><span className="agent-field-label">Runs at once</span>
            <input className="paper-field" type="number" min={1} max={6} aria-label="Slots" value={slots} onChange={event => setSlots(event.target.value)} />
          </label>
        </div>}
        {isAgent && <label className="agent-field"><span className="agent-field-label">Placeholder</span>
          <input className="paper-field" aria-label="Placeholder key" value={key} placeholder={pinned?.key ?? "key"} onChange={event => setKey(event.target.value)} />
        </label>}
        {isAgent && <div className="agent-model-list stage-model-list">
          <ModelColumnHeadings />
          <ModelRow className="stage-model-primary" id={`stage-model-${stage.id}`} menu={menu} runner={backend} model={model} thinking={thinking}
            labels={{ harness: "Harness", model: "Model", thinking: "Thinking" }}
            staleReason={editor.context?.modelStatus?.id === model ? editor.context.modelStatus.reason : undefined}
            onRunner={setRunner} onModel={setModel} onThinking={setThinking} />
          {menu && alternates.map((alternate, index) => <ModelRow key={index} className="alternate-row" id={`alternate-${stage.id}-${index}`}
            labels={{ harness: `Alternate ${index + 1} harness`, model: `Alternate ${index + 1} model`, thinking: `Alternate ${index + 1} thinking`, remove: `Remove alternate ${index + 1}` }}
            menu={menu} runner={alternate.runner ?? (menu.defaultRunner as Backend)} model={alternate.model} thinking={alternate.thinking ?? ""}
            onRunner={runner => setAlternate(index, { runner: runner as Backend })} onModel={model => setAlternate(index, { model })}
            onThinking={value => setAlternate(index, { thinking: value === "" ? undefined : value as Thinking })}
            onRemove={readOnly ? undefined : () => setAlternates(alternates.filter((_, position) => position !== index))} />)}
        </div>}
        {isAgent && menu && <p className="agent-hint">{alternates.length === 0 ? "Only the model above." : menu.alternatesNote}</p>}
        {isAgent && menu && !readOnly && <div>
          <button type="button" className="action-button small" onClick={() => setAlternates([...alternates, { runner: menu.defaultRunner as Backend, model: "" }])}>Add a model</button>
        </div>}
      </div>
      </fieldset>
    </section>

    {isAgent && <section className="agent-card belt-card stage-brief-card" aria-label="Stage brief">
      <div className="form-heading"><span className="paper-tag">Brief</span><h3>What the agent is told</h3></div>
      <textarea className="paper-field agent-brief" id={`stage-brief-${stage.id}`} aria-label="Brief" value={shownBrief} rows={12} readOnly={readOnly}
        onChange={event => { const value = event.target.value; setBrief(value === inherited ? "" : value); }} />
      <p className="agent-hint">{brief === ""
        ? `The brief of ${pinned?.name ?? "the agent"} v${stage.agent?.version ?? 1}. ${readOnly ? "" : "Edit it to change it for this belt only. "}`
        : "Changed for this belt. "}The issue, any note, and the outcome footer are added automatically.</p>
      {!readOnly && <>
      {brief !== "" && <button type="button" className="quiet-button stage-brief-reset" disabled={reason !== null}
        onClick={() => { setBrief(""); apply({ brief: "" }); }}>Use the agent's brief</button>}
      {form.typed && <button type="button" className="quiet-button" onClick={form.clear}>Discard typed changes</button>}
      {placeholders.length > 0 && <ul className="placeholder-chips" aria-label="Placeholders">
        {placeholders.map(entry => <li key={entry.name} title={entry.description}><code>{`{${entry.name}}`}</code></li>)}
      </ul>}
      {prose && <div className="placeholder-prose">
        <p className="field-note">{prose.resolutionNote}</p>
        <details>
          <summary>How placeholders work</summary>
          <p>{prose.introduction}</p>
          <p>{prose.example}</p>
          <p className="field-note">{prose.exampleNote}</p>
          <p>{prose.resolution}</p>
          <p>{prose.warnings}</p>
          <p>{prose.syntax}</p>
        </details>
      </div>}
      </>}
    </section>}

    <section className="agent-card belt-card stage-outcomes-card" aria-label="Stage outcomes">
      <fieldset className="editor-fields" disabled={readOnly}>
      <OutcomesEditor stageId={stage.id} kind={kind} outcomes={outcomes} context={editor.context} menu={menu} disabled={reason !== null} readOnly={readOnly} onChange={setOutcomes} />

      {isAgent && <label className="belt-field fallback-field"><span>If the run fails or ends without one of these</span>
        <select aria-label="When the stage fails" value={fallback} onChange={event => setFallback(event.target.value)}>
          {fallbackValues.map(item => <option key={item} value={item}>{fallbackLabels[item]}</option>)}
        </select>
      </label>}
      </fieldset>

      {menu && <section className="agent-footer outcome-preview" aria-label={menu.preview.title}>
        <span className="paper-tag">{menu.preview.title}</span>
        <pre>{menu.preview.lines.join("\n")}</pre>
      </section>}
    </section>
  </section>;
}
