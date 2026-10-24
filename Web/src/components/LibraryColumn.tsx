import { useState, type ReactNode } from "react";
import { loadPreferences, savePreferences } from "../state/preferences";

export interface LibraryRow {
  id: string;
  /** The row's accessible name and visible text. */
  label: string;
  /** Extra detail shown under the label. */
  detail?: ReactNode;
  /** A grey chip beside the title, such as PAUSED, ARCHIVED or NEW. */
  tag?: string;
  /** Drawn quieter, as a paused belt is. */
  dimmed?: boolean;
  title?: string;
  /** Drawn before the title and detail, as an agent's icon is. */
  leading?: ReactNode;
}

export interface LibraryColumnProps {
  title: string;
  rows: readonly LibraryRow[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  /** Adds a new row. The Add control is shown whenever this is supplied. */
  onAdd?: () => void;
  addLabel?: string;
  addDisabled?: boolean;
  addReason?: string;
  footer?: ReactNode;
  /** Shown in place of the rows when there are none. */
  empty: ReactNode;
  /** The preference key that remembers whether the column is folded. Folding is a browser display preference only. */
  foldKey: string;
}

/** Stores the fold state in the browser preferences. Nothing here reaches the daemon. */
function useFolded(foldKey: string): [boolean, (folded: boolean) => void] {
  const [folded, setFolded] = useState(() => loadPreferences().listFolded[foldKey] ?? false);
  const update = (next: boolean) => {
    setFolded(next);
    const preferences = loadPreferences();
    savePreferences({ ...preferences, listFolded: { ...preferences.listFolded, [foldKey]: next } });
  };
  return [folded, update];
}

/**
 * The Belts and Agents list column: a title with a fold control, independently scrolling rows, an Add
 * control and an optional footer. Folded, it becomes a 40px rail with a reopen control. Selection is kept
 * by the caller, so folding never changes which row is selected.
 */
export function LibraryColumn({ title, rows, selectedId, onSelect, onAdd, addLabel = "Add", addDisabled = false, addReason, footer, empty, foldKey }: LibraryColumnProps) {
  const [folded, setFolded] = useFolded(foldKey);
  if (folded) {
    return <aside className="library-column library-column-folded" data-column={`library-${foldKey}`} aria-label={title} style={{ width: 40 }}>
      <button type="button" className="library-rail" aria-label={`Show ${title}`} title={`Show the ${title.toLowerCase()} list`} onClick={() => setFolded(false)}>
        <span className="library-rail-mark">»</span>
        <span className="library-rail-title">{title.toUpperCase()}</span>
      </button>
    </aside>;
  }
  return <aside className="library-column" data-column={`library-${foldKey}`} aria-label={title}>
    <header className="library-header">
      <h2 className="paper-caps">{title}</h2>
      <button type="button" className="library-fold" aria-label={`Fold ${title}`} title="Fold the list away" onClick={() => setFolded(true)}>«</button>
    </header>
    <div className="library-rows" role="list">
      {rows.length === 0 ? <div className="library-empty">{empty}</div> : rows.map(row => <div role="listitem" key={row.id}>
        <button type="button" data-agent-id={row.id} className={`library-row ${row.id === selectedId ? "selected" : ""} ${row.dimmed ? "is-dimmed" : ""}`}
          aria-pressed={row.id === selectedId} title={row.title} onClick={() => onSelect(row.id)}>
          {row.leading && <span className="library-row-leading" aria-hidden="true">{row.leading}</span>}
          <span className="library-row-text">
            <span className="library-row-top"><span className="library-row-label">{row.label}</span>{row.tag && <span className="paper-chip paper-chip--grey">{row.tag}</span>}</span>
            {row.detail && <span className="library-row-detail">{row.detail}</span>}
          </span>
        </button>
      </div>)}
      {onAdd && <>
        <button type="button" className="library-add" disabled={addDisabled} title={addReason} onClick={onAdd}>{addLabel}</button>
        {addDisabled && addReason && <p className="agent-action-reason" role="status">{addReason}</p>}
      </>}
      {footer && <div className="library-footer">{footer}</div>}
    </div>
  </aside>;
}
