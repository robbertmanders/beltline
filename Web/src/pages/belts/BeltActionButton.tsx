import { useId, type ReactNode } from "react";

/** Whether a belt action is available, and the reason it is not when it is not. */
export interface Gate {
  enabled: boolean;
  reason: string | null;
}

/** A pill button; when it is unavailable its reason is its help, and screen readers hear it as its description. */
export function ActionButton({ label, gate, kind = "", onClick, ariaLabel, help }: { label: ReactNode; gate: Gate; kind?: string; onClick: () => void; ariaLabel?: string; help?: string }) {
  const reasonId = useId();
  return <span className="belt-action">
    <button type="button" className={`btn ${kind}`} disabled={!gate.enabled} aria-label={ariaLabel}
      aria-describedby={gate.reason ? reasonId : undefined} title={gate.reason ?? help} onClick={onClick}>{label}</button>
    {gate.reason && <span id={reasonId} className="screen-reader-only">{gate.reason}</span>}
  </span>;
}
