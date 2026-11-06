import { useEffect, useId, useState, type ReactNode } from "react";

import type { ConnectionStatus } from "../api/eventConnection";
import { isGlobalActionId } from "../api/actions";

/** The shape every daemon action record shares, global or run-scoped. */
export interface ActionControlAction {
  id: string;
  label?: string;
  enabled: boolean;
  reason?: string | null;
  /** What the control does, shown as its help while it is available. */
  help?: string;
}

/** The app's button kinds: plain, primary (ink), slate and danger pills. */
export type ButtonKind = "plain" | "primary" | "slate" | "danger";

export interface ActionControlProps {
  /** The daemon action this control stands for. It is never submitted when the daemon does not offer it. */
  id: string;
  fallbackLabel: string;
  action: ActionControlAction | undefined;
  canRead: boolean;
  connection: ConnectionStatus;
  pending: boolean;
  primary?: boolean;
  kind?: ButtonKind;
  /** Whether this client knows the action's endpoint. Defaults to the shell's global commands. */
  supported?: boolean;
  /** Extra class names. */
  className?: string;
  /** The smaller button. */
  small?: boolean;
  /** Draws the button as this icon alone; the label stays its accessible name and its tooltip. */
  icon?: ReactNode;
  /** Shows the control as a switch: its checked state, its icon, whether only the icon shows, and a shorter visible label. */
  switchOptions?: { checked: boolean; icon: ReactNode; iconOnly: boolean; text?: string };
  onActivate: () => void;
}

/**
 * One global control. It is available only when the daemon offers its action as enabled, the API version
 * is compatible and the connection is online. Every reason it is unavailable is written out for keyboard
 * and screen-reader users, not only shown as a tooltip.
 */
export function ActionControl({ id, fallbackLabel, action, canRead, connection, pending, primary, kind, supported: supportedProp, className, small, icon, switchOptions, onActivate }: ActionControlProps) {
  const explanationId = useId();
  const [showWorking, setShowWorking] = useState(false);
  useEffect(() => {
    if (!pending) { setShowWorking(false); return; }
    // Fast commands finish without flashing a temporary label.
    const timer = window.setTimeout(() => setShowWorking(true), 150);
    return () => window.clearTimeout(timer);
  }, [pending]);
  const label = action?.label ?? fallbackLabel;
  const supported = supportedProp ?? isGlobalActionId(id);
  const restrictions: string[] = [];
  if (!action) restrictions.push("The daemon does not offer this action.");
  else if (!supported) restrictions.push("Unsupported action.");
  if (!canRead) restrictions.push("Waiting for the daemon API version.");
  else if (connection !== "connected") restrictions.push("Unavailable while offline or reconnecting.");
  if (action?.reason) restrictions.unshift(action.reason);
  const available = Boolean(action && supported && action.enabled && canRead && connection === "connected");
  const disabled = !available || pending;
  const look = kind ?? (primary ? "primary" : "plain");
  const explanation = restrictions.join(" ");
  // As in the app, why a control is unavailable shows as its help; screen readers hear it as its description.
  const explanationText = <span className="action-explanation screen-reader-only" id={explanationId}>{explanation}</span>;
  if (switchOptions) {
    // A switch keeps its label and says its state through aria-checked; the daemon's action still decides whether it works.
    return <div className="nav-switch">
      <button type="button" role="switch" aria-checked={switchOptions.checked} aria-label={switchOptions.iconOnly || switchOptions.text ? fallbackLabel : undefined}
        className={["tg action-switch", className ?? ""].filter(Boolean).join(" ")} disabled={disabled} aria-busy={pending} aria-describedby={explanationId}
        title={explanation || action?.help || fallbackLabel} onClick={onActivate}>
        {switchOptions.icon}
        <span className="t" aria-hidden="true" />
        {!switchOptions.iconOnly && <span className="action-switch-label">{switchOptions.text ?? fallbackLabel}</span>}
      </button>
      {explanationText}
    </div>;
  }
  return <div className="global-action">
    <button type="button" className={["btn", small ? "sm" : "", icon ? "icon" : "", look === "primary" || look === "slate" ? "fill" : look === "danger" ? "danger" : "", className ?? ""].filter(Boolean).join(" ")} disabled={disabled} aria-busy={pending} aria-describedby={explanationId}
      aria-label={icon ? (pending && showWorking ? "Working…" : label) : undefined} title={icon ? [label, explanation || action?.help].filter(Boolean).join(". ") : explanation || action?.help || undefined} onClick={onActivate}>
      {icon ?? <span className="btn-label" data-label={label}><span>{pending && showWorking ? "Working…" : label}</span></span>}
    </button>
    {explanationText}
  </div>;
}
