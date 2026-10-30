import { useId, type ReactNode } from "react";
import { findRunAction } from "../../api/runCommands";
import type { PromptFacts } from "../../api/wire.gen";
import { MarkdownBlocks } from "../../components/MarkdownBlocks";
import type { RunInlineControlsProps } from "./RunInlineControls";

/**
 * A prompt stage's editor: the run's task with the outcome buttons under it, and the work item beside it for you
 * to read. The words, the task a visit back starts with and the source choice's start come from the daemon; the
 * unsent prompt and checkbox are the tab's draft until an outcome sends them.
 */
export function RunPromptEntry({ prompt, failure, ...props }: RunInlineControlsProps & { prompt: PromptFacts; failure: ReactNode }) {
  const { view, stage, panel, draft, command, submit, canRead, connection } = props;
  const id = useId();
  const online = connection === "connected" && canRead;
  const pending = command.pending;
  const outcomes = panel.facts?.approval?.outcomes ?? [];
  const primaryId = panel.facts?.approval?.primary;
  const text = draft.prompt ?? prompt.task ?? "";
  const source = draft.source ?? prompt.source;
  return <div className="run-prompt">
    <div className="run-decide run-prompt-editor">
      <label className="paper-caps" htmlFor={`${id}-text`}>{prompt.label}</label>
      <p id={`${id}-help`} className="run-muted">{prompt.help}</p>
      <textarea id={`${id}-text`} className="paper-field" rows={16} placeholder={prompt.placeholder}
        aria-describedby={`${id}-help`} value={text} disabled={!online || pending}
        onChange={event => props.onPromptChange(event.currentTarget.value)} />
      <label className="run-prompt-source">
        <input type="checkbox" checked={source === "shared"} disabled={!online || pending} aria-describedby={`${id}-source`}
          onChange={event => props.onSourceChange(event.currentTarget.checked ? "shared" : "withheld")} />
        <span>{prompt.sourceLabel}</span>
      </label>
      <p id={`${id}-source`} className="run-muted">{prompt.sourceHelp}</p>
      <div className="run-decide-actions">
        {outcomes.map(outcome => {
          const action = findRunAction(view.actions, "decide", { stage: stage.id, outcome: outcome.name });
          const takesPrompt = action?.needsNote === true;
          return <button key={outcome.id} type="button"
            className={`action-button${outcome.id === primaryId ? " slate" : outcome.tone === "problem" ? " danger" : ""}`}
            disabled={!online || pending || !action?.enabled || (takesPrompt && text.trim() === "")}
            title={action?.reason ?? action?.thenHelp ?? undefined}
            onClick={() => submit("decide", takesPrompt
              ? { outcome: outcome.name, note: text, stage: stage.id, source }
              : { outcome: outcome.name, note: "", stage: stage.id })}>{outcome.label}</button>;
        })}
      </div>
      {failure}
    </div>
    <aside className="run-prompt-context" aria-label={prompt.contextLabel}>
      <span className="paper-caps">{prompt.contextLabel}</span>
      <MarkdownBlocks blocks={prompt.context} />
    </aside>
  </div>;
}
