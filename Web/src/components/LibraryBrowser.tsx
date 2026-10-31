import { Fragment, useLayoutEffect, useRef, type ReactNode } from "react";
import type { LibraryBrowser as Browser, LibraryCard } from "../api/wire.gen";
import { describeError } from "../api/errors";
import { useRuntime } from "../app/runtime";
import { StageIconTile } from "../theme/stages/StageBox";
import { Segmented, Tag } from "./Paper";
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

  return <main className="library-browser" ref={element} data-column={`${singular}s-browser`}
    onScroll={event => scrollPositions.set(scrollID, event.currentTarget.scrollTop)}>
    <header className="library-browser-header">
      <div className="library-browser-heading"><Tag>Workspace</Tag><h1>{title}</h1><p>{lead}</p></div>
      <div className="library-browser-actions">
        {assistantButton}
        <button type="button" className="action-button slate" disabled={newReason !== undefined || busy} title={newReason} onClick={onNew}>{newAction?.label ?? `New ${singular}`}</button>
      </div>
    </header>
    <div className="library-browser-toolbar">
      <label className="library-browser-search">
        <input className="paper-field" aria-label={`Search ${title.toLowerCase()}`} placeholder={`Search ${title.toLowerCase()}`} value={search}
          onChange={event => onSearch(event.currentTarget.value)} />
        {search && <button type="button" className="toolbar-search-clear" aria-label={`Clear ${title} search`} onClick={() => onSearch("")}>×</button>}
      </label>
      {browser && <Segmented label={`${title} filter`} value={filter} onChange={onFilter}
        options={browser.filters.map(option => ({ value: option.id, label: `${option.label} ${option.count}` }))} />}
    </div>
    {newReason && browser && <p className="library-browser-muted">{newReason}</p>}
    {drafts.length > 0 && <section className="library-browser-drafts" aria-label="Unsent drafts">
      <span className="paper-caps">Unsent drafts</span>
      {drafts.map(draft => <button type="button" className="library-draft" key={draft.id} onClick={draft.onOpen}>{draft.name || `Unnamed ${singular}`} <span className="paper-chip paper-chip--need">Draft</span></button>)}
    </section>}
    {loading && !browser && <p role="status" className="loading-state">Loading {title.toLowerCase()}…</p>}
    {hasError && <section role="alert" className="error-panel">
      <h2>{browser ? "Refresh failed" : `Unable to load ${title.toLowerCase()}`}</h2><p>{describeError(error).message}</p>
      <button type="button" className="action-button small" disabled={busy || !runtime.canRead} onClick={onRetry}>Retry</button>
    </section>}
    {hasError && browser && <p role="status" className="stale-error">Showing the last loaded library; refresh failed.</p>}
    {commandError !== null && commandError !== undefined && <section role="alert" className="error-panel">
      <h2>Could not complete command</h2><p>{describeError(commandError).message}</p>
      {onRetryCommand && <button type="button" className="action-button small" disabled={busy || !connected} onClick={onRetryCommand}>Retry command</button>}
    </section>}
    {browser && <section className="library-browser-results" aria-label={`${title} library`} aria-busy={busy}>
      <p className="loaded-state screen-reader-only" role="status">{title} loaded</p>
      {browser.items.length === 0 && <div className="library-browser-empty" role="status">
        <h2>{browser.totalCount === 0 ? `No ${title.toLowerCase()} yet` : `No ${title.toLowerCase()} match`}</h2>
        <p>{browser.totalCount === 0 ? `Start with New ${singular}, or ask the agent to set them up.` : "Try another search or choose a different filter."}</p>
      </div>}
      <div className={`library-card-grid library-card-grid--${singular}`}>{browser.items.map(card => <LibraryResourceCard key={card.id} card={card} kind={singular} onOpen={() => onOpen(card.id)} />)}</div>
      <p className="library-browser-count">{browser.matchCount} shown · {browser.totalCount} in the library</p>
    </section>}
  </main>;
}

/**
 * A visual card; it uses the daemon's labels and stage strip without deriving resource state. `source`, on the Library
 * page, names the repository the card comes from at its foot.
 */
export function LibraryResourceCard({ card, kind, onOpen, source }: { card: LibraryCard; kind: "belt" | "agent"; onOpen: () => void; source?: string }) {
  return <button type="button" className={`library-resource-card library-resource-card--${kind}`} aria-label={`Open ${card.name}`} onClick={onOpen}>
    <span className="library-card-top"><span className={`paper-chip${kind === "agent" ? " paper-chip--grey" : ""}`}>{kind === "belt" ? "Belt" : "Agent"}</span>{card.shadow && <span className="paper-chip paper-chip--shadow library-card-shadow">{card.shadow}</span>}<span className="library-card-version">v{card.version}</span></span>
    <span className="library-card-title">{card.icon && <StageIconTile icon={card.icon} tint={card.tint ?? "grey"} size={24} />}<span>{card.name}</span></span>
    {card.stages.length > 0 && <span className="library-stage-strip" aria-label="Stage preview">{card.stages.map((stage, index) => <Fragment key={index}>
      {index > 0 && <i aria-hidden="true" />}<StageIconTile icon={stage.icon} tint={stage.tint} size={22} />
    </Fragment>)}</span>}
    <span className={`library-card-foot${source ? " library-card-foot--source" : ""}`}>{source && <span className="library-card-source">{source}</span>}<span className={`paper-chip paper-chip--${card.statusTone}`}>{card.status}</span></span>
  </button>;
}

export function LibraryDetailHeader({ backLabel, onBack, children }: { backLabel: string; onBack: () => void; children?: ReactNode }) {
  return <div className="library-detail-navigation"><button type="button" className="library-back" aria-label={backLabel} onClick={onBack}>← {backLabel}</button>{children}</div>;
}
