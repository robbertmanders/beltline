import { useId, type ReactNode } from "react";
import { Icon, type IconName } from "../../components/Icon";

/** Whether a belt action is available, and the reason it is not when it is not. */
export interface Gate {
  enabled: boolean;
  reason: string | null;
}

/**
 * A belt command: a pill button for what finishes an edit (Save, Discard, a shadow's decision), or, with an icon, a
 * quiet link with its icon, as the run page's links are. When it is unavailable its reason is its help, and screen
 * readers hear it as its description.
 */
export function ActionButton({ label, gate, kind = "", icon, onClick, ariaLabel, help }: { label: ReactNode; gate: Gate; kind?: string; icon?: IconName; onClick: () => void; ariaLabel?: string; help?: string }) {
  const reasonId = useId();
  return <span className="belt-action">
    <button type="button" className={icon ? `belt-link ${kind}` : `btn ${kind}`} disabled={!gate.enabled} aria-label={ariaLabel}
      aria-describedby={gate.reason ? reasonId : undefined} title={gate.reason ?? help} onClick={onClick}>{icon && <Icon name={icon} />}{label}</button>
    {gate.reason && <span id={reasonId} className="screen-reader-only">{gate.reason}</span>}
  </span>;
}
