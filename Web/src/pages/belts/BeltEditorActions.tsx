import type { ReactNode } from "react";
import type { AfkMode, Belt, BeltAction } from "../../api/wire.gen";
import { ActionButton, type Gate } from "./BeltActionButton";
import type { BeltRecord } from "./beltRecords";
import { afkHelp } from "./beltEditorHelpers";
import { manualEntries } from "./TriggerForm";

export interface BeltEditorActionsProps {
  belt: Belt;
  /** The library entry for the saved belt; null for a new or copied belt, which has no saved commands. */
  snapshot: BeltRecord | null;
  archived: boolean;
  paused: boolean;
  /** True while the draft has unsent changes or typed input, so Discard shows and Save shows when it can be used. */
  editing: boolean;
  /** The stages whose agent has a newer version; the Update offer reads the count. */
  outdated: number;
  saveGate: Gate;
  /** A new shadow's start, which the daemon may refuse: a refused start stays on the belt header, with its reason, before any edit. */
  startGate?: Gate;
  shareGate: Gate;
  saving: boolean;
  /** The gate for a belt action by its daemon id; with no id it gates on the connection and a running command alone. */
  actionGate: (id?: BeltAction["id"]) => Gate;
  assistantButton?: ReactNode;
  onShare: () => void;
  onUpdateAll: () => void;
  onStartTask: () => void;
  onPause: () => void;
  onAfk: (mode: AfkMode) => void;
  onArchive: () => void;
  onCopy: () => void;
  /** Set while the editor shows the belt's shadow: the live version's own commands give way to the shadow's. */
  shadowMode?: boolean;
  /** Save's label, such as "Start shadow test" for a shadow not saved yet. */
  saveLabel?: string;
  onCreateShadow?: () => void;
  onDecideShadow?: (decision: "accept" | "discard") => void;
  onDiscard: () => void;
  onSave: () => void;
}

/** The belt header's action buttons. Their gates and labels come from the daemon; this only decides which are shown. */
export function BeltEditorActions({ belt, snapshot, archived, paused, editing, outdated, saveGate, startGate, shareGate, saving, actionGate, assistantButton, onShare, onUpdateAll, onStartTask, onPause, onAfk, onArchive, onCopy, onDiscard, onSave,
  shadowMode = false, saveLabel = "Save", onCreateShadow, onDecideShadow }: BeltEditorActionsProps) {
  const manual = snapshot ? manualEntries(snapshot.belt) : [];
  // The AFK button's on-state, label and next mode come from the daemon's belt resource.
  const afkOn = snapshot?.afk.on ?? false;
  const afkLabel = snapshot?.afk.label ?? "AFK: Global";
  const nextAfk: AfkMode = snapshot?.afk.next ?? "always";

  const live = !shadowMode;
  const shadowRuns = snapshot?.shadow !== undefined;
  // The daemon refuses the new shadow's start: its Start button stays, disabled with the reason, in place of Save.
  const refusedStart = startGate !== undefined && !startGate.enabled;
  return <div className="belt-actions">
    {live && <ActionButton label="Share…" kind="slate" gate={shareGate} help="Preview and propose this saved belt version" onClick={onShare} />}
    {snapshot && !archived && outdated > 0 && <ActionButton label={outdated === 1 ? "Update 1 stage" : `Update all ${outdated} stages`} gate={actionGate("update-all-stages")}
      help="Move every stage whose agent has a newer version to it. Settings you changed on a stage stay. Save to start using it." onClick={onUpdateAll} />}
    {live && snapshot && !archived && manual.length > 0 && <ActionButton label="Start a task…" kind="slate" gate={actionGate()} help="Start a run on a task you describe" onClick={onStartTask} />}
    {snapshot && !archived && <ActionButton label={paused ? "Resume belt" : "Pause belt"} gate={actionGate(paused ? "resume" : "pause")}
      help="A paused belt takes no new issues and starts no new sessions. Runs already going keep going." onClick={onPause} />}
    {live && snapshot && !archived && <ActionButton ariaLabel="AFK mode for new runs" label={<><span className={`afk-lamp ${afkOn ? "is-on" : ""}`} aria-hidden="true" />{afkLabel}</>}
      kind={`belt-afk ${afkOn ? "slate" : ""}`} gate={actionGate("afk")} help={`${afkHelp(belt.afk, afkOn)} Click for ${nextAfk === "always" ? "AFK: Always" : nextAfk === "never" ? "AFK: Never" : "AFK: Global"}.`}
      onClick={() => onAfk(nextAfk)} />}
    {live && snapshot && <ActionButton label={archived ? "Restore" : "Archive"} gate={actionGate(archived ? "unarchive" : "archive")} onClick={onArchive} />}
    {live && snapshot && <ActionButton label="Copy belt" gate={actionGate("copy")} help="Start a new belt from this one. It takes no issues until you save it." onClick={onCopy} />}
    {live && snapshot && !archived && !shadowRuns && onCreateShadow && <ActionButton label="Create shadow…" kind="shadow-outline" gate={actionGate("shadow-create")}
      help="Try a changed copy of this belt on half of its new runs, then accept or discard it." onClick={onCreateShadow} />}
    {shadowMode && shadowRuns && onDecideShadow && <>
      <ActionButton label="Discard shadow…" gate={actionGate("shadow-discard")} help="End the test and keep the live version." onClick={() => onDecideShadow("discard")} />
      <ActionButton label="Accept shadow…" kind="shadow-primary" gate={actionGate("shadow-accept")} help="Make the shadow the live version and end the test." onClick={() => onDecideShadow("accept")} />
    </>}
    {editing && <ActionButton label="Discard" gate={{ enabled: true, reason: null }} onClick={onDiscard} />}
    {editing && saveGate.enabled && !refusedStart && <ActionButton label={saving ? "Saving…" : saveLabel} kind={shadowMode ? "shadow-primary" : "primary"} gate={saveGate} onClick={onSave} />}
    {refusedStart && <ActionButton label={saveLabel} kind="shadow-primary" gate={startGate} onClick={onSave} />}
    {assistantButton}
  </div>;
}
