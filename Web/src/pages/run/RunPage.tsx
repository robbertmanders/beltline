import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
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
import { Caps, Chip, Tag } from "../../components/Paper";
import { RunLinkRows } from "./RunLinks";
import { RunMenu } from "./RunMenu";
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
  const linkRows = view ? <RunLinkRows runId={runId} links={view.links} worktrees={view.worktrees} worktreeAction={view.worktreeAction} worktreeUnavailable={view.worktreeUnavailable}
    command={command} canRead={runtime.canRead} connection={runtime.connection} onDone={() => setMenuOpen(false)} /> : null;
  const notFound = viewQuery.error && typeof viewQuery.error === "object" && "status" in viewQuery.error && viewQuery.error.status === 404;
  // One props object for the stage panel's inline forms and the Run controls menu, so both read the same
  // selection, draft and command controller.
  const controlsProps: Omit<RunControlsProps, "open" | "onOpenChange"> | null = view && panel && stage ? {
    adjustOpen, onAdjustOpenChange: setAdjustOpen,
    runId, run: view.run, view, stage, panel, selectedAttemptId, draft,
    onAnswerChange: value => setDraft("answer", value),
    onDecisionNoteChange: (outcomeId, value) => { if (selectedStageId) runDraftStore.setNote(runId, selectedStageId, outcomeId, value); },
    onPendingOutcomeChange: value => setDraft("pendingOutcome", value),
    onIncludeSubjectChange: value => { if (selectedStageId) runDraftStore.setIncludeSubject(runId, selectedStageId, value); },
    onSendMessageChange: value => setDraft("sendMessage", value),
    answerVersion: () => selectedStageId ? runDraftStore.version(runId, selectedStageId, "answer") : 0,
    command, submit, canRead: runtime.canRead, connection: runtime.connection, onErased: () => undefined
  } : null;

  return <div className="run-workspace" ref={workspaceRef}>
    <article className="page-content run-page">
      {menuOpen && <div className="run-menu-scrim" onClick={() => setMenuOpen(false)} />}
      {runtime.canRead && viewQuery.isPending && <p className="loading-state" role="status">Loading run…</p>}
      {runtime.canRead && notFound && <section className="empty-panel" role="status">
        <h2>This run is gone</h2>
        <p>{describeError(viewQuery.error).message}</p>
        <button type="button" className="quiet-button" onClick={() => navigate("/work")}>← Work</button>
      </section>}
      {runtime.canRead && viewQuery.error && !notFound && (!view || view === undefined) && <section className="error-panel" role="alert">
        <h2>Unable to load this run</h2>
        <p>{describeError(viewQuery.error).message}</p>
        <ProblemItems problems={describeError(viewQuery.error).problems} />
      </section>}
      {view && <div className="run-topbar">
        <button type="button" className="run-back" onClick={() => { if (window.history.length > 1) navigate(-1); else navigate("/work"); }}>← Work</button>
        <div className="run-actions">
          {/* One menu holds the run's controls, then its open links; without a stage there are no controls, only the links. */}
          {controlsProps
            ? <RunControls {...controlsProps} open={menuOpen} onOpenChange={setMenuOpen} links={linkRows} />
            : <RunMenu label="Run actions" open={menuOpen} onOpenChange={setMenuOpen}>{linkRows}</RunMenu>}
          {view.active && <RunAfkButton view={view} submit={submit} online={online} pending={command.pending} />}
          {view.quickActions.includes("start-now") && <QuickAction view={view} id="start-now" label="Start now" submit={submit} online={online} pending={command.pending} />}
          {view.quickActions.includes("retry-continuation") && <QuickAction view={view} id="retry-continuation" label="Retry" submit={submit} online={online} pending={command.pending} />}
          {!preferences.sessionsOpen && <button type="button" className="action-button small" title="Show the sessions" aria-label="Sessions" aria-expanded={false}
            onClick={() => update({ sessionsOpen: true })}>Sessions{sessionCount > 0 ? ` · ${sessionCount}` : ""}</button>}
        </div>
      </div>}
      {view && <header className="page-heading run-heading">
        <Tag>{view.heading}</Tag>
        <h1 className="run-title"><span className="run-title-ref">{view.reference} </span>{view.run.issue.title}</h1>
        {view.cost.line && <Caps className="run-cost" >COST {view.cost.line}</Caps>}
        {view.pauseCopy && <p className="run-pause" role="status">{view.pauseCopy}</p>}
        {(view.run.issue.labels.length > 0 || view.firstVerdict || view.shadow) && <ul className="run-label-list" aria-label="Labels">
          {view.shadow && <li data-shadow><Chip tone="shadow" title="This run started on the shadow version and keeps it to the end">{view.shadow}</Chip></li>}
          {view.run.issue.labels.map(label => <li key={label}><Chip tone="grey">{label}</Chip></li>)}
          {view.firstVerdict && <li data-first-verdict={view.firstVerdict}><Chip>{view.firstVerdict}</Chip></li>}
        </ul>}
      </header>}
      {view && view.legs.length > 0 && <section className="run-legs paper-card" aria-label="Split routes">
        <ul className="route-row-list">
          {view.legs.map(leg => <li key={leg.id} className={`route-row route-${leg.look}`}>
            <Tag>{leg.route ?? "route"}</Tag>
            <span className="route-stage">{leg.stage}</span>
            <Caps className={`route-state route-state-${leg.look}`}>{leg.state}</Caps>
          </li>)}
        </ul>
        <p className="route-note">Run controls apply to the whole run.</p>
      </section>}
      {view && <RunBoxes boxes={view.boxes} metadata={view.metadata} selectedStageId={selectedStageId} onSelectStage={selectStage} opening={opening} />}
      {view && controlsProps && <RunStagePanel runId={runId} run={view.run} panel={controlsProps.panel} stage={controlsProps.stage}
        selectedAttempt={selectedAttemptId} onSelectAttempt={selectAttempt} watchNotes={view.watchNotes}
        command={command} canRead={runtime.canRead} connection={runtime.connection}
        sessionsOpen={preferences.sessionsOpen} onWatchSession={() => update({ sessionsOpen: true })}
        onAdjust={() => setAdjustOpen(true)}
        answer={<RunInlineControls {...controlsProps} part="answer" />} decision={<RunInlineControls {...controlsProps} part="decision" />} />}
      {view && viewQuery.isError && <p className="stale-error" role="status">Showing the last loaded run; refresh failed.</p>}
    </article>
    {preferences.sessionsOpen && <SessionsSidebar key={runId} runId={runId} selectedStage={selectedStageId} selectedAttempt={selectedAttemptId}
      available={workspaceWidth} stored={preferences.sessionsWidth} onWidthChange={sessionsWidth => update({ sessionsWidth })}
      onSelect={selectSession} onHide={() => update({ sessionsOpen: false })} />}
  </div>;
}
