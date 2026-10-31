import type { AgentOutcome, AgentOutcomeTarget } from "../../api/wire.gen";
import { ThenControls, ToggleMenu, toneLabels } from "../../components/ThenControls";

const tones = ["good", "you", "problem", "neutral"] as const;
/** Agents never take a route: only a belt has forks. */
const thenActions = ["next", "goto", "retry", "hold", "park", "done", "ship", "drop"] as const;
const thenLabels: Record<(typeof thenActions)[number], string> = {
  next: "Next stage", goto: "Go to a stage", retry: "Run again",
  hold: "Hold for me", park: "Park", done: "Done", ship: "Ship", drop: "Drop"
};

export function AgentOutcomeRow({ outcome, index, targets, onChange, onRemove }: {
  outcome: AgentOutcome;
  index: number;
  /** The agents the daemon lists as possible targets, already without this agent. */
  targets: readonly AgentOutcomeTarget[];
  onChange: (outcome: AgentOutcome) => void;
  /** Absent for a read-only agent, which has no remove button. */
  onRemove?: () => void;
}) {
  const number = index + 1;
  const toggleTarget = (id: string, on: boolean) => {
    const next = on ? [...outcome.to.filter(value => value !== id), id] : outcome.to.filter(value => value !== id);
    onChange({ ...outcome, to: next });
  };
  return <tr data-tone={outcome.tone}>
    <td><input aria-label={`Outcome ${number} name`} placeholder="name" value={outcome.name} onChange={event => onChange({ ...outcome, name: event.currentTarget.value })} /></td>
    <td><textarea aria-label={`Outcome ${number} when`} rows={1} placeholder="Describe when the agent should pick this" value={outcome.when}
      onChange={event => onChange({ ...outcome, when: event.currentTarget.value })} /></td>
    <td><select aria-label={`Outcome ${number} tone`} value={outcome.tone} onChange={event => onChange({ ...outcome, tone: event.currentTarget.value as AgentOutcome["tone"] })}>
      {tones.map(tone => <option key={tone} value={tone}>{toneLabels[tone]}</option>)}
    </select></td>
    <td className="outcome-then">
      <ThenControls number={number} then={outcome.then} actions={thenActions.map(id => ({ id, label: thenLabels[id] }))} disabled={false}
        onAction={action => onChange({ ...outcome, then: { ...outcome.then, do: action } })}
        onChange={change => onChange({ ...outcome, then: change(outcome.then) })}
        target={<ToggleMenu label={`Outcome ${number} agents`} placeholder="Pick agents" options={targets} selected={outcome.to} disabled={false} onToggle={toggleTarget} />} />
    </td>
    <td className="outcome-remove">
      {onRemove && <button type="button" className="outcome-remove-button" aria-label={`Remove outcome ${number}`} title={`Remove ${outcome.name || "this outcome"}`} onClick={onRemove}>×</button>}
    </td>
  </tr>;
}
