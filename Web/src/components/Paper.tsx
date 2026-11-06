import type { ReactNode } from "react";

/** The look a status takes, as the daemon names it. */
export type Look = "done" | "running" | "waiting" | "held" | "plain";

/** `[ BRACKETED ]` mono tag, as `Tag` in the app. */
export function Tag({ children, strong, muted, className = "" }: { children: ReactNode; strong?: boolean; muted?: boolean; className?: string }) {
  return <span className={`paper-tag ${strong ? "paper-tag--strong" : ""} ${muted ? "paper-tag--muted" : ""} ${className}`}>{children}</span>;
}

/** Small uppercase mono label. */
export function Caps({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <span className={`paper-caps ${className}`}>{children}</span>;
}

/** A rounded mono chip. `look` colours it as a stage status; `tone` names one of the four tones. */
export function Chip({ children, look, tone, className = "", title }: {
  children: ReactNode; look?: Look; tone?: "good" | "you" | "problem" | "neutral" | "grey" | "gold" | "shadow"; className?: string; title?: string
}) {
  const variant = look ? `paper-chip--look-${look}` : tone ? `paper-chip--${tone}` : "";
  return <span className={`paper-chip ${variant} ${className}`} title={title}>{children}</span>;
}

/** The solid chip that marks a run needing you: slate, or red when held. */
export function NeedChip({ children, held }: { children: ReactNode; held?: boolean }) {
  return <span className={`paper-chip paper-chip--need ${held ? "paper-chip--held" : ""}`}>{children}</span>;
}

/** The status above a stage box: a mono pill coloured by how the stage is doing. */
export function StatusPill({ text, look, compact }: { text: string; look: Look; compact?: boolean }) {
  // The tick sits in its own span so it can be sized apart from the text.
  const tick = text.startsWith("✓ ");
  return <span className={`paper-status paper-status--${look} ${compact ? "paper-status--compact" : ""}`}>
    {tick ? <><span className="paper-tick">✓</span>&nbsp;{text.slice(2)}</> : text}
  </span>;
}

/** The small gold ring that turns next to a running session. */
export function Spinner() {
  return <span className="paper-spinner" aria-hidden="true" />;
}

/** The segmented control: pill buttons in a rounded track. */
export function Segmented<T extends string>({ options, value, onChange, label, title }: {
  options: readonly { value: T; label: string }[]; value: T; onChange: (value: T) => void; label: string; title?: string
}) {
  return <div className="paper-segmented" role="group" aria-label={label} title={title}>
    {options.map(option => <button key={option.value} type="button" className="paper-segmented__option" aria-pressed={value === option.value}
      onClick={() => onChange(option.value)}>{option.label}</button>)}
  </div>;
}

/** A pop-up menu drawn like the app's selects: grey, a hairline border, a chevron. `pill` and `compact` are its smaller looks. */
export function Select<T extends string>({ options, value, onChange, label, pill, compact, accent, disabled, placeholder = "", className = "", width }: {
  options: readonly { value: T; label: string }[]; value: T; onChange: (value: T) => void; label: string;
  pill?: boolean; compact?: boolean; accent?: string; disabled?: boolean; placeholder?: string; className?: string; width?: number | string
}) {
  const current = options.find(option => option.value === value)?.label ?? placeholder;
  const variant = pill ? "paper-select--pill" : compact ? "paper-select--compact" : "";
  return <label className={`paper-select ${variant} ${accent ? "paper-select--accent" : ""} ${disabled ? "is-readonly" : ""} ${className}`} style={width !== undefined ? { width } : undefined}>
    {accent && <span className="paper-select__accent" style={{ background: accent }} aria-hidden="true" />}
    <span className="paper-select__value">{current}</span>
    {!disabled && <span className="paper-select__chevron" aria-hidden="true"><svg width="9" height="9" viewBox="0 0 10 10"><path d="M2 3.5L5 6.5L8 3.5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg></span>}
    <select aria-label={label} value={value} disabled={disabled} onChange={event => onChange(event.currentTarget.value as T)}>
      {!options.some(option => option.value === value) && <option value={value}>{placeholder || value}</option>}
      {options.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
    </select>
  </label>;
}
