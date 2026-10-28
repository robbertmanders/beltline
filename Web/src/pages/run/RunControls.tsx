import { Fragment, useState, type ReactNode } from "react";
import { ErrorProblems } from "../../components/ProblemList";
import { Sheet } from "../../components/Sheet";
import { findRunAction, type RunCommandId, type RunCommandOperands } from "../../api/runCommands";
import type { Run, RunAction, RunMenuRow, RunStage, RunViewResponse, Stage } from "../../api/wire.gen";
import type { CommandController } from "../../api/useCommand";
import type { ConnectionStatus } from "../../api/eventConnection";
import type { StageDraft } from "../../state/runDrafts";
import { RunMenu } from "./RunMenu";
import { RunStageAdjust } from "./RunStageAdjust";
import { isDesktopCommand } from "./RunLinks";

const afkModes = [{ id: "global", label: "Global" }, { id: "always", label: "Always" }, { id: "never", label: "Never" }] as const;

/**
 * Sends one run command. onSuccess runs only after the daemon's validated success, so a refusal leaves the
 * open menu, sheet or confirmation exactly where it was.
 */
export type RunCommandSubmit = (id: RunCommandId, operands: RunCommandOperands, onSuccess?: () => void) => void;

export interface RunControlsProps {
  runId: string;
  run: Run;
  view: RunViewResponse;
  stage: Stage;
  panel: RunStage;
  selectedAttemptId: string | null;
  draft: StageDraft;
  onAnswerChange: (value: string) => void;
  onDecisionNoteChange: (outcomeId: string, value: string) => void;
  onPendingOutcomeChange: (value: string | null) => void;
  onIncludeSubjectChange: (value: boolean) => void;
  onSendMessageChange: (value: string) => void;
  answerVersion: () => number;
  command: CommandController;
  submit: RunCommandSubmit;
  canRead: boolean;
  connection: ConnectionStatus;
  onErased: () => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The Adjust stage sheet, which the stage panel's Adjust button opens. */
  adjustOpen?: boolean;
  onAdjustOpenChange?: (open: boolean) => void;
}

/** The run's AFK button: its lamp lit while it is in AFK mode; a click moves to the next mode. */
export function RunAfkButton({ view, submit, online, pending }: { view: RunViewResponse; submit: RunCommandSubmit; online: boolean; pending: boolean }) {
  const action = findRunAction(view.actions, "afk", {});
  const enabled = online && !pending && action?.enabled === true;
  return <button type="button" className={`action-button small run-afk ${view.afk.on ? "slate" : ""}`} disabled={!enabled}
    aria-label="Run AFK mode" aria-valuetext={view.afk.label} title={action?.reason ?? view.afk.help}
    onClick={() => submit("afk", { afkMode: view.afk.next as "global" | "always" | "never" })}>
    <span className={`afk-lamp ${view.afk.on ? "is-on" : ""}`} aria-hidden="true" />{view.afk.label}
  </button>;
}

/** A slate shortcut beside the menus, such as Start now on a queued run. */
export function QuickAction({ view, id, label, submit, online, pending }: { view: RunViewResponse; id: "start-now" | "retry-continuation"; label: string; submit: RunCommandSubmit; online: boolean; pending: boolean }) {
  const action = findRunAction(view.actions, id, {});
  return <button type="button" className="action-button small slate" disabled={!online || pending || action?.enabled !== true}
    title={action?.reason ?? action?.help ?? undefined} onClick={() => submit(id, {})}>{label}</button>;
}

const menuIcons: Record<string, string> = {
  pause: "M5 3v10M11 3v10", play: "M5 3l8 5-8 5z", again: "M13 8a5 5 0 1 1-1.5-3.5M13 3v3h-3", send: "M3 8h9M9 4l4 4-4 4",
  park: "M2 9h12v4H2zM8 2v5M5.5 4.5L8 7l2.5-2.5", unpark: "M2 9h12v4H2zM8 7V2M5.5 4.5L8 2l2.5 2.5", stop: "M4 4h8v8H4z",
  drop: "M3 3h10v10H3zM6 6l4 4M10 6l-4 4", erase: "M3 4h10M6 4V2.5h4V4M4.5 4l.8 9.5h5.4l.8-9.5"
};

/** One row of the Run controls menu: an icon and its words, red for the ones that end something. */
function MenuRow({ icon, label, danger, action, onActivate, pending, online }: {
  icon: string; label: string; danger?: boolean; action: RunAction | undefined; onActivate: () => void; pending: boolean; online: boolean
}) {
  const enabled = online && !pending && action?.enabled === true;
  return <button type="button" className={`run-menu-row${danger ? " is-danger" : ""}`} disabled={!enabled} title={action?.reason ?? action?.help ?? undefined} onClick={onActivate}>
    <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d={menuIcons[icon]} /></svg>
    <span>{label}</span>
  </button>;
}

/** A command body sent by the page's own menu or sheets rather than by one of the inline forms. */
export function isInlineCommand(path: string | undefined): boolean {
  return path !== undefined && (path.endsWith("/answer") || path.endsWith("/decide"));
}

/** A refused command's problems, with a Try again that sends it once more. */
export function CommandFailure({ command }: { command: CommandController }) {
  return <div className="run-menu-error">
    <ErrorProblems error={command.error} />
    <button type="button" className="quiet-button" onClick={command.retry}>Try again</button>
  </div>;
}

interface Confirmation { id: "abort" | "drop" | "erase"; leg?: string }

/**
 * Every run command, offered only through the matching daemon action record, and the open links below them,
 * as one floating menu. The daemon lists the menu's rows, their words and the record each is enabled from;
 * every sheet's button is enabled from the record of what it sends. A missing or unknown action is
 * unavailable; a present refusal is shown in place. Forms
 * keep their text while a command is pending or refused, stay open until the daemon accepts it, and clear only
 * through the success path the container supplies. The sheets and confirmations stay mounted while the menu
 * closes, so they are rendered outside it.
 */
export function RunControls(props: RunControlsProps & { links?: ReactNode }) {
  const { run, view, stage, draft, command, submit, canRead, connection, open, onOpenChange, links } = props;
  const panel = props.panel;
  const [sheet, setSheet] = useState<null | "send" | "adjust" | "afk">(null);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [pendingSend, setPendingSend] = useState(stage.id);
  const [sendAgain, setSendAgain] = useState(false);
  const [sendTargets, setSendTargets] = useState<NonNullable<RunMenuRow["targets"]>>([]);
  const [sendEarlier, setSendEarlier] = useState(true);
  const [afkMode, setAfkMode] = useState<"global" | "always" | "never">("global");
  const pending = command.pending;
  const sendFacts = view.stages.find(item => item.id === pendingSend)?.facts?.send;
  const actions: readonly RunAction[] = view.actions;
  const actionFor = (id: string, selectors = {}) => findRunAction(actions, id, selectors);
  const online = connection === "connected" && canRead;
  const failed = command.error !== null && !isInlineCommand(command.lastRequest?.path) && !isDesktopCommand(command.lastRequest?.path);
  const adjustShown = sheet === "adjust" || props.adjustOpen === true;
  const closeSheet = () => { setSheet(null); props.onAdjustOpenChange?.(false); };

  const showSheet = (name: "send" | "adjust" | "afk") => {
    command.clearError();
    onOpenChange(false);
    setSheet(name);
  };
  const showConfirmation = (next: Confirmation) => {
    command.clearError();
    onOpenChange(false);
    setConfirmation(next);
  };
  // The Send sheet opens on the row's stage, offering the row's targets; Run again has none to choose.
  const openSend = (row: RunMenuRow) => {
    setSendAgain(row.opens === "sendAgain");
    setSendEarlier(true);
    setPendingSend(row.stage ?? stage.id);
    setSendTargets(row.targets ?? []);
    showSheet("send");
  };

  const activate = (row: RunMenuRow) => {
    switch (row.opens) {
      case "command": return submit(row.command, {}, () => onOpenChange(false));
      case "send": case "sendAgain": return openSend(row);
      case "confirm": return showConfirmation({ id: row.command as Confirmation["id"] });
    }
  };

  // Each sheet's button is enabled from the record of the command it sends.
  const sheetEnabled = (action: RunAction | undefined) => online && !pending && action?.enabled === true;
  const sendAction = actionFor("resume", { stage: pendingSend || stage.id });
  const adjustAction = actionFor("adjust-stage", { stage: stage.id });
  const afkAction = actionFor("afk");
  const confirmAction = confirmation ? actionFor(confirmation.id, confirmation.leg === undefined ? {} : { leg: confirmation.leg }) : undefined;

  // The daemon words what Abort and Drop do; Erase's words are the run's erasure copy.
  const confirmBody = confirmation?.id === "erase" ? view.erasureCopy
    : confirmation?.id === "abort" ? actionFor("abort")?.help ?? ""
    : actionFor("drop")?.help ?? "";

  const confirm = () => {
    const id = confirmation?.id;
    if (!id) return;
    const leg = confirmation?.leg;
    const accepted = () => setConfirmation(null);
    if (id === "erase") submit("erase", {}, accepted);
    else submit(id, leg === undefined ? {} : { leg }, accepted);
  };

  return <>
    <RunMenu label="Run actions" open={open} onOpenChange={value => { if (!value) command.clearError(); onOpenChange(value); }}>
      {view.menu.map(row => <Fragment key={`${row.command}-${row.label}`}>
        {row.divided && <div className="run-menu-divider" />}
        <MenuRow icon={row.icon} label={row.label} danger={row.danger} action={row.action} online={online} pending={pending} onActivate={() => activate(row)} />
      </Fragment>)}
      {failed && <CommandFailure command={command} />}
      {links && <>
        <div className="run-menu-divider" />
        {links}
      </>}
    </RunMenu>

    <Sheet title={run.stages.find(item => item.id === pendingSend)?.name ?? "Send to…"} tag="Send the run" width={620} open={sheet === "send"} onClose={() => setSheet(null)} footer={<>
      <button type="button" className="action-button" onClick={() => setSheet(null)}>Cancel</button>
      <button type="button" className="action-button primary" disabled={!sheetEnabled(sendAction)} title={sendAction?.reason ?? undefined}
        onClick={() => submit("resume", { stage: pendingSend || stage.id, note: draft.sendMessage.trim() === "" ? undefined : draft.sendMessage, earlier: sendEarlier }, () => setSheet(null))}>Send</button>
    </>}>
      {!sendAgain && <div className="sheet-field">
        <label htmlFor="send-to-stage">Stage</label>
        <select id="send-to-stage" value={pendingSend} onChange={event => setPendingSend(event.currentTarget.value)}>
          {sendTargets.map(item => <option key={item.stage} value={item.stage}>{item.name}</option>)}
        </select>
      </div>}
      {sendFacts?.earlier ? <>
        <label className="sheet-toggle"><input type="checkbox" checked={sendEarlier} onChange={event => setSendEarlier(event.currentTarget.checked)} /> {sendFacts.earlier}</label>
        {sendFacts.note && <p className="sheet-help">{sendFacts.note}</p>}
      </> : sendAgain && sendFacts && <p className="sheet-help">{sendFacts.again}</p>}
      <div className="sheet-field">
        <label htmlFor="send-to-message">Message (optional)</label>
        <textarea id="send-to-message" className="paper-field" value={draft.sendMessage} onChange={event => props.onSendMessageChange(event.currentTarget.value)} />
        <p className="sheet-help">The agent reads this before starting. {actionFor("resume")?.help}</p>
      </div>
      {failed && <ErrorProblems error={command.error} />}
    </Sheet>

    {adjustShown && (panel.adjust
      ? <RunStageAdjust key={`${props.runId}:${stage.id}`} runId={props.runId} stageId={stage.id} help={panel.adjust}
        action={adjustAction} enabled={sheetEnabled(adjustAction)} command={command} failed={failed} submit={submit} onClose={closeSheet} />
      : <Sheet title="Adjust stage" open onClose={closeSheet}><p role="status">Loading stage settings…</p></Sheet>)}

    <Sheet title="AFK" open={sheet === "afk"} onClose={() => setSheet(null)} footer={<>
      <button type="button" className="quiet-button" onClick={() => setSheet(null)}>Cancel</button>
      <button type="button" className="action-button primary" disabled={!sheetEnabled(afkAction)} title={afkAction?.reason ?? undefined}
        onClick={() => submit("afk", { afkMode }, () => setSheet(null))}>Apply</button>
    </>}>
      <fieldset className="run-radio-group">
        {afkModes.map(mode => <label key={mode.id}><input type="radio" name="afk-mode" checked={afkMode === mode.id} onChange={() => setAfkMode(mode.id)} /> {mode.label}</label>)}
      </fieldset>
      {failed && <ErrorProblems error={command.error} />}
    </Sheet>

    <Sheet title={confirmation?.id === "erase" ? `Erase ${view.reference}?` : confirmation?.id === "abort" ? `Abort ${view.reference}?` : `Drop ${view.reference}?`}
      className="run-confirmation" hideClose open={confirmation !== null} onClose={() => setConfirmation(null)} footer={<>
      <button type="button" className="run-confirmation-cancel" onClick={() => setConfirmation(null)}>Cancel</button>
      <button type="button" className="run-confirmation-destructive" disabled={!sheetEnabled(confirmAction)} title={confirmAction?.reason ?? undefined} onClick={confirm}>
        {confirmation?.id === "erase" ? "Erase the run" : confirmation?.id === "abort" ? "Abort the run" : "Drop the run"}
      </button>
    </>}>
      <p>{confirmBody}</p>
      {failed && <ErrorProblems error={command.error} />}
    </Sheet>
  </>;
}
