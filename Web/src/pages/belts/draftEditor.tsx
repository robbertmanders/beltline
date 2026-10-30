import { createContext, useContext } from "react";
import type { Belt, BeltContext, Stage } from "../../api/wire.gen";
import type { DraftOperands, DraftOperation } from "./beltCommands";
import { sameBelt, type TriggerStash, type TypedInput } from "./beltDrafts";

/** A form whose typed input an edit applies: when the daemon accepts the edit, the applied fields stop being unsent. */
export interface AppliedForm {
  form: string;
  applied: TypedInput;
  /** Whether the daemon's answer took the input. When it did not, the input stays and the form is marked `unchanged`. */
  took?: (answer: Belt) => boolean;
}

export type ProblemTargetKind = "belt" | "fork" | "route" | "stage" | "trigger" | "repository" | "outcome";

export interface ProblemTargetProps {
  "data-problem-target": string;
  "aria-invalid"?: true;
  "aria-describedby"?: string;
}

/** The typed field a form gets when the daemon's answer left the draft as it was. */
export const unchangedField = "__unchanged";

/** A stage the editor asks the daemon to build, then to add to the end of the line or feed into a stage. */
export interface StageRequest {
  type: "agent" | "approve" | "prompt" | "watch";
  agent?: { id: string; version: number };
  /** The route a new stage joins; absent for the main line. */
  route?: string;
  /** Feeds the new stage into this stage instead of adding it to the line. */
  feed?: string;
  /** Where in the stage list the stage goes; the end when absent. */
  index?: number;
}

/**
 * What the belt's structural controls use. Every structural edit goes through `operate`, which sends the
 * whole current draft and lets the daemon's answer replace it; typing goes through `edit` and sends nothing.
 */
export interface DraftEditorValue {
  belt: Belt;
  /** Why a structural edit cannot be sent now, or null when one can. */
  blocked: string | null;
  /** Why a structural edit is unavailable on this belt, from the daemon's action for it, or null. */
  structuralReason: string | null;
  pending: boolean;
  /** Sends one structural edit with the current draft. False when a command is already running. */
  operate: (operation: DraftOperation, operands: DraftOperands, applies?: AppliedForm, select?: string) => boolean;
  /** The fields typed into a form and not yet applied. They survive navigation within the app session. */
  typed: (form: string) => TypedInput;
  setTyped: (form: string, field: string, value: unknown) => void;
  /** Drops a form's typed fields, so it shows the draft again. */
  clearTyped: (form: string) => void;
  /** Asks the daemon for a stage, then adds it or feeds it in. */
  addStage: (request: StageRequest) => Promise<void>;
  /** Types into the draft. Local only. */
  edit: (change: (belt: Belt) => Belt) => void;
  stash: TriggerStash | undefined;
  setStash: (stash: TriggerStash) => void;
  /** The last failure, from a command or from building a stage. */
  failure: unknown;
  clearFailure: () => void;
  /**
   * The props that tie a field to the located problem about it: `data-problem-target` so the summary can focus
   * it, and, while the last failure names it, `aria-invalid` and `aria-describedby` pointing at the problem.
   */
  problemTarget: (kind: ProblemTargetKind, id?: string, field?: string) => ProblemTargetProps;
  /** The stage the editor shows in its form, or null. */
  selectedStageId: string | null;
  selectStage: (id: string | null) => void;
  /** The daemon's context for the draft: menus, outcome labels, placeholder help and the actions it may take. Null until read. */
  context: BeltContext | null;
  /** The routes of every fork, for the stage line's route labels. */
  routeName: (route: string) => string;
  /** The stage on the belt with this id. */
  stageById: (id: string) => Stage | undefined;
  /** True while the draft is the belt's shadow: its triggers, name and bases are live's and cannot change here. */
  shadow?: boolean;
}

const DraftEditorContext = createContext<DraftEditorValue | null>(null);

export const DraftEditorProvider = DraftEditorContext.Provider;

export function useDraftEditor(): DraftEditorValue {
  const value = useContext(DraftEditorContext);
  if (!value) throw new Error("useDraftEditor must be used within a belt editor");
  return value;
}

/**
 * One form's values: what the draft holds, with anything typed and not yet applied on top. Typing goes to the
 * app-level store, so it survives navigation; `applies` names the typed fields an edit sends, so the daemon's
 * answer drops only those and keeps anything typed while it was pending.
 */
export function useTypedForm<T extends Record<string, unknown>>(form: string, fromDraft: T) {
  const editor = useDraftEditor();
  const typed = editor.typed(form);
  const values = { ...fromDraft, ...typed } as T;
  const set = <K extends keyof T & string>(field: K, value: T[K]) => {
    // A pending answer may replace the draft, so keep even edits back to its current value.
    editor.setTyped(form, field, !editor.pending && sameBelt(value, fromDraft[field]) ? undefined : value);
  };
  const applies = (): AppliedForm => ({ form, applied: { ...typed } });
  return { values, set, applies, typed: Object.keys(typed).length > 0, clear: () => editor.clearTyped(form) };
}
