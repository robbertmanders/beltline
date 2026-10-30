import { useEffect, useRef, useState, type ReactNode } from "react";

/** The part of an outcome's `then` these controls edit; belts and agents both carry it. */
export interface ThenFields {
  do: "next" | "goto" | "route" | "retry" | "hold" | "park" | "done" | "ship" | "drop";
  withOutput?: boolean;
  max?: number;
  comment?: boolean;
}

/** The label of each outcome tone, read by the belt and agent outcome editors alike. */
export const toneLabels: Record<"good" | "you" | "problem" | "neutral", string> = {
  good: "Good", you: "Needs you", problem: "Problem", neutral: "Neutral"
};

/**
 * What an outcome does next, as the app laid it out: the action with its target beside it, then on their own
 * lines "pass the output along" (or "ask me for a note" on an approval, and nothing on a prompt stage, whose
 * prompt reaches every later stage as the run's task), "at most N ×" and the hold's comment.
 */
export function ThenControls<T extends ThenFields>({ number, then, actions, gate = false, passesOn = true, disabled, onAction, onChange, target }: {
  number: number;
  then: T;
  actions: readonly { id: string; label: string }[];
  gate?: boolean;
  /** Whether the outcome can pass text on as a note; false on a prompt stage. */
  passesOn?: boolean;
  disabled: boolean;
  onAction: (action: T["do"]) => void;
  onChange: (change: (then: T) => T) => void;
  target?: ReactNode;
}) {
  const action = then.do;
  return <div className="then-controls">
    <div className="then-controls-head">
      <select className="then-action" aria-label={`Outcome ${number} then`} value={action} disabled={disabled}
        onChange={event => onAction(event.target.value as T["do"])}>
        {actions.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
      </select>
      {(action === "goto" || action === "route") && target}
    </div>
    {passesOn && (action === "goto" || action === "route" || action === "retry" || action === "next") && <label className="then-toggle"
      title="Pass this stage's output to the next attempt as a note">
      <input type="checkbox" checked={then.withOutput === true} disabled={disabled}
        onChange={event => { const on = event.target.checked; onChange(current => ({ ...current, withOutput: on })); }} />
      <span>{gate ? "ask me for a note" : "pass the output along"}</span>
    </label>}
    {(action === "goto" || action === "retry") && <div className="then-max">
      <span>at most</span>
      <input type="number" min={0} max={9} placeholder="0" aria-label={`Outcome ${number} at most`} value={then.max ?? 0} disabled={disabled}
        onChange={event => { const value = Math.min(Math.max(Number(event.target.value) || 0, 0), 9); onChange(current => ({ ...current, max: value > 0 ? value : undefined })); }} />
      <span>×</span>
      <span className="then-max-note">0 = no limit</span>
    </div>}
    {action === "hold" && <label className="then-toggle">
      <input type="checkbox" checked={then.comment === true} disabled={disabled}
        onChange={event => { const on = event.target.checked; onChange(current => ({ ...current, comment: on })); }} />
      <span>comment on the issue or ticket</span>
    </label>}
  </div>;
}

/**
 * A pop-up of toggles, as the app's Menu of Toggles: a compact button naming what is picked, and a card of
 * checkboxes that stays open while you pick several.
 */
export function ToggleMenu({ label, placeholder, options, selected, disabled, onToggle }: {
  label: string;
  placeholder: string;
  options: readonly { id: string; label: string }[];
  selected: readonly string[];
  disabled: boolean;
  onToggle: (id: string, on: boolean) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => { if (!ref.current?.contains(event.target as Node)) setOpen(false); };
    const key = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", key);
    return () => { document.removeEventListener("mousedown", close); document.removeEventListener("keydown", key); };
  }, [open]);
  const names = selected.map(id => options.find(option => option.id === id)?.label ?? id);
  const summary = names.length === 0 ? placeholder : names.join(", ");
  return <div className="toggle-menu" ref={ref}>
    <button type="button" className={`toggle-menu-button${names.length === 0 ? " is-empty" : ""}`} aria-label={label} aria-haspopup="menu" aria-expanded={open}
      title={summary} disabled={disabled} onClick={() => setOpen(value => !value)}>
      <span>{summary}</span>
    </button>
    {open && <div className="toggle-menu-card" role="menu" aria-label={label}>
      {options.length === 0 && <span className="toggle-menu-empty">Nothing to pick</span>}
      {options.map(option => <label key={option.id} className="toggle-menu-item">
        <input type="checkbox" role="menuitemcheckbox" aria-checked={selected.includes(option.id)} checked={selected.includes(option.id)}
          onChange={event => onToggle(option.id, event.target.checked)} />
        <span>{option.label}</span>
      </label>)}
    </div>}
  </div>;
}
