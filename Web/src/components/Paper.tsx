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
  // Plex Mono draws ✓ like a root sign, so the tick comes from the system font.
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

export type NavIconKind = "overview" | "work" | "insights" | "belts" | "agents" | "library" | "afk" | "settings";

/** The navigation's icons, drawn on a 16 × 16 grid as in the app. */
export function NavIcon({ kind, size = 16 }: { kind: NavIconKind; size?: number }) {
  return <svg className="nav-icon" width={size} height={size} viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor"
    strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round">{navIconShapes[kind]}</svg>;
}

const navIconShapes: Record<NavIconKind, ReactNode> = {
  overview: <>{[[2, 2], [9, 2], [2, 9], [9, 9]].map(([x, y]) => <rect key={`${x}-${y}`} x={x} y={y} width={5} height={5} rx={1} />)}</>,
  work: <><rect x={1.5} y={10} width={13} height={3} rx={1.5} /><rect x={5.5} y={4} width={5} height={5} rx={1} /></>,
  // Three bars rising from a base line: how the belts do.
  insights: <><path d="M2 14H14" /><rect x={3} y={8} width={2.6} height={4} rx={0.6} /><rect x={6.7} y={4.5} width={2.6} height={7.5} rx={0.6} /><rect x={10.4} y={2} width={2.6} height={10} rx={0.6} /></>,
  belts: <><circle cx={3.5} cy={8} r={2} /><circle cx={12.5} cy={8} r={2} /><path d="M5.5 8H10.5" /></>,
  agents: <><rect x={2.5} y={3.5} width={11} height={10} rx={2.5} /><circle cx={5.8} cy={7.8} r={0.8} /><circle cx={10.2} cy={7.8} r={0.8} /><path d="M8 1.5V3.5" /></>,
  // Three volumes on a shelf, with one tipped over: the library of belts and agents.
  library: <><rect x={2.5} y={3} width={3} height={10.5} rx={0.8} /><rect x={6.5} y={3} width={3} height={10.5} rx={0.8} /><path d="M11.2 3.6L13.7 3.1L14.6 12.6L12.1 13.1Z" /></>,
  // A crescent moon: the outer arc bows left, the inner arc cuts a concave edge, meeting at the tips.
  afk: <path d="M12.538 3.004A5.9 5.9 0 1 0 12.538 12.996A5.25 5.25 0 0 1 12.538 3.004Z" />,
  settings: <><circle cx={8} cy={8} r={2.2} /><path d="M8 1.8V3.8M8 12.2V14.2M1.8 8H3.8M12.2 8H14.2M3.6 3.6L5 5M11 11L12.4 12.4M3.6 12.4L5 11M11 5L12.4 3.6" /></>
};

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
