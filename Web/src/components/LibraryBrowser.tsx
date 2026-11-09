import { useLayoutEffect, useRef, type ReactNode } from "react";
import type { LibraryBrowser as Browser, LibraryCard } from "../api/wire.gen";
import { describeError } from "../api/errors";
import { useRuntime } from "../app/runtime";
import { StageIconTile } from "../theme/stages/StageBox";
import { countWord } from "../words";
import { Icon } from "./Icon";
import "./library-browser.css";

export type { LibraryBrowser as LibraryBrowserData, LibraryCard } from "../api/wire.gen";

export interface LibraryBrowserProps {
  title: string;
  lead: string;
  browser?: Browser;
  search: string;
  filter: string;
  onSearch: (value: string) => void;
  onFilter: (value: string) => void;
  onOpen: (id: string) => void;
  onNew: () => void;
  onRetry: () => void;
  busy?: boolean;
  loading?: boolean;
  error?: unknown;
  commandError?: unknown;
  onRetryCommand?: () => void;
  assistantButton?: ReactNode;
  drafts?: Array<{ id: string; name: string; onOpen: () => void }>;
  scrollKey?: string;
}

/** The headline's second line, from the daemon's filter counts: what is paused, or what no belt uses. */
function subline(title: string, browser: Browser | undefined): string {
  const count = (id: string) => browser?.filters.find(item => item.id === id)?.count ?? 0;
  if (!browser) return "";
  if (browser.matchCount !== browser.totalCount) return `${countWord(browser.matchCount)} shown.`;
  if (title === "Belts") {
    const paused = count("paused");
    return paused ? `${countWord(paused)} ${paused === 1 ? "is" : "are"} paused.` : "Every one is running.";
  }
  const unused = count("unused");
  return unused ? `${countWord(unused)} on no belt.` : "Each saved version stays as it was.";
}

// Scroll is a browser display preference, retained while navigating within this app.
const scrollPositions = new Map<string, number>();

/** A library catalog. Every card, count, filter and status comes from the daemon. */
export function LibraryBrowser({ title, lead, browser, search, filter, onSearch, onFilter, onOpen, onNew,
  onRetry, busy = false, loading = false, error, commandError, onRetryCommand, assistantButton, drafts = [], scrollKey = title }: LibraryBrowserProps) {
  const runtime = useRuntime();
  const element = useRef<HTMLElement>(null);
  const scrollID = `${scrollKey}:${search}:${filter}`;
  const ready = browser !== undefined;
  useLayoutEffect(() => {
    if (ready && element.current) element.current.scrollTop = scrollPositions.get(scrollID) ?? 0;
  }, [scrollID, ready]);
  const newAction = browser?.actions.find(action => action.id === "new");
  const connected = runtime.canRead && runtime.connection === "connected";
  const newReason = !connected ? "Commands are unavailable until the daemon connection is ready."
    : !newAction ? "The daemon did not offer this command."
    : !newAction.enabled ? newAction.reason ?? "This command is not currently available." : undefined;
  const singular = title === "Belts" ? "belt" : "agent";
  const hasError = error !== null && error !== undefined;

  const plural = title.toLowerCase();
  return <main className="pin library-browser" ref={element} data-column={`${singular}s-browser`}
    onScroll={event => scrollPositions.set(scrollID, event.currentTarget.scrollTop)}>
    <header className="ph library-browser-header">
      <h1 className="h1" aria-label={title}>{browser ? `${countWord(browser.totalCount)} ${browser.totalCount === 1 ? singular : plural}.` : `${title}.`}<br />
        <span className="soft">{subline(title, browser)}</span></h1>
      <p className="lede">{lead}</p>
      <div className="ph-row library-browser-actions">
        <button type="button" className="btn fill" disabled={newReason !== undefined || busy} title={newReason} onClick={onNew}><Icon name="plus" />{newAction?.label ?? `New ${singular}`}</button>
        {assistantButton}
      </div>
      <div className="ph-row library-browser-toolbar" style={{ gap: "18px 28px" }}>
        <div className="library-browser-search">
          <input className="inp" aria-label={`Search ${plural}`} placeholder={`Search ${plural}`} value={search}
            onChange={event => onSearch(event.currentTarget.value)} onKeyDown={event => { if (event.key === "Escape") onSearch(""); }} />
          {search && <button type="button" className="search-clear" aria-label={`Clear ${title} search`} onClick={() => onSearch("")}><Icon name="x" /></button>}
        </div>
        {browser && <div className="seg" role="group" aria-label={`${title} filter`}>
          {browser.filters.map(option => <button key={option.id} type="button" aria-pressed={filter === option.id} onClick={() => onFilter(option.id)}>{option.label} {option.count}</button>)}
        </div>}
      </div>
      {newReason && browser && <p className="why">{newReason}</p>}
    </header>
    {drafts.length > 0 && <section className="grp library-browser-drafts" aria-label="Unsent drafts">
      <h2 className="gh"><span className="gn">{drafts.length}</span>Unsent drafts</h2>
      <ol className="rows">{drafts.map(draft => <li key={draft.id} className="row" style={{ gridTemplateColumns: "minmax(0,1fr) auto" }}>
        <button type="button" className="row-hit library-draft" onClick={draft.onOpen} aria-label={`${draft.name || `Unnamed ${singular}`} Draft`} />
        <div className="t">{draft.name || `Unnamed ${singular}`}</div><span className="flag">Draft</span>
      </li>)}</ol>
    </section>}
    {loading && !browser && <p role="status" className="help">Loading {plural}…</p>}
    {hasError && <section role="alert" className="note error-panel">
      <div><h2 className="h3">{browser ? "Refresh failed" : `Unable to load ${plural}`}</h2><p>{describeError(error).message}</p></div>
      <button type="button" className="btn sm" disabled={busy || !runtime.canRead} onClick={onRetry}>Retry</button>
    </section>}
    {hasError && browser && <p role="status" className="why">Showing the last loaded library; refresh failed.</p>}
    {commandError !== null && commandError !== undefined && <section role="alert" className="note error-panel">
      <div><h2 className="h3">Could not complete command</h2><p>{describeError(commandError).message}</p></div>
      {onRetryCommand && <button type="button" className="btn sm" disabled={busy || !connected} onClick={onRetryCommand}>Retry command</button>}
    </section>}
    {browser && <section className="library-browser-results" aria-label={`${title} library`} aria-busy={busy}>
      <p className="loaded-state screen-reader-only" role="status">{title} loaded</p>
      {browser.items.length === 0 && <div className="empty library-browser-empty" role="status">
        <h2 className="h3">{browser.totalCount === 0 ? `No ${plural} yet` : `No ${plural} match`}</h2>
        <p className="help">{browser.totalCount === 0 ? `Start with New ${singular}, or ask the assistant to set them up.` : "Try another search or choose a different filter."}</p>
      </div>}
      {browser.items.length > 0 && <ol className="rows library-card-grid">{browser.items.map(card => <LibraryResourceCard key={card.id} card={card} kind={singular} onOpen={() => onOpen(card.id)} />)}</ol>}
      <p className="help library-browser-count">{browser.matchCount} shown · {browser.totalCount} in the library</p>
    </section>}
  </main>;
}

/**
 * A visual card; it uses the daemon's labels and stage strip without deriving resource state. `source`, on the Library
 * page, names the repository the card comes from at its foot.
 */
export function LibraryResourceCard({ card, kind, onOpen, source }: { card: LibraryCard; kind: "belt" | "agent"; onOpen: () => void; source?: string }) {
  return <li className={`row br library-resource-card library-resource-card--${kind}`}>
    <button type="button" className="row-hit" aria-label={`Open ${card.name}`} onClick={onOpen} />
    <div><div className="t library-card-title">{card.name}</div>
      <div className="m"><span className="library-card-version">v{card.version}</span>{source && <span className="library-card-source">{source}</span>}
        {card.shadow && <span className="flag f library-card-shadow">{card.shadow}</span>}</div></div>
    <span className="c library-card-detail" style={{ whiteSpace: "normal" }}>{source === card.detail ? "" : card.detail}</span>
    {card.stages.length > 0 ? <span className="seq library-stage-strip" aria-label="Stage preview">{card.stages.map((stage, index) => <span key={index}><StageIconTile icon={stage.icon} tint={stage.tint} size={20} /></span>)}</span>
      : <span className="seq">{card.icon && <StageIconTile icon={card.icon} tint={card.tint ?? "grey"} size={20} />}</span>}
    <span className="c r"><span className={`flag ${card.statusTone === "good" ? "f" : card.statusTone === "problem" ? "o" : "f"}`}>{card.status}</span></span>
  </li>;
}
