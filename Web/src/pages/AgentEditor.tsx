import { useId, type CSSProperties, type ReactNode } from "react";
import { isActionEnabled } from "../api/actions";
import { describeError } from "../api/errors";
import { stageIconValues, tintValues, workspaceValues } from "../api/wire.gen";
import type { Agent, AgentAction, AgentPresentation, AgentRecordResponse, Backend, LibraryOrigin, StageIcon } from "../api/wire.gen";



import { useCommand } from "../api/useCommand";
import { ErrorProblems, ProblemItems } from "../components/ProblemList";
import { ModelColumnHeadings } from "../components/ModelColumnHeadings";
import { StageBox } from "../theme/stages/StageBox";
import { AgentOutcomeRow } from "./agents/AgentOutcomeRow";
import { backendLabels } from "../components/backendLabels";
import { RunnerFields } from "./agents/AgentRunnerFields";

const workspaceLabels: Record<(typeof workspaceValues)[number], string> = {
  readOnly: "Read-only checkout",
  worktree: "The run's worktree",
  worktreeReadOnly: "Read-only worktree"
};
const tints = tintValues;
/** The icons an agent may pick: the first sixteen of the daemon's icons (StageIcon.agentChoices). */
export const agentIconChoices: readonly StageIcon[] = stageIconValues.slice(0, 16);
const iconLabels: Partial<Record<StageIcon, string>> = { doc: "Document", approve: "Approval", scope: "Magnifier", chip: "Chip" };
/** The label the daemon gives an icon; the same words as StageIcon.label. */
export function iconLabel(icon: StageIcon): string {
  return iconLabels[icon] ?? icon[0]!.toUpperCase() + icon.slice(1);
}

function titleCase(value: string): string {
  return value.length ? value[0]!.toUpperCase() + value.slice(1) : value;
}



export interface AgentEditorProps {
  agent: Agent;
  /** The saved agent the editor compares with. */
  saved: Pick<AgentRecordResponse, "agent"> | null;
  actions: readonly AgentAction[];
  presentation: AgentPresentation | undefined;
  /** The agent's folder on the daemon's Mac, from the saved agent's detail. */
  folder?: string;
  isNew: boolean;
  dirty: boolean;
  canCommand: boolean;
  commandReason?: string;
  commandError: unknown;
  onRetry: () => void;
  onChange: (agent: Agent) => void;
  onChangeRunner: (runner: Backend, alternateIndex?: number) => void;
  onOpenBelt: (beltId: string) => void;
  onNew: () => void;
  onSave: () => void;
  /** The daemon's actions for this draft have not arrived yet. */
  actionsLoading?: boolean;
  /** A save is on its way. */
  saving?: boolean;
  onDiscard: () => void;
  onArchive: (archived: boolean) => void;
  onShare: () => void;
  /** The configuration assistant's button, at the end of the header's actions. */
  assistantButton?: ReactNode;
  /** The library repository a saved agent was installed from, if any. */
  origin?: LibraryOrigin | null;
}

/** The Agents editor: settings, brief, outcomes and Used by, all on a draft. */
export function AgentEditor(props: AgentEditorProps) {
  const { agent, saved, actions, presentation, isNew, dirty, canCommand, commandReason, commandError, onRetry, onChange, onChangeRunner } = props;
  const breadcrumb = [
    agent.origin ? "Installed" : "Your agent",
    isNew ? "new" : `v${agent.version}`,
    presentation?.archived ? "archived" : null
  ].filter(Boolean).join(" · ");
  const action = (id: string) => actions.find(entry => entry.id === id);
  const change = (patch: Partial<Agent>) => onChange({ ...agent, ...patch });
  const placeholders = presentation?.placeholders;
  const altId = useId();
  // Save is there when the daemon allows it, or while its answer for a fresh draft is on the way.
  const canSave = canCommand && (props.actionsLoading === true || isActionEnabled(actions, "save"));
  const shareAction = action("share");
  const shareReason = !saved ? "Save this agent before sharing it." : dirty || isNew ? "Save changes before sharing"
    : shareAction && !shareAction.enabled ? shareAction.reason ?? "Sharing is not available for this agent." : undefined;
  const archiveAction = presentation?.archived ? action("unarchive") : action("archive");
  const glyph: StageIcon = agent.icon ?? presentation?.glyph ?? "chip";
  return <section className="agent-editor" data-column="agents-editor" data-agent-id={agent.id} data-ready={presentation ? "true" : undefined} aria-label="Agent editor">
    <header className="belt-editor-header agent-editor-header">
      <div className="belt-editor-title">
        <p className="paper-tag" aria-label="Agent kind">{breadcrumb}</p>
        <h2 className={agent.name.trim() ? "" : "is-unnamed"}>{agent.name.trim() || "Unnamed agent"}</h2>
      </div>
      <div className="belt-actions">
        <button type="button" className="action-button slate" disabled={!!shareReason} title={shareReason ?? "Share this saved agent version to a library repository"} onClick={props.onShare}>Share…</button>
        {!isNew && !dirty && archiveAction && <button type="button" className="action-button" disabled={!canCommand || !archiveAction.enabled}
          title={archiveAction.reason ?? commandReason ?? (presentation?.archived ? "Let new stages use this agent again, and put it back in this list" : "Take it out of this list and the + menu. Belts that use it keep working.")}
          onClick={() => props.onArchive(!presentation?.archived)}>{archiveAction.label}</button>}
        {(dirty || isNew) && <>
          <button type="button" className="action-button" onClick={props.onDiscard}>Discard</button>
          {/* Save is there only when it can be used; offline, the banner says why. */}
          {canSave && <button type="button" className="action-button primary" aria-busy={props.saving || undefined}
            onClick={() => { if (!props.saving) props.onSave(); }}>{props.saving ? "Saving…" : "Save"}</button>}
        </>}
        {props.assistantButton}
      </div>
    </header>

    {!isNew && props.origin && <p className="agent-origin">Installed from {props.origin.repository}{props.origin.path ? ` / ${props.origin.path}` : ""}</p>}

    {(dirty || isNew) && presentation && <p className="belt-note agent-status"><span className="belt-note-lamp" aria-hidden="true" />{presentation.status}</p>}

    {commandError !== null && commandError !== undefined && <div className="agent-command-error" role="alert">
      <ProblemItems problems={describeError(commandError).problems} />
      <p>{describeError(commandError).message}</p>
      <button type="button" className="action-button small" onClick={onRetry}>Retry</button>
    </div>}

    {presentation && presentation.warnings.length > 0 && <ul className="belt-warnings agent-warnings" aria-label="Validation warnings">
      {presentation.warnings.map((warning, index) => <li key={index} className="agent-warning">{warning}</li>)}
    </ul>}

    <div className="agent-card belt-card">
      <div className="agent-card-head">
        <div className="form-heading"><span className="paper-tag">Agent</span><h3>What a new stage starts with</h3></div>
        <span className="agent-stage-preview" aria-label="How a stage with this agent looks">
          <StageBox icon={glyph} tint={agent.tint} caption="Agent" title={agent.name.trim() || "Unnamed agent"}
            details={[backendLabels[agent.runner]]} style={{ position: "relative", width: 170, height: 56 }} />
        </span>
      </div>
      <div className="agent-form">
        <div className="agent-form-row agent-form-row--pair">
          <label className="agent-field"><span className="agent-field-label">Id</span>
            {isNew
              ? <input className="paper-field" aria-label="Id" placeholder="lowercase-id" value={agent.id} onChange={event => change({ id: event.currentTarget.value })} />
              : <output className="paper-field agent-readonly" aria-label="Id" title="The id names the agent's folder and never changes. Rename it with Name.">{agent.id}</output>}
          </label>
          <label className="agent-field"><span className="agent-field-label">Name</span>
            <input className="paper-field" aria-label="Name" placeholder="Name" value={agent.name} onChange={event => change({ name: event.currentTarget.value })} />
          </label>
        </div>
        <div className="agent-form-row agent-form-row--pair">
          <label className="agent-field"><span className="agent-field-label">Icon</span>
            <select className="paper-field" aria-label="Icon" value={agent.icon ?? ""} onChange={event => change({ icon: event.currentTarget.value ? event.currentTarget.value as StageIcon : undefined })}>
              <option value="">Default (Chip)</option>
              {agentIconChoices.map(value => <option key={value} value={value}>{iconLabel(value)}</option>)}
            </select>
          </label>
          <label className="agent-field agent-tint" style={{ "--agent-tint": `var(--tint-${agent.tint})` } as CSSProperties}><span className="agent-field-label">Colour</span>
            <select className="paper-field" aria-label="Colour" value={agent.tint} onChange={event => change({ tint: event.currentTarget.value as Agent["tint"] })}>
              {tints.map(value => <option key={value} value={value}>{titleCase(value)}</option>)}
            </select>
          </label>
        </div>
        <div className="agent-form-row agent-form-row--pair">
          <label className="agent-field"><span className="agent-field-label">Workspace</span>
            <select className="paper-field" aria-label="Workspace" value={agent.workspace} onChange={event => change({ workspace: event.currentTarget.value as Agent["workspace"] })}>
              {workspaceValues.map(value => <option key={value} value={value}>{workspaceLabels[value]}</option>)}
            </select>
          </label>
          <label className="agent-field agent-slots"><span className="agent-field-label">Runs at once</span>
            <input className="paper-field" aria-label="Runs at once" type="number" min={1} max={6} value={agent.slots}
              onChange={event => change({ slots: Math.trunc(Number(event.currentTarget.value)) || 0 })} />
          </label>
        </div>
        <p className="agent-hint">{agent.workspace === "worktree"
          ? "It works in the run's worktree, so it changes code: stages after it can read the change, and Ship can open a pull request."
          : "It can't change the repository. Beltline checks and throws away any change it makes."}</p>
        <div className="agent-model-list">
          <ModelColumnHeadings />
          <div className="agent-form-row agent-form-row--model">
            <RunnerFields idPrefix="primary" runner={agent.runner} model={agent.model} thinking={agent.thinking} runnerDisabled={!canCommand}
              onModel={model => change({ model })} onThinking={thinking => change({ thinking: thinking ?? undefined })}
              onChangeRunner={runner => onChangeRunner(runner)} />
          </div>
          {agent.alternates.map((alternate, index) => <div key={`${altId}-${index}`} className="agent-form-row agent-form-row--model">
            <RunnerFields idPrefix={`alternate-${index}`} runner={alternate.runner ?? agent.runner} model={alternate.model} thinking={alternate.thinking} runnerDisabled={!canCommand}
              onModel={model => change({ alternates: agent.alternates.map((entry, at) => at === index ? { ...entry, model } : entry) })}
              onThinking={thinking => change({ alternates: agent.alternates.map((entry, at) => at === index ? { ...entry, thinking: thinking ?? undefined } : entry) })}
              onChangeRunner={runner => onChangeRunner(runner, index)} />
            <button type="button" className="quiet-button agent-form-remove" aria-label={`Remove alternate ${index + 1}`}
              onClick={() => change({ alternates: agent.alternates.filter((_, at) => at !== index) })}>Remove</button>
          </div>)}
        </div>
        <div>
          <button type="button" className="action-button small" onClick={() => change({ alternates: [...agent.alternates, { model: "", runner: agent.runner }] })}>+ Add a model</button>
        </div>
      </div>
    </div>

    <div className="agent-card belt-card">
      <div className="form-heading"><span className="paper-tag">Brief</span><h3>What the agent is told</h3></div>
      <textarea className="paper-field agent-brief" aria-label="Brief" rows={12} value={agent.brief} onChange={event => change({ brief: event.currentTarget.value })} />
      <p className="agent-hint">Write this agent&apos;s current result to {"{output}"}. Name other outputs with the placeholders chosen on the belt&apos;s stages. The issue, any note, and the outcome footer are added automatically.</p>
      {placeholders && <div className="agent-placeholders">
        <span className="paper-caps">Placeholders</span>
        <ul>{placeholders.entries.map(entry => <li key={entry.name}><code>{`{${entry.name}}`}</code> — {entry.description}</li>)}</ul>
      </div>}
    </div>

    <div className="agent-card belt-card">
      <div className="agent-outcomes-header"><div className="form-heading"><span className="paper-tag">Outcomes</span><h3>How it can end</h3>
        <p className="agent-lead">The agent picks one and writes it as the last line of its output, so describe when each one applies. A new stage starts with these; “Go to a stage” goes to a stage with one of the agents you pick: the nearest earlier one, or else the nearest later one.</p></div>
        <button type="button" className="action-button small"
          onClick={() => change({ outcomes: [...agent.outcomes, { name: "", when: "", tone: "neutral", then: { do: "hold" }, to: [] }] })}>+ Add outcome</button>
      </div>
      <div className="outcomes-scroll" data-scroll-viewport>
        <table className="outcomes-table">
          <caption className="screen-reader-only">Outcomes</caption>
          <thead><tr>
            <th scope="col">Outcome</th><th scope="col">Pick it when</th><th scope="col">Tone</th><th scope="col">Then</th><th scope="col"><span className="screen-reader-only">Remove</span></th>
          </tr></thead>
          <tbody>
            {agent.outcomes.map((outcome, index) => <AgentOutcomeRow key={index} outcome={outcome} index={index} targets={presentation?.outcomeTargets ?? []}
              onChange={next => change({ outcomes: agent.outcomes.map((entry, at) => at === index ? next : entry) })}
              onRemove={() => change({ outcomes: agent.outcomes.filter((_, at) => at !== index) })} />)}
          </tbody>
        </table>
      </div>
      {presentation && <div className="agent-footer">
        <span className="paper-tag">Added to the end of the brief</span>
        <pre>{presentation.footer}</pre>
      </div>}
    </div>

    {!isNew && <div className="agent-card belt-card">
      <div className="form-heading"><span className="paper-tag">Belts</span><h3>{presentation && presentation.usedBy.length > 0 ? `Used by ${presentation.usedBy.length} ${presentation.usedBy.length === 1 ? "belt" : "belts"}` : "No belt uses it yet"}</h3></div>
      {presentation && presentation.usedBy.length > 0 && <ul className="agent-usage">{presentation.usedBy.map(use => <li key={use.beltId}>
        <span className="agent-usage-name">{use.beltName}</span>
        <code>v{use.version}</code>
        {use.version < agent.version && <span className="agent-hint">Open the belt and pick “Update to v{agent.version}” on its stage to move it.</span>}
        <a className="action-button small agent-open-belt" href={`/belts?belt=${encodeURIComponent(use.beltId)}`} onClick={event => { event.preventDefault(); props.onOpenBelt(use.beltId); }}>Open belt</a>
      </li>)}</ul>}
      {saved?.agent.key && <p className="agent-hint">Legacy key <code>{saved.agent.key}</code> still renders older runs.</p>}
    </div>}

    {!isNew && props.folder && <AgentFolderRow agentId={agent.id} version={agent.version} folder={props.folder} />}
  </section>;
}

/** The agent's folder, with the Finder reveal the old app offered next to it. */
function AgentFolderRow({ agentId, version, folder }: { agentId: string; version: number; folder: string }) {
  const command = useCommand();
  const reveal = () => command.submit({
    path: "/api/v1/desktop/reveal", body: { kind: "agent", agentId }, affects: [], publish: () => {}
  });
  return <div className="agent-field agent-folder">
    <span className="agent-field-label">Folder</span>
    <div className="agent-folder-line">
      <code className="agent-folder-path" title={folder}>{folder}</code>
      <button type="button" className="paper-button paper-button--small" disabled={command.pending} onClick={reveal}
        title={`Each version is a Markdown file. Add a v${version + 1}.md by hand and it shows here after a restart.`}>Open folder</button>
    </div>
    {command.error ? <ErrorProblems error={command.error} /> : null}
  </div>;
}
