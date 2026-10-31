import { useEffect, useState, type ReactNode } from "react";
import type { QueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { useCommand } from "../../api/useCommand";
import { apiRequest } from "../../api/client";
import { useRuntime } from "../../app/runtime";
import type { AfkMode, Belt, BeltAction, BeltDraftResponse, BeltResourceResponse, BeltStageTemplateResponse, ShadowSummary, Stage } from "../../api/wire.gen";
import {
  afkRequest,
  archiveRequest,
  copyRequest,
  draftRequest,
  pauseRequest,
  publishBeltResource,
  saveRequest,
  shadowSaveRequest,
  stageTemplateRequest,
  updateAllStagesRequest,
  updateStageRequest,
  type DraftOperands,
  type DraftOperation
} from "./beltCommands";
import { rebaseTyped, sameBelt, useBeltDrafts, type BeltDraft } from "./beltDrafts";
import { beltRecord, type BeltRecord } from "./beltRecords";
import { DraftEditorProvider, unchangedField, type AppliedForm, type DraftEditorValue, type StageRequest } from "./draftEditor";
import { locatedProblems, locationKey, ProblemSummary } from "./ProblemSummary";
import type { Gate } from "./BeltActionButton";
import { typedOperation } from "./beltEditorHelpers";
import { StageLine } from "./StageLine";
import { Tag } from "../../components/Paper";
import { StageForm } from "./StageForm";
import { TriggerForm } from "./TriggerForm";
import { TaskSheet } from "./TaskSheet";
import { SharedBeltCard } from "./SharedBeltCard";
import { BeltEditorActions } from "./BeltEditorActions";
import { AgentLeave, type AgentRef } from "./AgentLeave";
import { useBeltAgentChoices } from "./useBeltAgentChoices";
import { useBeltContext } from "./useBeltContext";
import { useBeltSaveQueue } from "./useBeltSaveQueue";
import { ShadowBar } from "./ShadowBar";
import { ShadowDecisionSheet, type ShadowDecision } from "./ShadowSheets";

export interface BeltEditorProps {
  /** The draft to show: the unsent draft if there is one, otherwise the saved belt. */
  base: BeltDraft;
  /** The library entry the draft is compared with; null for a new or copied belt. */
  snapshot: BeltRecord | null;
  /** Every belt id in the library, archived ones included, for Discard's fallback. */
  allIds: readonly string[];
  /** The live belt ids in list order, for Discard's fallback. */
  liveIds: readonly string[];
  onSelect: (key: string | null) => void;
  /** Moves the selection when a command's answer names a different key, but only if the user is still on the key the command came from. */
  followSelection: (from: string | null, to: string) => void;
  onShare: (id: string, version: number, name?: string) => void;
  onAssistantState?: (state: { dirty: boolean; saveAvailable: boolean; save: (then?: () => void) => void } | null) => void;
  /** The configuration assistant's button, at the end of the header's actions. */
  assistantButton?: ReactNode;
  /** True when the saved belt this draft came from is no longer in the library, so the draft cannot be saved. */
  missing?: boolean;
  /**
   * Set while the editor shows the belt's shadow: its running test, or null for a new shadow that waits for its
   * first save. The draft is then the shadow's, and Save saves a shadow version.
   */
  shadow?: { summary: ShadowSummary | null };
  /** Switches between the live version and the shadow. */
  onShadowMode?: (shadow: boolean) => void;
}

/**
 * The selected belt's editor. Typing changes the browser's draft only. Structural edits and the saved-belt
 * commands go to the daemon, and the page takes the answer as the new state. The daemon's context for the draft
 * is read after mount and after each committed change, never while typing; a late context answer that a newer
 * one has replaced is dropped. A command the daemon refuses leaves the draft and the last successful state as they were.
 */
export function BeltEditor({ base, snapshot, allIds, liveIds, onSelect, followSelection, onShare, onAssistantState, assistantButton, missing = false, shadow, onShadowMode }: BeltEditorProps) {
  const runtime = useRuntime();
  const drafts = useBeltDrafts();
  const command = useCommand();
  const navigate = useNavigate();
  const { key, belt, unsaved } = base;
  const shadowMode = shadow !== undefined;
  const shadowSummary = shadowMode ? shadow.summary : snapshot?.shadow ?? null;
  const beltId = snapshot?.belt.id ?? belt.id;
  // A shadow's draft is compared with its saved shadow version; a new shadow has none, so it is unsent.
  const saved = shadowMode ? shadowSummary?.draft ?? null : snapshot?.belt ?? null;
  const dirty = unsaved || (saved !== null && !sameBelt(belt, saved));
  const archived = snapshot?.archived ?? false;
  const paused = snapshot?.paused ?? false;
  // The warnings of the last answer for this belt, else those the library read carried for the saved version.
  const warnings = drafts.warnings[key] ?? (shadowMode ? shadowSummary?.warnings : snapshot?.warnings) ?? [];

  const [sheetOpen, setSheetOpen] = useState(false);
  const [decision, setDecision] = useState<ShadowDecision | null>(null);
  const [agentLeave, setAgentLeave] = useState<AgentRef | null>(null);
  const [templateFailure, setTemplateFailure] = useState<unknown>(null);
  // The open stage form and the typed form input live in the app-level store, so they survive navigation.
  const selectedStageId = drafts.selectedStages[key] ?? null;
  const setSelectedStageId = (stage: string | null) => drafts.selectStage(key, stage);
  const typedForms = drafts.inputs[key] ?? {};
  const hasTyped = Object.keys(typedForms).length > 0;

  const agentChoices = useBeltAgentChoices(archived);
  // The context is read again when the belt's resource changes, or the shadow's version, so the daemon's gates follow a settings change.
  const { context, refreshContext } = useBeltContext(key, belt, selectedStageId, `${snapshot?.revision ?? ""}|${shadowSummary?.version ?? ""}`, shadowMode);

  const connectionReason = runtime.connection === "connected" ? null
    : runtime.connection === "offline" ? "The daemon is offline, so this cannot be sent. It becomes available when the daemon reconnects."
    : runtime.connection === "incompatible" ? "This page does not support the daemon's API version."
    : "Waiting for the daemon connection.";
  const blockedReason = connectionReason ?? (command.pending ? "A belt command is already running." : null);
  const gate = (action: BeltAction | undefined): Gate => {
    const reason = blockedReason ?? (action && !action.enabled ? action.reason ?? "This is not available for this belt." : null);
    return { enabled: reason === null, reason };
  };
  // The daemon's context knows the draft's actions, so it is preferred; the library read is the fallback.
  const actionFor = (id: BeltAction["id"]): BeltAction | undefined =>
    (context?.actions ?? snapshot?.actions ?? []).find(action => action.id === id);
  const actionGate = (id?: BeltAction["id"]): Gate => gate(id ? actionFor(id) : undefined);
  // Save is shown only when it can be used: typed input is applied as part of the save, and a click while
  // another belt command runs waits for it. Without the daemon there is no Save; the offline banner says why.
  const saveGate: Gate = connectionReason ? { enabled: false, reason: connectionReason }
    : missing ? { enabled: false, reason: "This belt is no longer in the library, so it cannot be saved. Discard the changes to finish." }
    : dirty || hasTyped ? { enabled: true, reason: null } : { enabled: false, reason: "No unsaved changes to save." };
  // Structural edits change only the draft, so only the connection and a pending command gate them.
  const structuralGate = gate(undefined);
  // Starting a shadow test is the daemon's shadow-create command: a new shadow gets its gate, so a refused start shows its reason.
  const startGate = shadowMode && !shadowSummary ? actionGate("shadow-create") : undefined;

  const edit = (change: (current: Belt) => Belt) => drafts.update(key, current => {
    const from = current ?? base;
    // The first edit of a saved belt remembers the saved draft it started from; a draft that comes back to it is clean.
    const basis = from.unsaved ? undefined : current ? current.basis : base.belt;
    return { ...from, belt: change(from.belt), basis };
  });

  /** Takes a daemon answer for the draft that was submitted. Input typed while the request was pending is kept. */
  const settle = (submitted: Belt, response: BeltDraftResponse, applies?: AppliedForm) => {
    drafts.setWarnings(key, response.warnings);
    if (applies && applies.took && !applies.took(response.draft)) drafts.type(key, applies.form, unchangedField, true);
    else if (applies) drafts.settleTyped(key, applies.form, applies.applied);
    drafts.update(key, current => {
      const merged = current ? rebaseTyped(response.draft, current.belt, submitted) : response.draft;
      if (!unsaved && saved && sameBelt(merged, saved)) return undefined;
      return { key, belt: merged, unsaved, sourceKey: base.sourceKey, basis: current?.basis };
    });
    refreshContext();
  };

  /** Sends one structural edit with the draft as it is now. */
  const operate = (operation: DraftOperation, operands: DraftOperands, applies?: AppliedForm, select?: string): boolean => {
    const submitted = drafts.latest(key)?.belt ?? belt;
    return command.submit(draftRequest(operation, submitted, operands, (_queryClient, response) => {
      settle(submitted, response, applies);
      // A stage added from the line opens in the form, as the app's "+" did.
      if (select) drafts.selectStage(key, select);
    }));
  };

  const updateStage = (stageId: string) => {
    const submitted = drafts.latest(key)?.belt ?? belt;
    command.submit(updateStageRequest(submitted, stageId, (_queryClient, response) => settle(submitted, response)));
  };

  const updateAll = () => {
    const submitted = drafts.latest(key)?.belt ?? belt;
    command.submit(updateAllStagesRequest(submitted, (_queryClient, response) => settle(submitted, response)));
  };

  /** A stage the daemon builds for the draft, then added or fed in. The template is read-only; the edit that follows changes the draft. */
  const addStage = async ({ type, agent, route, feed, index }: StageRequest): Promise<void> => {
    setTemplateFailure(null);
    const draft = drafts.latest(key)?.belt ?? belt;
    const template = stageTemplateRequest(draft, type, agent, route, index);
    try {
      const built = await apiRequest<BeltStageTemplateResponse>(template.path, { method: "POST", body: template.body, onUnauthorized: runtime.requireSignIn });
      if (feed) operate("feed-into", { stageId: feed, stage: built.stage });
      else operate("stage-add", { stage: built.stage, route, ...(index !== undefined ? { index } : {}) }, undefined, built.stage.id);
    } catch (error) {
      setTemplateFailure(error);
    }
  };

  /** Saves the submitted draft. On success the belt is the saved version, and `then` runs with its key. */
  const saveDraft = (then?: (savedKey: string) => void) => {
    let submitted = drafts.latest(key)?.belt ?? belt;
    const forms = Object.entries(typedForms).map(([form, applied]) => ({ form, applied: { ...applied } }));
    const answer = (queryClient: QueryClient, response: BeltResourceResponse) => {
      publishBeltResource(queryClient, response);
      // A shadow's draft keeps its key: its belt is the same one, saved as a shadow version.
      const savedKey = shadowMode ? key : beltRecord(response.resource).belt.id;
      drafts.setWarnings(savedKey, response.warnings);
      // Only the submitted edits are cleared; anything typed while the save was pending stays in the draft.
      drafts.move(key, savedKey, current => {
        if (!current || sameBelt(current.belt, submitted)) return undefined;
        return { ...current, unsaved: false, sourceKey: null };
      });
      refreshContext();
      if (savedKey !== key) followSelection(key, savedKey);
      then?.(savedKey);
    };
    const request = shadowMode ? shadowSaveRequest(beltId, submitted, answer) : saveRequest(submitted, answer);
    if (forms.length > 0) request.prepare = async () => {
      // Typed input is applied first, as each form's own Enter or Apply would, so Save never waits on it.
      for (const applies of forms) {
        const operation = typedOperation(submitted, applies.form, applies.applied);
        if (!operation) continue;
        const edit = draftRequest(operation.op, submitted, operation.operands, () => {});
        const response = await apiRequest<BeltDraftResponse>(edit.path, { method: "POST", body: edit.body, onUnauthorized: runtime.requireSignIn });
        settle(submitted, response, applies);
        submitted = response.draft;
      }
      return { belt: submitted };
    };
    command.submit(request);
  };

  const saves = useBeltSaveQueue(command.pending, saveDraft);
  const requestSave = () => saves.request(missing);

  useEffect(() => {
    onAssistantState?.({ dirty: dirty || hasTyped, saveAvailable: saveGate.enabled, save: afterSave => saves.saveLatest(() => afterSave?.()) });
    return () => onAssistantState?.(null);
  }, [dirty, hasTyped, saveGate.enabled, onAssistantState]);

  const copy = () => {
    const sourceKey = key;
    const newKey = drafts.nextKey();
    command.submit(copyRequest(belt, (_queryClient, response) => {
      drafts.setWarnings(newKey, response.warnings);
      drafts.update(newKey, () => ({ key: newKey, belt: response.draft, unsaved: true, sourceKey }));
      followSelection(sourceKey, newKey);
    }));
  };

  const discard = () => {
    drafts.update(key, () => undefined);
    drafts.dropTyped(key);
    setSelectedStageId(null);
    // A new shadow that is discarded before its first save leaves the live version showing.
    if (shadowMode && unsaved) { onShadowMode?.(false); return; }
    if (!unsaved) return;
    const source = base.sourceKey;
    const sourceExists = source !== null && (source in drafts.drafts || allIds.includes(source));
    onSelect(sourceExists ? source : liveIds[0] ?? null);
  };

  /** Takes the daemon's answer to an archive, pause or AFK command: the library takes the belt and the context is read again. */
  const answerResource = (queryClient: QueryClient, response: BeltResourceResponse) => {
    publishBeltResource(queryClient, response);
    drafts.setWarnings(key, response.warnings);
    refreshContext();
  };

  const setArchived = () => command.submit(archiveRequest(key, !archived, answerResource));

  const setPaused = () => command.submit(pauseRequest(key, !paused, answerResource));

  const setAfk = (afk: AfkMode) => command.submit(afkRequest(key, afk, answerResource));

  const routeName = (route: string): string => {
    for (const fork of belt.forks) {
      const found = fork.routes.find(item => item.id === route);
      if (found) return found.name;
    }
    return route;
  };

  const stageById = (id: string): Stage | undefined => belt.stages.find(stage => stage.id === id);

  // The daemon words both notes (`BeltEditorWarning`); only the browser knows whether input is unsent, so it picks one.
  const pauseNote = (dirty ? context?.notes?.whileEditing : context?.notes?.whenSaved) ?? null;

  // Share opens the library's share sheet for the saved version; it is never offered for a draft with unsaved changes.
  const shareGate: Gate = !snapshot ? { enabled: false, reason: "Save this belt before sharing it." }
    : dirty || hasTyped ? { enabled: false, reason: "Save changes before sharing" }
    : actionGate("share");

  const located = locatedProblems(command.error ?? templateFailure);
  const problemTarget: DraftEditorValue["problemTarget"] = (kind, id, field) => {
    const target = locationKey(kind, id, field);
    const problem = located.get(target);
    return problem ? { "data-problem-target": target, "aria-invalid": true, "aria-describedby": problem.domId } : { "data-problem-target": target };
  };

  const value: DraftEditorValue = {
    belt,
    blocked: blockedReason,
    structuralReason: structuralGate.reason,
    pending: command.pending,
    operate,
    typed: form => typedForms[form] ?? {},
    setTyped: (form, field, value) => drafts.type(key, form, field, value),
    clearTyped: form => drafts.settleTyped(key, form),
    addStage,
    edit,
    stash: drafts.stashes[key],
    setStash: stash => drafts.setStash(key, stash),
    failure: command.error ?? templateFailure,
    clearFailure: () => { command.clearError(); setTemplateFailure(null); },
    problemTarget,
    selectedStageId,
    selectStage: setSelectedStageId,
    routeName,
    stageById,
    context,
    shadow: shadowMode
  };

  const editing = dirty || unsaved || hasTyped;
  const resumeBlocked = paused && (snapshot?.warnings.length ?? 0) > 0;
  const note = shadowMode
    ? (editing ? (unsaved ? `A shadow of v${snapshot?.belt.version ?? belt.version}. Saving it starts the test.`
      : `Unsaved changes to the shadow. Saving makes a new shadow version; runs already on v${belt.version} keep it.`) : null)
    : pauseNote ?? (editing ? (unsaved ? "A new belt. It starts taking issues once you save it."
      : `Unsaved changes. Existing runs keep the stages, policies and agents of v${belt.version}. Saving applies the AFK choice to existing and new runs.`) : null);
  const tag = shadowMode ? (unsaved && !shadowSummary ? "New shadow" : `Belt · shadow v${belt.version} · ${paused ? "paused" : "running"}`)
    : unsaved ? "New belt" : `Belt · v${belt.version} · ${paused ? "paused" : "running"}`;

  return <section className="belt-editor" aria-label="Belt editor" data-column="belts-editor" data-belt={key}>
    <header className="belt-editor-header">
      <div className="belt-editor-title">
        <Tag>{tag}</Tag>
        <h2 className={belt.name ? "" : "is-unnamed"}>{belt.name || "Unnamed belt"}</h2>
        {snapshot?.origin && <p className="field-note belt-origin">{`Installed from ${snapshot.origin.repository}${snapshot.origin.path ? ` / ${snapshot.origin.path}` : ""}.`}</p>}
      </div>
      <BeltEditorActions belt={belt} snapshot={snapshot} archived={archived} paused={paused} editing={editing}
        outdated={context?.outdatedStages ?? 0} saveGate={saveGate} startGate={startGate} shareGate={shareGate} saving={saves.busy} actionGate={actionGate}
        assistantButton={assistantButton} onShare={() => onShare(key, belt.version, belt.name)} onUpdateAll={updateAll}
        onStartTask={() => setSheetOpen(true)} onPause={setPaused} onAfk={setAfk} onArchive={setArchived} onCopy={copy}
        onDiscard={discard} onSave={requestSave}
        shadowMode={shadowMode} saveLabel={shadowMode && !shadowSummary ? "Start shadow test" : "Save"}
        onCreateShadow={onShadowMode ? () => onShadowMode(true) : undefined} onDecideShadow={setDecision} />
    </header>
    {snapshot && onShadowMode && (shadowMode || shadowSummary) && <ShadowBar beltId={beltId} liveVersion={snapshot.belt.version}
      summary={shadowSummary} shadowMode={shadowMode} onMode={onShadowMode} />}

    <DraftEditorProvider value={value}>
      <StageLine line={context?.line ?? null} agents={agentChoices.agents} agentChoice={agentChoices.choice}
        onAgentChoice={agentChoices.setChoice} onNeedAgents={agentChoices.needAgents}
        onUpdateStage={stageId => updateStage(stageId)} updateStageEnabled={gate(actionFor("update-stage")).enabled} />

      {(note || resumeBlocked) && <div className="belt-notes">
        {note && <p className="belt-note"><span className="belt-note-lamp" aria-hidden="true" />{note}</p>}
        {resumeBlocked && <p className="belt-note-plain">Resolve the warnings before resuming this belt.</p>}
      </div>}
      {warnings.length > 0 && <ul className="belt-warnings" aria-label="Warnings">{warnings.map((warning, index) => <li key={index}>{warning}</li>)}</ul>}

      <ProblemSummary failure={command.error ?? templateFailure} onDismiss={value.clearFailure}
        onReveal={location => { if ((location.kind === "stage" || location.kind === "outcome") && location.id) setSelectedStageId(location.id); }} />

      <div className="belt-forms">
        {selectedStageId !== null && stageById(selectedStageId) ? <StageForm key={selectedStageId} stage={stageById(selectedStageId)!}
          onEditAgent={agent => setAgentLeave(agent)} onUpdateStage={stageId => updateStage(stageId)}
          updateStageEnabled={gate(actionFor("update-stage")).enabled} />
          : <>
            <SharedBeltCard beltKey={key} />
            {shadowMode ? <fieldset className="shadow-locked-fields" disabled>
              <p className="field-note shadow-locked">The triggers come from the live version. Change them there; the shadow follows.</p>
              <TriggerForm />
            </fieldset> : <TriggerForm />}
          </>}
      </div>
      {decision && shadowSummary && snapshot && <ShadowDecisionSheet beltId={beltId} beltName={snapshot.belt.name} shadowLabel={shadowSummary.label}
        test={shadowSummary.test} decision={decision} onClose={() => setDecision(null)}
        onDone={() => { drafts.update(key, () => undefined); drafts.dropTyped(key); onShadowMode?.(false); }} />}
      {sheetOpen && saved && <TaskSheet belt={saved} context={dirty ? null : context} onClose={() => setSheetOpen(false)} />}
      {agentLeave && <AgentLeave agent={agentLeave} dirty={dirty || hasTyped} canSave={saveGate.enabled}
        onClose={() => setAgentLeave(null)}
        onOpen={() => { setAgentLeave(null); navigate(`/agents?agent=${encodeURIComponent(`${agentLeave.id}@${agentLeave.version}`)}`); }}
        onSaveAndOpen={() => saveDraft(() => {
          setAgentLeave(null);
          navigate(`/agents?agent=${encodeURIComponent(`${agentLeave.id}@${agentLeave.version}`)}`);
        })} />}
    </DraftEditorProvider>
  </section>;
}
