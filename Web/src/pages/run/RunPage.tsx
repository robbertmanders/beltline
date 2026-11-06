import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useState, useSyncExternalStore, type CSSProperties } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { apiRequest } from "../../api/client";
import { describeError } from "../../api/errors";
import { continueSessionRequest, eraseRequest, runCommandRequest } from "../../api/runCommands";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import type { ContinueSessionResponse, RunResponse, RunStage, RunViewResponse } from "../../api/wire.gen";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { ProblemItems } from "../../components/ProblemList";
import { useElementWidth } from "../../components/useElementWidth";
import { usePreferences } from "../../state/usePreferences";
import { emptyStageDraft, runDraftStore, type StageDraft } from "../../state/runDrafts";
import { QuickAction, RunAfkButton, RunControls, type RunCommandSubmit, type RunControlsProps } from "./RunControls";
import { RunInlineControls } from "./RunInlineControls";
import { Icon } from "../../components/Icon";
import { rememberClick } from "../../components/reveal";
import { RunLinkRows } from "./RunLinks";
import { RunBoxes } from "./RunBoxes";
import { RunStagePanel } from "./RunStagePanel";
import { SessionsSidebar } from "./SessionsSidebar";

const widthFallback = 1080;

function useDraftState() {
  return useSyncExternalStore(runDraftStore.subscribe, runDraftStore.getSnapshot, runDraftStore.getSnapshot);
}

export function RunPage() {
  const params = useParams();
  const navigate = useNavigate();
  const runtime = useRuntime();
  const { preferences, update } = usePreferences();
  const command = useCommand();
  const [searchParams, setSearchParams] = useSearchParams();
  const [menuOpen, setMenuOpen] = useState(false);
  const [adjustOpen, setAdjustOpen] = useState(false);
  const drafts = useDraftState();
  const runId = params.id ?? "";
  const [workspaceRef, measuredWorkspaceWidth] = useElementWidth<HTMLDivElement>();
  const workspaceWidth = measuredWorkspaceWidth > 0 ? measuredWorkspaceWidth : widthFallback;
  const viewPath = `/api/v1/views/run/${encodeURIComponent(runId)}`;
  const viewEnabled = runtime.canRead && runId.length > 0;

  // The page asks for the stage it shows, so the daemon sends that stage's Adjust help alone. A stage
  // the URL does not name is the run's current stage, which the daemon picks as the page does.
  const stageParam = searchParams.get("stage");
  const viewQuery = useQuery({
    queryKey: apiQueryKey(viewPath, stageParam ?? ""),
    queryFn: ({ signal }) => apiRequest<RunViewResponse>(`${viewPath}${stageParam === null ? "" : `?stage=${encodeURIComponent(stageParam)}`}`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: viewEnabled,
    // Keep the view on screen while another stage loads, but never another run.
    placeholderData: (previousData, previousQuery) => previousQuery?.queryKey[1] === viewPath ? previousData : undefined,
    retry: false,
    meta: { resourceDeps: resourcesForPath(viewPath) }
  });
  const view = viewQuery.data;

  useEffect(() => { document.title = "Beltline — Run"; }, []);
  useEffect(() => { setMenuOpen(false); }, [runId]);

  const stages = view?.stages ?? [];
  const attemptParam = searchParams.get("attempt");
  const fallbackStageId = stages.find(stage => stage.id === view?.run.stageId)?.id ?? stages[0]?.id ?? null;
  const selectedStageId = stageParam !== null && stages.some(stage => stage.id === stageParam) ? stageParam : fallbackStageId;
  const panel: RunStage | undefined = stages.find(stage => stage.id === selectedStageId);
  const attemptValid = attemptParam !== null && panel?.attempts.some(attempt => attempt.id === attemptParam) === true;
  const selectedAttemptId = attemptValid ? attemptParam : panel?.defaultAttempt ?? null;

  // Invalid or foreign selections fall back to the current stage and its default attempt, and the bad
  // query values are removed so a reload and Back/Forward land on the same valid selection.
  useEffect(() => {
    if (!view) return;
    const badStage = stageParam !== null && !stages.some(stage => stage.id === stageParam);
    const badAttempt = attemptParam !== null && panel?.attempts.some(attempt => attempt.id === attemptParam) !== true;
    if (!badStage && !badAttempt) return;
    setSearchParams(previous => {
      const next = new URLSearchParams(previous);
      if (badStage) next.delete("stage");
      if (badAttempt) next.delete("attempt");
      return next;
    }, { replace: true });
  }, [view, stageParam, attemptParam, stages, panel, setSearchParams]);

  const selectStage = useCallback((stageId: string) => {
    const target = stages.find(stage => stage.id === stageId);
    setSearchParams(previous => {
      const next = new URLSearchParams(previous);
      next.set("stage", stageId);
      if (target?.defaultAttempt) next.set("attempt", target.defaultAttempt);
      else next.delete("attempt");
      return next;
    });
  }, [stages, setSearchParams]);

  const selectAttempt = useCallback((attemptId: string) => {
    setSearchParams(previous => {
      const next = new URLSearchParams(previous);
      if (selectedStageId) next.set("stage", selectedStageId);
      next.set("attempt", attemptId);
      return next;
    });
  }, [selectedStageId, setSearchParams]);

  // A Sessions row selects its stage and attempt in the URL, so the Run page and the sidebar follow one selection.
  const selectSession = useCallback((stageId: string, attemptId: string) => {
    setSearchParams(previous => {
      const next = new URLSearchParams(previous);
      next.set("stage", stageId);
      next.set("attempt", attemptId);
      return next;
    });
  }, [setSearchParams]);

  const submit: RunCommandSubmit = useCallback((id, operands, onSuccess) => {
    if (id === "erase") {
      command.submit({ ...eraseRequest(runId), publish: () => { runDraftStore.clearRun(runId); navigate("/work"); onSuccess?.(); } });
      return;
    }
    if (id === "continue-session") {
      const request = continueSessionRequest(runId, operands.attempt ?? "");
      command.submit({ ...request, publish: (queryClient, result: ContinueSessionResponse) => { request.publish(queryClient, result); onSuccess?.(); } });
      return;
    }
    let clear: (() => void) | undefined;
    if (id === "answer" && selectedStageId) {
      const version = runDraftStore.version(runId, selectedStageId, "answer");
      clear = () => runDraftStore.clearAnswer(runId, selectedStageId, version);
    } else if (id === "decide" && selectedStageId) {
      clear = () => runDraftStore.clearDecide(runId, selectedStageId);
    } else if (id === "resume" || id === "drop") {
      clear = () => runDraftStore.clearRun(runId);
    }
    const request = runCommandRequest(runId, id, operands);
    command.submit({ ...request, publish: (queryClient, result: RunResponse) => { request.publish(queryClient, result); clear?.(); onSuccess?.(); } });
  }, [command, runId, selectedStageId, navigate]);

  const stage = view?.run.stages.find(item => item.id === selectedStageId);
  const draft: StageDraft = { ...emptyStageDraft, ...drafts[runId]?.[selectedStageId ?? ""] };
  const setDraft = (field: keyof StageDraft, value: StageDraft[keyof StageDraft]) => {
    if (!selectedStageId) return;
    if (field === "answer") runDraftStore.setAnswer(runId, selectedStageId, value as string);
    else if (field === "pendingOutcome") runDraftStore.setPendingOutcome(runId, selectedStageId, value as string | null);
    else if (field === "sendMessage") runDraftStore.setSendMessage(runId, selectedStageId, value as string);
  };

  const online = runtime.canRead && runtime.connection === "connected";
  const sessionCount = view?.sessionCount ?? 0;
  // The stage line opens with the run's current stage in its middle; the URL's selection never moves it.
  const opening = view ? { run: runId, stage: view.run.stageId } : undefined;
  const notFound = viewQuery.error && typeof viewQuery.error === "object" && "status" in viewQuery.error && viewQuery.error.status === 404;
  // One props object for the stage panel's inline forms and the Run controls menu, so both read the same
  // selection, draft and command controller.
  const controlsProps: Omit<RunControlsProps, "open" | "onOpenChange"> | null = view && panel && stage ? {
    adjustOpen, onAdjustOpenChange: setAdjustOpen,
    runId, run: view.run, view, stage, panel, selectedAttemptId, draft,
    onAnswerChange: value => setDraft("answer", value),
    onDecisionNoteChange: (outcomeId, value) => { if (selectedStageId) runDraftStore.setNote(runId, selectedStageId, outcomeId, value); },
    onPendingOutcomeChange: value => setDraft("pendingOutcome", value),
    onPromptChange: value => { if (selectedStageId) runDraftStore.setPrompt(runId, selectedStageId, value); },
    onSourceChange: value => { if (selectedStageId) runDraftStore.setSource(runId, selectedStageId, value); },
    onSendMessageChange: value => setDraft("sendMessage", value),
    answerVersion: () => selectedStageId ? runDraftStore.version(runId, selectedStageId, "answer") : 0,
    command, submit, canRead: runtime.canRead, connection: runtime.connection, onErased: () => undefined
  } : null;

  const close = useCallback(() => { if (window.history.length > 1) navigate(-1); else navigate("/work"); }, [navigate]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || document.querySelector("[data-sheet-layer]")) return;
      const target = event.target as HTMLElement | null;
      if (target && (target.closest(".xterm") || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))) return;
      close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [close]);
  const [shown, setShown] = useState(false);
  const [origin] = useState(() => rememberClick.last());
  useEffect(() => { const frame = requestAnimationFrame(() => setShown(true)); return () => cancelAnimationFrame(frame); }, []);
  const current = view?.stages.find(item => item.id === view.run.stageId);
  const selected = view?.stages.find(item => item.id === selectedStageId);

  return <div className={`runv inv ${shown ? "open" : ""}`} ref={workspaceRef} role="region" aria-label="Run"
    style={{ "--cx": `${origin.x}px`, "--cy": `${origin.y}px` } as CSSProperties}>
    <div className="run-scroll scroll" data-column="main">
    <header className="rtop">
      <nav className="crumb" style={{ margin: 0 }} aria-label="Where you are">
        <button type="button" className="run-back" onClick={close}><Icon name="back" /><span>Work</span></button>
        {view && <><span>/</span><span>{view.heading}</span><span>/</span><span>{view.reference}</span></>}
      </nav>
      <span className="run-top-actions">
        {!preferences.sessionsOpen && <button type="button" className="btn sm ghost" title="Show the sessions" aria-label="Sessions" aria-expanded={false}
          onClick={() => update({ sessionsOpen: true })}><Icon name="term" />Sessions{sessionCount > 0 ? ` · ${sessionCount}` : ""}</button>}
        <button type="button" className="crumb" style={{ margin: 0 }} onClick={close}>Close <span className="kbd">Esc</span></button>
      </span>
    </header>
    <main className="rin run-page">
      {runtime.canRead && viewQuery.isPending && <p className="help" role="status">Loading run…</p>}
      {runtime.canRead && notFound && <section className="empty" role="status">
        <h2 className="h2">This run is gone</h2>
        <p className="help">{describeError(viewQuery.error).message}</p>
        <button type="button" className="btn" onClick={() => navigate("/work")}><Icon name="back" />Work</button>
      </section>}
      {runtime.canRead && viewQuery.error && !notFound && (!view || view === undefined) && <section className="note error-panel" role="alert"><div>
        <h2 className="h3">Unable to load this run</h2>
        <p>{describeError(viewQuery.error).message}</p>
        <ProblemItems problems={describeError(viewQuery.error).problems} />
      </div></section>}
      {view && <>
        <div className="rmeta">
          <span>{view.heading}</span>
          {view.cost.line && <span className="run-cost">Cost {view.cost.line}</span>}
          {view.shadow && <span className="flag f" data-shadow title="This run started on the shadow version and keeps it to the end">{view.shadow}</span>}
          {view.firstVerdict && <span className="flag o" data-first-verdict={view.firstVerdict}>{view.firstVerdict}</span>}
          {view.run.issue.labels.length > 0 && <ul className="run-label-list" aria-label="Labels">{view.run.issue.labels.map(label => <li key={label}><span className="flag f">{label}</span></li>)}</ul>}
        </div>
        <h1 className="rh run-title"><span className="run-title-ref soft">{view.reference} </span>{view.run.issue.title}</h1>
        {view.pauseCopy && <p className="lede run-pause" role="status">{view.pauseCopy}</p>}
        <div className="rlinks">
          <RunLinkRows runId={runId} links={view.links} worktrees={view.worktrees} worktreeAction={view.worktreeAction} worktreeUnavailable={view.worktreeUnavailable}
            command={command} canRead={runtime.canRead} connection={runtime.connection} onDone={() => setMenuOpen(false)} />
        </div>
        <div className="rctl">
          {current && <span className={`flag ${current.status.look === "waiting" ? "" : "o"}`}>{current.status.text}</span>}
          {current && <span className="soft" style={{ fontSize: 14 }}>{current.name}</span>}
          <span className="sp" />
          {view.active && <RunAfkButton view={view} submit={submit} online={online} pending={command.pending} />}
          {view.quickActions.includes("start-now") && <QuickAction view={view} id="start-now" label="Start now" submit={submit} online={online} pending={command.pending} />}
          {view.quickActions.includes("retry-continuation") && <QuickAction view={view} id="retry-continuation" label="Retry" submit={submit} online={online} pending={command.pending} />}
          {controlsProps && <RunControls {...controlsProps} open={menuOpen} onOpenChange={setMenuOpen} />}
        </div>
        {view.legs.length > 0 && <section className="run-legs" aria-label="Split routes">
          <ul className="side route-row-list">
            {view.legs.map(leg => <li key={leg.id} className={`route-row route-${leg.look}`}>
              <span>{leg.route ?? "route"}</span> <b>{leg.stage}</b> <span className={`route-state route-state-${leg.look}`}>{leg.state}</span>
            </li>)}
          </ul>
          <p className="why route-note">Run controls apply to the whole run.</p>
        </section>}
        <section className="route"><RunBoxes boxes={view.boxes} metadata={view.metadata} selectedStageId={selectedStageId} onSelectStage={selectStage} opening={opening} /></section>
        {/* Only the selected stage: the line above is how you move between them. */}
        <div className="rgrid run-chapters">
          <div>
            {selected && controlsProps && <section className="here-c run-stage" data-stage-chapter={selected.id} aria-label={selected.name}>
              <div className="hd">
                <span className="cn disp soft">{String(selected.number).padStart(2, "0")}</span>
                <h2 className="ct">{selected.name}</h2>
                {/* The status is in the panel's own head, so the line under the name says only who works it and how often. */}
                {(selected.worker || selected.attempts.length > 1) && <span className="sub">
                  {[selected.worker, selected.attempts.length > 1 ? `${selected.attempts.length} tries` : null].filter(Boolean).join(" · ")}</span>}
              </div>
              <div className="bd">
                <RunStagePanel runId={runId} run={view.run} panel={controlsProps.panel} stage={controlsProps.stage}
                  selectedAttempt={selectedAttemptId} onSelectAttempt={selectAttempt} watchNotes={view.watchNotes}
                  command={command} canRead={runtime.canRead} connection={runtime.connection}
                  sessionsOpen={preferences.sessionsOpen} onWatchSession={() => update({ sessionsOpen: true })} onAdjust={() => setAdjustOpen(true)}
                  answer={<RunInlineControls {...controlsProps} part="answer" />} decision={<RunInlineControls {...controlsProps} part="decision" />} />
              </div>
            </section>}
            {viewQuery.isError && <p className="why" role="status">Showing the last loaded run; refresh failed.</p>}
          </div>
        </div>
      </>}
    </main>
    </div>
    {view && preferences.sessionsOpen && <SessionsSidebar key={runId} runId={runId} selectedStage={selectedStageId} selectedAttempt={selectedAttemptId}
      available={workspaceWidth} stored={preferences.sessionsWidth} onWidthChange={sessionsWidth => update({ sessionsWidth })}
      onSelect={selectSession} onHide={() => update({ sessionsOpen: false })} count={sessionCount} />}
  </div>;
}
