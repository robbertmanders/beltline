import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";
import { apiRequest } from "../api/client";
import { describeError } from "../api/errors";
import { assistantCommandRequest, assistantPath, assistantQueryKey } from "../api/assistantCommands";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import { backendValues } from "../api/wire.gen";
import type { AssistantConfigurationResponse, AssistantResponse, Backend, ModelRunnerResource, Thinking } from "../api/wire.gen";
import { useCommand } from "../api/useCommand";
import { useRuntime } from "../app/runtime";
import { Sheet } from "./Sheet";
import { LazyTerminal } from "./LazyTerminal";
import { clampSidebarWidth, drawnSidebarWidth, sidebarKeyboardStep } from "../state/preferences";
import { backendLabels } from "./backendLabels";

const modelsPath = (backend: Backend) => `/api/v1/models/${backend}`;

export interface AssistantSidebarProps {
  available: number;
  /** What the page beside the sidebar keeps: on Belts and Agents, the library column and its editor's minimum. */
  reserved?: number;
  stored: number | null;
  onWidthChange: (width: number | null) => void;
  onHide: () => void;
  request: string;
  onRequestChange: (request: string) => void;
  dirty: boolean;
}

/** The shared assistant resource, configuration controls, conversation and terminal attachment. */
export function AssistantSidebar({ available, reserved, stored, onWidthChange, onHide, request, onRequestChange, dirty }: AssistantSidebarProps) {
  const runtime = useRuntime();
  const command = useCommand();
  const query = useQuery({
    queryKey: assistantQueryKey,
    queryFn: ({ signal }) => apiRequest<AssistantResponse>(assistantPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(assistantPath) }
  });
  const resource = runtime.canRead ? query.data : undefined;
  const [selection, setSelection] = useState<AssistantConfigurationResponse | null>(null);
  const [draftWidth, setDraftWidth] = useState<number | null>(null);
  const [startWarning, setStartWarning] = useState(false);
  const dragStart = useRef<{ x: number; width: number } | null>(null);

  // A successful configuration command or event refresh replaces the confirmed selection. A refused
  // configuration command leaves the read untouched, so the person's local attempt remains visible.
  useEffect(() => {
    if (resource) setSelection(resource.configuration);
  }, [resource?.configuration]);
  const local = selection ?? resource?.configuration ?? null;
  const modelQuery = useQuery({
    queryKey: apiQueryKey(modelsPath(local?.backend ?? "pi")),
    queryFn: ({ signal }) => apiRequest<ModelRunnerResource>(modelsPath(local?.backend ?? "pi"), { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && local !== null,
    retry: false,
    meta: { resourceDeps: resourcesForPath(modelsPath(local?.backend ?? "pi")) }
  });

  const availableOffer = (id: "configuration" | "start" | "stop") => resource?.actions.find(action => action.id === id);
  const configurationOffer = availableOffer("configuration");
  const startOffer = availableOffer("start");
  const stopOffer = availableOffer("stop");
  const connectionReady = runtime.canRead && runtime.connection === "connected";
  const connectionReason = runtime.connection === "offline" ? "The daemon is offline. Commands and terminal input are unavailable."
    : runtime.connection === "incompatible" ? "This page does not support the daemon's API version."
    : runtime.connection === "connecting" ? "Waiting for the daemon connection."
    : null;
  const offerReason = (offer: typeof configurationOffer) => !connectionReady ? connectionReason
    : !offer ? "The daemon did not offer this command."
    : !offer.enabled ? offer.reason ?? "This command is not currently available." : null;
  const chosen = draftWidth ?? stored ?? available / 2;
  const drawn = drawnSidebarWidth(available, draftWidth ?? stored, reserved);

  const sendConfiguration = (next: AssistantConfigurationResponse) => {
    setSelection(next);
    if (!connectionReady || command.pending || !configurationOffer?.enabled) return;
    command.submit(assistantCommandRequest("configuration", next));
  };

  const submitStart = () => {
    if (!resource || !connectionReady || command.pending || !startOffer?.enabled || request.trim() === "") return;
    const write = assistantCommandRequest("start", { request });
    command.submit({ ...write, publish: (queryClient, response: AssistantResponse) => {
      write.publish(queryClient, response);
      onRequestChange("");
    } });
  };

  const onStart = () => {
    if (!resource || !connectionReady || command.pending || !startOffer?.enabled || request.trim() === "") return;
    if (dirty) setStartWarning(true);
    else submitStart();
  };

  const onStop = () => {
    if (!connectionReady || command.pending || !stopOffer?.enabled) return;
    command.submit(assistantCommandRequest("stop", {}));
  };

  const commitWidth = () => {
    if (draftWidth !== null) onWidthChange(draftWidth);
    dragStart.current = null;
    setDraftWidth(null);
  };
  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    dragStart.current = { x: event.clientX, width: chosen };
    event.currentTarget.setPointerCapture?.(event.pointerId);
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const start = dragStart.current;
    if (start) setDraftWidth(clampSidebarWidth(start.width + start.x - event.clientX));
  };
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    onWidthChange(clampSidebarWidth(chosen + (event.key === "ArrowLeft" ? sidebarKeyboardStep : -sidebarKeyboardStep)));
  };
  const currentError = command.error;
  const currentStatus = resource?.busy ? "Busy" : resource?.running ? "Running" : "Stopped";
  const savedSelection = resource?.configuration;
  const selectionChanged = local && savedSelection && (local.backend !== savedSelection.backend || local.model !== savedSelection.model || local.thinking !== savedSelection.thinking);
  const modelChoices = modelQuery.data?.choices ?? [];
  const currentModelOffered = local ? modelChoices.some(choice => choice.id === local.model) : false;
  const modelFailure = modelQuery.isError ? describeError(modelQuery.error).message : null;

  return <aside className="assistant-panel" data-column="assistant" aria-label="Assistant" style={{ width: `${drawn}px` }}>
    <div className="assistant-divider" role="separator" aria-orientation="vertical" aria-label="Resize Assistant" aria-valuemin={320} aria-valuemax={900} aria-valuenow={Math.round(chosen)} tabIndex={0}
      onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={commitWidth} onPointerCancel={commitWidth}
      onDoubleClick={() => { dragStart.current = null; setDraftWidth(null); onWidthChange(null); }} onKeyDown={onKeyDown} />
    <header className="assistant-header">
      <span className="paper-tag assistant-tag">Agent</span>
      <span className="assistant-status screen-reader-only" role="status">{resource ? currentStatus : "Loading"}</span>
      <button type="button" className="sessions-hide" onClick={onHide}>Hide ›</button>
    </header>
    {query.isPending && !resource && <p className="loading-state" role="status">Loading assistant…</p>}
    {query.isError && !resource && <section className="assistant-read-error" role="alert"><h3>Unable to load assistant</h3><p>{describeError(query.error).message}</p></section>}
    {query.isError && resource && <p className="assistant-stale" role="status">Showing stale assistant data; refresh failed.</p>}
    {resource && <>
      <section className="assistant-configuration" aria-label="Assistant configuration">
        <fieldset disabled={!connectionReady || command.pending || !configurationOffer?.enabled || resource.running || resource.busy}>
          <div className="assistant-inline">
          <label className="assistant-compact"><span aria-hidden="true">Harness</span>
            <select aria-label="Harness" value={local?.backend ?? resource.configuration.backend} onChange={event => {
              const backend = event.target.value as Backend;
              const previous = local ?? resource.configuration;
              sendConfiguration({ ...previous, backend });
            }}>
              {backendValues.map(backend => <option key={backend} value={backend}>{backendLabels[backend]}</option>)}
            </select>
          </label>
          <label className="assistant-compact"><span aria-hidden="true">Thinking</span>
            <select aria-label="Thinking" value={local?.thinking ?? ""} onChange={event => {
              const previous = local ?? resource.configuration;
              sendConfiguration({ ...previous, thinking: event.target.value === "" ? null : event.target.value as Thinking });
            }}>
              <option value="">Default</option>
              {(modelQuery.data?.thinking ?? []).map(choice => <option key={choice.id} value={choice.id}>{choice.label}</option>)}
            </select>
          </label>
          </div>
          {modelQuery.data?.thinkingNote && <p className="assistant-note">{modelQuery.data.thinkingNote}</p>}
          <label className="assistant-field"><span>Model</span>
            <select aria-label="Model" value={local?.model ?? ""} onChange={event => {
              const previous = local ?? resource.configuration;
              sendConfiguration({ ...previous, model: event.target.value });
            }}>
              {local && !currentModelOffered && <option value={local.model}>{local.model} · saved selection</option>}
              {modelChoices.map(choice => <option key={choice.id} value={choice.id}>{choice.name}</option>)}
            </select>
          </label>
        </fieldset>
        {selectionChanged && <p className="assistant-saved-selection">Saved configuration: {backendLabels[savedSelection!.backend]} · {savedSelection!.model} · {savedSelection!.thinking ?? "Default"}</p>}
        {modelQuery.isPending && <p className="assistant-note" role="status">Loading model choices…</p>}
        {modelFailure && <p className="assistant-error" role="alert">{modelFailure}</p>}
        {offerReason(configurationOffer) && <p className="assistant-action-reason">{offerReason(configurationOffer)}</p>}
      </section>
      {currentError !== null && currentError !== undefined && <p className="assistant-error" role="alert">{describeError(currentError).message}</p>}
      {resource.running
        ? <section className="assistant-conversation" aria-label="Assistant conversation">
            {resource.command
              ? <LazyTerminal key={`${resource.session}:${resource.command.join(" ")}`} target={{ session: resource.session, takeOver: false }} offline={!connectionReady} label="Assistant terminal" />
              : <p className="assistant-demo" role="status">Simulated conversation. No real agent is launched in demo mode.</p>}
            <div className="assistant-conversation-foot">
              <p className="assistant-terminal-note">Type in the terminal to continue.</p>
              <button type="button" className="action-button small" disabled={!connectionReady || command.pending || !stopOffer?.enabled} title={offerReason(stopOffer) ?? undefined} onClick={onStop}>Stop</button>
            </div>
            {offerReason(stopOffer) && <p className="assistant-action-reason">{offerReason(stopOffer)}</p>}
          </section>
        : <section className="assistant-ask" aria-label="Start an assistant conversation">
            <p>Ask for changes to your belts and agents. They take effect as the agent saves them: new runs use them, runs already going keep theirs.</p>
            <textarea className="assistant-request" aria-label="Request" value={request} onChange={event => onRequestChange(event.target.value)} />
            {dirty && <p className="assistant-dirty-note">You have unsaved changes on this page. Save them before you start.</p>}
            <button type="button" className="action-button small slate assistant-start" disabled={!connectionReady || command.pending || !startOffer?.enabled || request.trim() === ""} title={offerReason(startOffer) ?? undefined} onClick={onStart}>Start</button>
            {offerReason(startOffer) && <p className="assistant-action-reason">{offerReason(startOffer)}</p>}
          </section>}
      {resource.problems.length > 0 && <section className="assistant-problems" aria-label="Problems in what it changed">
        <h3>Problems in what it changed</h3>
        <ul>{resource.problems.map((problem, index) => <li key={`${index}:${problem}`}>{problem}</li>)}</ul>
        <p>Ask the agent to fix them. A belt or agent whose newest version can't be read is missing from the app until then.</p>
      </section>}
    </>}
    <Sheet title="Save your changes first?" open={startWarning} onClose={() => setStartWarning(false)} footer={<>
      <button type="button" className="paper-button" onClick={() => setStartWarning(false)}>Cancel</button>
      <button type="button" className="paper-button paper-button--primary" onClick={() => { setStartWarning(false); submitStart(); }}>Start anyway</button>
    </>}>
      <p>The agent changes your belts and agents directly. It can overwrite or remove what you are editing, and your unsaved changes are not kept anywhere. It is better to save first.</p>
    </Sheet>
  </aside>;
}

export const assistantRiskCopy = "The agent changes your belts and agents directly. It can overwrite or remove what you are editing, and your unsaved changes are not kept anywhere. It is better to save first.";

export interface AssistantOpenButtonProps {
  open: boolean;
  dirty: boolean;
  saveAvailable: boolean;
  onOpenChange: (open: boolean) => void;
  onSaveAndOpen: (afterSave: () => void) => void;
}

/** The page entry button and native-equivalent dirty-edit confirmation. */
export function AssistantOpenButton({ open, dirty, saveAvailable, onOpenChange, onSaveAndOpen }: AssistantOpenButtonProps) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const requestOpen = () => {
    if (open) onOpenChange(false);
    else if (dirty) setConfirmOpen(true);
    else onOpenChange(true);
  };
  return <>
    <button type="button" className={`action-button${open ? " slate" : ""}`} aria-expanded={open} title="Ask an agent to change your belts and agents" onClick={requestOpen}>Agent</button>
    <Sheet title="Save your changes first?" open={confirmOpen} onClose={() => setConfirmOpen(false)} footer={<>
      <button type="button" className="paper-button" onClick={() => setConfirmOpen(false)}>Cancel</button>
      <button type="button" className="paper-button" onClick={() => { setConfirmOpen(false); onOpenChange(true); }}>Open without saving</button>
      {saveAvailable && <button type="button" className="paper-button paper-button--primary" onClick={() => { setConfirmOpen(false); onSaveAndOpen(() => onOpenChange(true)); }}>Save and open</button>}
    </>}>
      <p>{assistantRiskCopy}</p>
    </Sheet>
  </>;
}
