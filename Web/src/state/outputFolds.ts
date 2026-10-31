/**
 * Which sections of an agent's output this browser has opened. The choice is browser-local: it belongs to one
 * run and one attempt, and the daemon never sees it. Each section is keyed by the daemon's section ID, so a
 * stored choice for a heading that no longer exists is simply never read.
 *
 * Storage can be unreadable, hold malformed JSON or refuse writes. Reads then give no saved choices, and writes
 * report `false`; the caller keeps its own copy, so the sections still open and close for the current view.
 */

export const outputFoldsStorageKey = "beltline.outputFolds.v1";

/** A section ID to whether it is open, for one attempt. */
export type OutputFolds = Record<string, boolean>;

/** Every saved choice, by run ID, then attempt ID. Only boolean leaves are kept. */
type StoredFolds = Record<string, Record<string, OutputFolds>>;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

/** The saved choices that have the expected shape. Anything else is dropped rather than trusted. */
function parseFolds(raw: string | null): StoredFolds {
  if (raw === null) return {};
  let parsed: unknown;
  try { parsed = JSON.parse(raw); } catch { return {}; }
  if (!isRecord(parsed)) return {};
  const folds: StoredFolds = {};
  for (const [runId, attempts] of Object.entries(parsed)) {
    if (!isRecord(attempts)) continue;
    for (const [attemptId, sections] of Object.entries(attempts)) {
      if (!isRecord(sections)) continue;
      const open: OutputFolds = {};
      for (const [sectionId, value] of Object.entries(sections)) {
        if (typeof value === "boolean") open[sectionId] = value;
      }
      folds[runId] = { ...folds[runId], [attemptId]: open };
    }
  }
  return folds;
}

/** The choices this browser saved for one attempt, or none when storage cannot be read. */
export function readOutputFolds(runId: string, attemptId: string, storage?: Pick<Storage, "getItem">): OutputFolds {
  try {
    return parseFolds((storage ?? window.localStorage).getItem(outputFoldsStorageKey))[runId]?.[attemptId] ?? {};
  } catch {
    return {};
  }
}

/**
 * Saves the choices for one attempt, keeping the other attempts' choices. Returns false when storage refused the
 * write; the choices still apply for the current view.
 */
export function writeOutputFolds(runId: string, attemptId: string, folds: OutputFolds, storage?: Pick<Storage, "getItem" | "setItem">): boolean {
  try {
    const target = storage ?? window.localStorage;
    const stored = parseFolds(target.getItem(outputFoldsStorageKey));
    stored[runId] = { ...stored[runId], [attemptId]: folds };
    target.setItem(outputFoldsStorageKey, JSON.stringify(stored));
    return true;
  } catch {
    return false;
  }
}
