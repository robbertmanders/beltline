import { useEffect, useRef, useState, type FormEvent } from "react";
import { triggerKindValues } from "../../api/wire.gen";
import type { Belt, TriggerEntry, TriggerFormPresentation } from "../../api/wire.gen";
import { unchangedField, useDraftEditor, useTypedForm } from "./draftEditor";
import { sameBelt, type TriggerStash } from "./beltDrafts";

type Kind = TriggerEntry["trigger"]["kind"];

/** Kind names for the list rows, which are drawn before the editor's context is read. The form uses the daemon's. */
const triggerLabels: Record<Kind, string> = {
  issues: "GitHub issues",
  reviews: "Review requests",
  jira: "Jira",
  schedule: "Schedule",
  manual: "Manual task"
};

/** The manual entries of a belt, each with its original position among all the triggers. */
export function manualEntries(belt: Belt): Array<{ entry: TriggerEntry; position: number }> {
  return belt.triggers.flatMap((entry, position) => entry.trigger.kind === "manual" ? [{ entry, position: position + 1 }] : []);
}

type Choice = { value: string; label: string };

/** The presentation's choices, with a saved value it does not list kept and named by its value, so it is never replaced unseen. */
function withCurrent(choices: readonly Choice[], current: string): readonly Choice[] {
  return current === "" || choices.some(choice => choice.value === current) ? choices : [...choices, { value: current, label: current }];
}

/**
 * The trigger section as the app drew it: one card with the entries as numbered chips, Add trigger and the selected
 * entry's Move up, Move down and Remove; then the selected entry's own card. Every change is a daemon edit of the
 * draft. A pick applies at once; typed text applies on Enter or when the field loses focus. Headings, notes and
 * choices are the daemon's presentation.
 */
export function TriggerForm() {
  const editor = useDraftEditor();
  const reason = editor.blocked;
  const disabled = reason !== null;
  const presentation = editor.context?.triggerForm ?? null;
  const triggers = editor.belt.triggers;
  const count = triggers.length;
  const several = count > 1;
  const [chosen, setChosen] = useState<string | null>(null);
  const selectedIndex = Math.max(0, triggers.findIndex(entry => entry.id === chosen));
  const selected = triggers[selectedIndex];
  // A trigger added from here becomes the selected one, as in the app.
  const added = useRef(count);
  useEffect(() => {
    if (count > added.current && triggers[count - 1]) setChosen(triggers[count - 1]!.id);
    added.current = count;
  }, [count, triggers]);
  const kindOf = (entry: TriggerEntry) => presentation?.kinds.find(item => item.id === entry.trigger.kind);
  return <>
    <section className="belt-card belt-triggers" aria-label="Triggers">
      <div className="belt-triggers-head">
        <div className="form-heading"><span className="paper-tag">{several ? `Triggers · ${count}` : "Trigger"}</span><h3>{presentation?.heading ?? "Trigger"}</h3></div>
        {!disabled && <button type="button" className="action-button small" onClick={() => editor.operate("trigger-add", {})}>Add trigger</button>}
      </div>
      {reason && !editor.readOnly && <span id="trigger-add-reason" className="action-reason">{reason}</span>}
      {count === 0 ? <p className="field-note">{presentation?.empty ?? "Add at least one trigger"}</p> : <>
        <div className="trigger-chips">
          {triggers.map((entry, index) => <button key={entry.id} type="button" title={kindOf(entry)?.heading}
            className={`action-button small${entry.id === selected?.id ? " slate" : ""}`} aria-pressed={entry.id === selected?.id}
            onClick={() => setChosen(entry.id)}>{`${index + 1} · ${kindOf(entry)?.title ?? triggerLabels[entry.trigger.kind]}`}</button>)}
        </div>
        {!disabled && selected && <div className="trigger-controls">
          <button type="button" className="action-button small" disabled={selectedIndex === 0} aria-label={`Move trigger ${selectedIndex + 1} earlier`}
            onClick={() => editor.operate("trigger-move", { entryId: selected.id, offset: -1 })}>Move up</button>
          <button type="button" className="action-button small" disabled={selectedIndex === count - 1} aria-label={`Move trigger ${selectedIndex + 1} later`}
            onClick={() => editor.operate("trigger-move", { entryId: selected.id, offset: 1 })}>Move down</button>
          <button type="button" className="action-button small" aria-label={`Remove trigger ${selectedIndex + 1} ${triggerLabels[selected.trigger.kind]}`}
            onClick={() => { editor.operate("trigger-remove", { entryId: selected.id }); setChosen(null); }}>Remove</button>
        </div>}
      </>}
    </section>
    {selected && <TriggerEntryEditor key={selected.id} entry={selected} position={selectedIndex + 1} several={several}
      stash={editor.stash} presentation={presentation} />}
  </>;
}

/** The settings a kind's form edits, as text, from the trigger the draft holds. */
function settingsOf(trigger: Record<string, unknown>): Record<string, string> {
  return Object.fromEntries(["label", "assignee", "poll", "reviewers", "jql"].flatMap(field =>
    trigger[field] === undefined || trigger[field] === null ? [] : [[field, String(trigger[field])]]));
}

function TriggerEntryEditor({ entry, position, several, stash, presentation }: {
  entry: TriggerEntry;
  position: number;
  several: boolean;
  stash: TriggerStash | undefined;
  presentation: TriggerFormPresentation | null;
}) {
  const editor = useDraftEditor();
  const kind = entry.trigger.kind;
  const trigger = entry.trigger as Record<string, unknown>;
  const info = presentation?.kinds.find(item => item.id === kind);
  // Typed settings are kept per kind, so switching kinds and back finds them again, as the stash does for the draft.
  const settings = useTypedForm(`trigger:${entry.id}:${kind}`, settingsOf(trigger));
  const repo = useTypedForm(`repository:${entry.id}`, { text: "" } as Record<string, unknown>);
  const disabled = editor.blocked !== null;
  const values = settings.values;
  const repoText = String(repo.values.text ?? "");

  const switchKind = (next: Kind) => {
    const kept = stash && stash.entry === entry.id ? stash : { entry: entry.id, triggers: {} };
    const updated: TriggerStash = { entry: entry.id, triggers: { ...kept.triggers, [kind]: trigger } };
    editor.setStash(updated);
    editor.operate("trigger-kind", { entryId: entry.id, kind: next, stash: updated });
  };

  const apply = (override: Record<string, string> = {}) => {
    const next: Record<string, unknown> = { ...trigger };
    for (const [field, value] of Object.entries({ ...values, ...override })) next[field] = field === "poll" ? Number(value) : value;
    editor.operate("trigger-update", { entryId: entry.id, trigger: next }, settings.applies());
  };

  // The text goes as typed; the daemon normalizes it, and leaves the entry alone for input that is not
  // owner/repository or is already listed. Then the text stays, as in the native form.
  const addRepository = (event: FormEvent) => {
    event.preventDefault();
    if (repoText.trim() === "" || disabled) return;
    const before = entry.repos;
    editor.operate("repository-add", { entryId: entry.id, repository: repoText }, {
      ...repo.applies(),
      took: answer => !sameBelt(answer.triggers.find(item => item.id === entry.id)?.repos ?? [], before)
    });
  };
  const notAdded = repo.values[unchangedField] === true;

  const select = (name: string, label: string, choices: readonly Choice[]) => <label key={name} className="belt-field"><span>{label}</span>
    <select id={`trigger-${entry.id}-${name}`} aria-label={`${label} for trigger ${position}`} value={values[name] ?? ""} disabled={disabled}
      onChange={event => { const value = event.target.value; settings.set(name, value); apply({ [name]: value }); }}>
      {withCurrent(choices, values[name] ?? "").map(choice => <option key={choice.value} value={choice.value}>{choice.label}</option>)}
    </select>
  </label>;
  // Typed text stays in the browser until Enter sends it or the field loses focus (the JQL, which takes several lines, only then).
  const typedApply = { onKeyDown: (event: { key: string; preventDefault: () => void }) => { if (event.key === "Enter") { event.preventDefault(); apply(); } } };

  const kindFields = kind === "issues" ? <div className="belt-field-row">
    <label className="belt-field"><span>Has label</span>
      <input id={`trigger-${entry.id}-label`} className="paper-field" aria-label={`Has label for trigger ${position}`} placeholder="agent" value={values.label ?? ""} disabled={disabled}
        onChange={event => settings.set("label", event.target.value)} onBlur={() => { if (settings.typed) apply(); }} {...typedApply} />
    </label>
    {select("assignee", "Assigned to", presentation?.assignees ?? [])}
  </div> : kind === "reviews" ? select("reviewers", "Review asked of", presentation?.reviewers ?? [])
    : kind === "jira" ? <label className="belt-field"><span>JQL</span>
      <textarea id={`trigger-${entry.id}-jql`} className="paper-field trigger-jql" aria-label={`JQL for trigger ${position}`} rows={1} placeholder="project = PAY AND labels = agent"
        value={values.jql ?? ""} disabled={disabled} onChange={event => settings.set("jql", event.target.value)} onBlur={() => { if (settings.typed) apply(); }} />
    </label> : null;

  return <section id={`trigger-${entry.id}`} className="belt-card trigger-entry" data-trigger-id={entry.id} aria-label={`Trigger ${position}`}>
    {several && <div className="form-heading"><span className="paper-tag">{`Trigger ${position}`}</span><h3>{info?.heading ?? ""}</h3></div>}
    <div className="belt-field-row">
      <label className="belt-field"><span>Work comes from</span>
        <select aria-label={`Kind of trigger ${position}`} {...editor.problemTarget("trigger", entry.id)} value={kind} disabled={disabled} onChange={event => switchKind(event.target.value as Kind)}>
          {(presentation?.kinds ?? triggerKindValues.map(id => ({ id, title: triggerLabels[id] }))).map(item => <option key={item.id} value={item.id}>{item.title}</option>)}
        </select>
      </label>
      {info?.polls ? select("poll", "Check every", presentation?.pollChoices ?? []) : <span />}
    </div>
    {kindFields}
    {kind === "schedule" && <ScheduleSettings entry={entry} position={position} disabled={disabled} presentation={presentation} />}
    {info?.note && <p className="field-note">{info.note}</p>}

    {!editor.readOnly && <div className="belt-field trigger-repositories">
      <span>{info?.repositoriesTitle ?? "Repositories"}</span>
      <form className="repo-chips" aria-label={`Repositories for trigger ${position}`} onSubmit={addRepository}>
        {entry.repos.map(name => <span key={name} id={`repository-${entry.id}-${name}`} className="repo-chip">
          <span>{name}</span>
          <button type="button" disabled={disabled} aria-label={`Remove ${name} from trigger ${position}`} title={`Remove ${name}`} {...editor.problemTarget("repository", entry.id, name)}
            onClick={() => editor.operate("repository-remove", { entryId: entry.id, repository: name })}>×</button>
        </span>)}
        {(!info?.oneRepo || entry.repos.length === 0) && <input aria-label={`Add a repository to trigger ${position}`} placeholder="owner/repository, then Enter" value={repoText} disabled={disabled}
          onChange={event => { repo.set("text", event.target.value); repo.set(unchangedField, undefined); }} />}
      </form>
      {notAdded && <p className="field-note" role="status">Not added: this is not owner/repository, or it is already listed.</p>}
    </div>}
  </section>;
}

/**
 * A schedule trigger's settings. Typing is local; Apply sends the whole trigger as one trigger-update. The summary,
 * the next run and the note about missed and overlapping times are the daemon's, for the draft as it is.
 */
function ScheduleSettings({ entry, position, disabled, presentation }: { entry: TriggerEntry; position: number; disabled: boolean; presentation: TriggerFormPresentation | null }) {
  const editor = useDraftEditor();
  const trigger = entry.trigger as unknown as { schedule?: Record<string, unknown>; title?: string; task?: string };
  const schedule = trigger.schedule ?? {};
  const form = useTypedForm(`schedule:${entry.id}`, {
    days: String(schedule.days ?? "weekdays"),
    weekday: String(schedule.weekday ?? 2),
    hour: String(schedule.hour ?? 9),
    minute: String(schedule.minute ?? 0),
    title: String(trigger.title ?? ""),
    task: String(trigger.task ?? "")
  });
  const { days, weekday, hour, minute, title, task } = form.values;
  const preview = editor.context?.schedules.find(item => item.entry === entry.id);
  const next = preview?.next ? new Date(preview.next) : null;

  const apply = (override: Partial<Record<"days" | "weekday" | "hour" | "minute", string>> = {}) => {
    const next = { days, weekday, hour, minute, ...override };
    editor.operate("trigger-update", {
      entryId: entry.id,
      trigger: { ...entry.trigger, schedule: { days: next.days, weekday: Number(next.weekday), hour: Number(next.hour), minute: Number(next.minute) }, title, task }
    }, form.applies());
  };
  const choose = (name: "days" | "weekday" | "hour" | "minute", label: string, choices: readonly Choice[]) =>
    <label className="belt-field"><span>{label}</span>
      <select aria-label={`${label} for trigger ${position}`} value={form.values[name]} disabled={disabled} onChange={event => { const value = event.target.value; form.set(name, value); apply({ [name]: value }); }}>
        {withCurrent(choices, form.values[name]).map(choice => <option key={choice.value} value={choice.value}>{choice.label}</option>)}
      </select>
    </label>;

  return <div className="schedule-settings" aria-label={`Schedule for trigger ${position}`}>
    <div className="belt-field-row">
      {choose("days", "Runs", presentation?.scheduleDays ?? [])}
      <div className="schedule-when">
        {days === "weekly" && choose("weekday", "On", presentation?.weekdays ?? [])}
        {choose("hour", "At", presentation?.hours ?? [])}
        {choose("minute", "Minute", presentation?.minutes ?? [])}
      </div>
    </div>
    <label className="belt-field"><span>Task</span>
      <input className="paper-field" aria-label={`Task title for trigger ${position}`} placeholder="Sweep flaky tests" value={title} disabled={disabled} onChange={event => form.set("title", event.target.value)}
        onBlur={() => { if (form.typed) apply(); }} onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); apply(); } }} />
    </label>
    <label className="belt-field"><span>What it should do</span>
      <textarea className="paper-field" aria-label={`Task for trigger ${position}`} rows={4} value={task} disabled={disabled} onChange={event => form.set("task", event.target.value)}
        onBlur={() => { if (form.typed) apply(); }} />
    </label>
    {preview && <p className="field-note schedule-summary" role="note">
      {next ? `Next run ${next.toLocaleString(undefined, { weekday: "long", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}. ` : ""}{preview.note}
    </p>}
  </div>;
}
