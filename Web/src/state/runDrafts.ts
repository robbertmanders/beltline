/**
 * Unsent run input, owned by the browser. Drafts are keyed by run and source stage so navigation
 * between pages, stages and attempts never mixes them. They live in per-tab sessionStorage (so a reload
 * in the same tab keeps them) with an in-memory fallback when storage is unavailable; they are never
 * written to localStorage, the daemon or another tab.
 */

export const runDraftsStorageKey = "beltline.web.runDrafts";

export type DraftField = "answer" | "decisionNote" | "sendMessage";
export type PendingOutcome = string | null;

export interface StageDraft {
  answer: string;
  /** Decision notes, one per outcome id, so changing outcome shows that outcome's note. */
  notes: Record<string, string>;
  /** The outcome id whose note box is open, or null after Cancel. */
  pendingOutcome: string | null;
  includeSubject?: boolean;
  sendMessage: string;
}

export interface RunDrafts {
  [stageId: string]: Partial<StageDraft>;
}

export interface RunDraftState {
  [runId: string]: RunDrafts;
}

interface PersistedState {
  version: 1;
  runs: RunDraftState;
}

const versionKey = (runId: string, stageId: string, field: DraftField) => `${runId}\u0000${stageId}\u0000${field}`;

export const emptyStageDraft: StageDraft = { answer: "", notes: {}, pendingOutcome: null, sendMessage: "" };

function parse(serialized: string | null): RunDraftState {
  if (!serialized) return {};
  try {
    const parsed = JSON.parse(serialized) as PersistedState;
    return parsed && parsed.version === 1 && parsed.runs && typeof parsed.runs === "object" ? parsed.runs : {};
  } catch {
    return {};
  }
}

/**
 * A small observable store. Every edit bumps a per-field version; a command records the version it
 * submitted so a late success clears only the text that was actually sent.
 */
export class RunDraftStore {
  private state: RunDraftState;
  private versions = new Map<string, number>();
  private listeners = new Set<() => void>();
  private storage: Pick<Storage, "getItem" | "setItem"> | null;

  constructor(storage: Pick<Storage, "getItem" | "setItem"> | null = safeSessionStorage()) {
    this.storage = storage;
    this.state = parse(storage?.getItem(runDraftsStorageKey) ?? null);
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  };

  getSnapshot = (): RunDraftState => this.state;

  private commit(next: RunDraftState): void {
    this.state = next;
    try { this.storage?.setItem(runDraftsStorageKey, JSON.stringify({ version: 1, runs: next } satisfies PersistedState)); }
    catch { /* Keep the in-memory copy when storage fills or is restricted. */ }
    for (const listener of this.listeners) listener();
  }

  stage(runId: string, stageId: string): StageDraft {
    return { ...emptyStageDraft, ...this.state[runId]?.[stageId] };
  }

  private write(runId: string, stageId: string, patch: Partial<StageDraft>): void {
    const run = { ...(this.state[runId] ?? {}) };
    run[stageId] = { ...run[stageId], ...patch };
    this.commit({ ...this.state, [runId]: run });
  }

  setAnswer(runId: string, stageId: string, value: string): number {
    this.write(runId, stageId, { answer: value });
    return this.bump(runId, stageId, "answer");
  }

  /** The note kept for one outcome of a stage; changing outcome shows the new outcome's note. */
  note(runId: string, stageId: string, outcomeId: string): string {
    return this.state[runId]?.[stageId]?.notes?.[outcomeId] ?? "";
  }

  setNote(runId: string, stageId: string, outcomeId: string, value: string): void {
    const current = this.state[runId]?.[stageId]?.notes ?? {};
    this.write(runId, stageId, { notes: { ...current, [outcomeId]: value } });
  }

  setPendingOutcome(runId: string, stageId: string, outcome: PendingOutcome): void {
    this.write(runId, stageId, { pendingOutcome: outcome });
  }

  setIncludeSubject(runId: string, stageId: string, value: boolean): void {
    this.write(runId, stageId, { includeSubject: value });
  }

  setSendMessage(runId: string, stageId: string, value: string): void {
    this.write(runId, stageId, { sendMessage: value });
    this.bump(runId, stageId, "sendMessage");
  }

  /** The version token a command records before submitting, to tell later edits apart. */
  version(runId: string, stageId: string, field: DraftField): number {
    return this.versions.get(versionKey(runId, stageId, field)) ?? 0;
  }

  private bump(runId: string, stageId: string, field: DraftField): number {
    const key = versionKey(runId, stageId, field);
    const next = (this.versions.get(key) ?? 0) + 1;
    this.versions.set(key, next);
    return next;
  }

  /** Clears the submitted answer only when it was not edited after submission. */
  clearAnswer(runId: string, stageId: string, submittedVersion: number): void {
    if (this.version(runId, stageId, "answer") !== submittedVersion) return;
    this.write(runId, stageId, { answer: "" });
    this.versions.set(versionKey(runId, stageId, "answer"), submittedVersion + 1);
  }

  /** Decide clears that stage's per-outcome notes and its pending selection. */
  clearDecide(runId: string, stageId: string): void {
    this.write(runId, stageId, { notes: {}, pendingOutcome: null, includeSubject: undefined });
  }

  /** Resume, whole-run Drop and Erase clear every draft of the run. */
  clearRun(runId: string): void {
    if (this.state[runId] === undefined) return;
    const next = { ...this.state };
    delete next[runId];
    this.commit(next);
  }

  /** Cancel clears the pending decision selection without touching the note. */
  cancelDecision(runId: string, stageId: string): void {
    this.write(runId, stageId, { pendingOutcome: null });
  }

  /** Test and lifecycle helper: forget every draft without touching storage history. */
  clearAll(): void {
    this.versions.clear();
    this.commit({});
  }
}

function safeSessionStorage(): Pick<Storage, "getItem" | "setItem"> | null {
  try {
    const storage = typeof window === "undefined" ? null : window.sessionStorage;
    const probe = "beltline.web.runDrafts.probe";
    storage?.setItem(probe, "1");
    storage?.getItem(probe);
    return storage;
  } catch {
    return null;
  }
}

export const runDraftStore = new RunDraftStore();
