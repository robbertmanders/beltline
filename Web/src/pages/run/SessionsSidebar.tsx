import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { apiRequest } from "../../api/client";
import { describeError } from "../../api/errors";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { continueSessionRequest, desktopRequest } from "../../api/runCommands";
import type { ContinueSessionResponse, SessionsView, SessionSidebarProjectionRow } from "../../api/wire.gen";
import { type TerminalTarget } from "../../api/terminalSocket";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { LazyTerminal } from "../../components/LazyTerminal";

// The Mac app's marks: a half moon while it runs, a cross for a problem, a tick once it finished.
const statusGlyph: Record<SessionSidebarProjectionRow["status"], string> = { live: "◐", problem: "✕", done: "✓" };
const statusLabel: Record<SessionSidebarProjectionRow["status"], string> = { live: "Live", problem: "Problem", done: "Done" };

export interface SessionsSidebarProps {
  runId: string;
  /** The Run page's selected stage and attempt, which the daemon resolves and may correct. */
  selectedStage: string | null;
  selectedAttempt: string | null;
  /** The workspace width the sidebar shares with the main column. */
  available: number;
  /** The width the user chose, or null for half of the workspace. */
  stored: number | null;
  onWidthChange: (width: number | null) => void;
  onSelect: (stageId: string, attemptId: string) => void;
  onHide: () => void;
  /** How many sessions the run has, for the heading. */
  count?: number;
}

/**
 * The Run page's Sessions sidebar: the run's session-bearing attempts, the selected one's terminal or saved
 * log, and the offers the daemon permits for it. Every fact comes from the daemon's Sessions read. The
 * terminal attaches only to the target the read names, or to the one a Continue returns, and takes over
 * only after an enabled offer is activated.
 */
export function SessionsSidebar({ runId, selectedStage, selectedAttempt, onSelect, count }: SessionsSidebarProps) {
  const runtime = useRuntime();
  const command = useCommand();
  const path = `/api/v1/views/run/${encodeURIComponent(runId)}/sessions`;
  const search = new URLSearchParams();
  if (selectedStage) search.set("stage", selectedStage);
  if (selectedAttempt) search.set("attempt", selectedAttempt);
  const url = search.toString() ? `${path}?${search.toString()}` : path;
  const query = useQuery({
    queryKey: apiQueryKey(path, { stage: selectedStage, attempt: selectedAttempt }),
    queryFn: ({ signal }) => apiRequest<SessionsView>(url, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && runId.length > 0,
    retry: false,
    // A new selection keeps the rows on screen until its read arrives, so they do not blink out and back.
    placeholderData: keepPreviousData,
    meta: { resourceDeps: resourcesForPath(path) }
  });
  // Without authorization the last read is not shown, so its terminal detaches and no input can reach it.
  const view = runtime.canRead ? query.data : undefined;
  const selectedId = view?.selection.attemptId ?? null;
  const selectedRow = view?.rows.find(row => row.selected) ?? null;
  const daemonOffline = runtime.connection === "offline";

  const [takingOver, setTakingOver] = useState(false);
  const [continued, setContinued] = useState<{ attemptId: string; target: string } | null>(null);
  // Take over and a Continue target belong to one selected attempt: choosing another one clears both.
  useEffect(() => {
    setTakingOver(false);
    setContinued(null);
  }, [runId, selectedId]);

  const takeOverOffer = view?.takeOver ?? null;
  const terminal = view?.terminal ?? null;
  const takeOverEnabled = takeOverOffer?.enabled === true && !daemonOffline;
  const continuedHere = continued && continued.attemptId === selectedId ? continued.target : null;
  const target: TerminalTarget | null = continuedHere
    ? { session: continuedHere, takeOver: false }
    : terminal ? { session: terminal.target, takeOver: takingOver && takeOverEnabled && !terminal.continued } : null;
  const ghosttyEnabled = view?.ghostty.enabled === true && !daemonOffline;
  const continueOffer = view?.continueSession ?? null;
  const continueEnabled = continueOffer?.enabled === true && !daemonOffline && !command.pending;

  const onContinue = (attemptId: string) => {
    const request = continueSessionRequest(runId, attemptId);
    command.submit({
      ...request,
      publish: (queryClient, result: ContinueSessionResponse) => {
        request.publish(queryClient, result);
        setContinued({ attemptId, target: result.target });
      }
    });
  };

  const onGhostty = (attemptId: string) => {
    command.submit({ ...desktopRequest("ghostty", runId, { attemptId }), publish: () => undefined });
  };

  return <aside className="sess sessions-panel" data-column="sessions" data-ready={view ? "true" : undefined} aria-label="Sessions">
    <div className="sess-h"><h2 className="h2">Sessions</h2><span className="soft" style={{ fontSize: 13 }}>{count ?? view?.rows.length ?? 0} · {takingOver ? "you have the keyboard" : "read only"}</span></div>
    {!view && !query.error && <p className="help" role="status">Loading sessions…</p>}
    {!view && query.error && <section className="note error-panel" role="alert"><div><h3 className="h3">Unable to load sessions</h3><p>{describeError(query.error).message}</p></div></section>}
    {view && view.rows.length === 0 && <p className="help sessions-empty" role="status">{view.emptyMessage}</p>}
    {view && view.rows.length > 0 && <ol className="atts session-list" aria-label="Session list">
      {view.rows.map(row => <li key={row.attemptId}>
        <button type="button" className="att session-entry" aria-pressed={row.selected} aria-current={row.selected} title={row.session} onClick={() => onSelect(row.stageId, row.attemptId)}>
          <span><span className="session-status" data-status={row.status} aria-label={statusLabel[row.status]}>{statusGlyph[row.status]}</span> <b className="session-title">{row.title}</b></span>
          <span className="session-caption">{row.caption}</span>
        </button>
      </li>)}
    </ol>}
    {(view?.usage?.tokens || view?.usage?.estimate) && <div className="sess-f session-usage-block">
      {view.usage.tokens && <span className="session-usage">{view.usage.tokens}</span>}
      {view.usage.estimate && <span className="session-usage-note">{view.usage.estimate}</span>}
    </div>}
    {view && selectedId && view.selection.fallback && <p className="help session-fallback" role="status">{view.selection.fallback}</p>}
    {view && selectedId && <section className="session-detail" aria-label="Selected session">
      <div className="sess-f session-bar">
        <span className="mono session-bar-name">tmux · {selectedRow?.session ?? ""}</span>
        <span className="session-actions">
          {takeOverOffer && terminal && !terminal.continued && (takingOver
            ? <button type="button" className="link terminal-action" onClick={() => setTakingOver(false)}>Watch only</button>
            : takeOverEnabled
              ? <button type="button" className="link terminal-action" onClick={() => setTakingOver(true)}>Take over</button>
              : takeOverOffer.reason && <span className="session-reason">{takeOverOffer.reason}</span>)}
          {ghosttyEnabled && <button type="button" className="link terminal-action" onClick={() => onGhostty(selectedId)}>Attach in Ghostty</button>}
          {continueOffer && (continueOffer.enabled
            ? <button type="button" className="link terminal-action" disabled={!continueEnabled} title="Start the agent again in this session, interactively" onClick={() => onContinue(selectedId)}>Continue</button>
            : continueOffer.reason && <span className="session-reason">{continueOffer.reason}</span>)}
        </span>
      </div>
      {command.error !== null && command.error !== undefined && <p className="why session-error" role="alert">{describeError(command.error).message}</p>}
      <div className="term session-screen">
        {target
          ? <LazyTerminal key={`${target.session}|${target.takeOver ? "take-over" : "watch"}`} target={target} offline={daemonOffline} label={`Terminal for ${selectedRow?.title ?? "session"}`} />
          : view.log !== null && view.log !== undefined && <pre className="session-log" aria-label="Saved log">{view.log}</pre>}
      </div>
    </section>}
  </aside>;
}
