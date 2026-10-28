import type { QueryClient } from "@tanstack/react-query";
import { apiQueryKey } from "./resources";
import type { ContinueSessionResponse, EventResource, RunAction, RunResponse } from "./wire.gen";
import type { CommandRequest } from "./useCommand";

/** Every command the run page can post. The ids are also the daemon's endpoint segments. */
export const runCommandIds = [
  "decide", "resume", "answer", "abort", "drop", "park", "unpark", "pause", "resume-run",
  "retry-continuation", "start-now", "continue-session", "adjust-stage", "afk", "erase", "overrule-afk"
] as const;
export type RunCommandId = (typeof runCommandIds)[number];

/** The reads any run command can change: the run, its view, output and artifacts. */
export const runCommandAffects: readonly EventResource[] = ["runs", "library", "settings", "state"];

export interface RunActionSelectors {
  stage?: string;
  leg?: string;
  outcome?: string;
  attempt?: string;
  decision?: string;
}

function isUnscoped(action: RunAction): boolean {
  return action.stage === undefined && action.leg === undefined && action.outcome === undefined
    && action.attempt === undefined && action.decision === undefined;
}

/**
 * The contextual record matching every supplied selector, or the unscoped record when none are given.
 * A named selector never falls back to an enabled unscoped record: an absent contextual record leaves
 * the command unavailable.
 */
export function findRunAction(actions: readonly RunAction[], id: string, selectors: RunActionSelectors = {}): RunAction | undefined {
  const wanted = Object.entries(selectors).filter(([, value]) => value !== undefined && value !== null) as Array<[keyof RunActionSelectors, string]>;
  return actions.find(action => {
    if (action.id !== id) return false;
    if (wanted.length === 0) return isUnscoped(action);
    return wanted.every(([key, value]) => action[key] === value);
  });
}

export interface RunCommandOperands {
  stage?: string;
  leg?: string;
  outcome?: string;
  attempt?: string;
  decision?: string;
  note?: string;
  includeSubject?: boolean;
  answer?: string | null;
  earlier?: boolean;
  model?: string;
  thinking?: string | null;
  brief?: string | null;
  runner?: string | null;
  afkMode?: string;
  comments?: string;
  acceptedWarning?: boolean;
}

export interface RunCommandSpec {
  path: string;
  body: unknown;
}

/** The single POST a run control sends. Bodies mirror daemon/internal/server/api/api_run_commands.go exactly. */
export function runCommandSpec(id: RunCommandId, runId: string, operands: RunCommandOperands = {}): RunCommandSpec {
  const base = `/api/v1/runs/${encodeURIComponent(runId)}`;
  switch (id) {
    case "decide": return { path: `${base}/decide`, body: {
      outcome: operands.outcome, ...(operands.note === undefined ? {} : { note: operands.note }),
      ...(operands.includeSubject === undefined ? {} : { includeSubject: operands.includeSubject })
    } };
    case "resume": return {
      path: `${base}/resume`,
      body: {
        ...(operands.stage === undefined ? {} : { stage: operands.stage }),
        ...(operands.note === undefined ? {} : { note: operands.note }),
        ...(operands.leg === undefined ? {} : { leg: operands.leg }),
        ...(operands.earlier === undefined ? {} : { earlier: operands.earlier })
      }
    };
    case "answer": return { path: `${base}/answer`, body: { answer: operands.answer ?? null } };
    case "abort": return { path: `${base}/abort`, body: operands.leg === undefined ? {} : { leg: operands.leg } };
    case "drop": return { path: `${base}/drop`, body: operands.leg === undefined ? {} : { leg: operands.leg } };
    case "retry-continuation": return { path: `${base}/retry-continuation`, body: operands.leg === undefined ? {} : { leg: operands.leg } };
    case "continue-session": return { path: `${base}/continue-session`, body: { attempt: operands.attempt } };
    case "adjust-stage": return {
      path: `${base}/adjust-stage`,
      body: {
        stage: operands.stage,
        model: operands.model,
        ...(operands.thinking === undefined ? {} : { thinking: operands.thinking }),
        ...(operands.brief === undefined ? {} : { brief: operands.brief }),
        ...(operands.runner === undefined ? {} : { runner: operands.runner })
      }
    };
    case "afk": return { path: `${base}/afk`, body: { mode: operands.afkMode } };
    case "overrule-afk": return {
      path: `${base}/overrule-afk`,
      body: { decision: operands.decision, stage: operands.stage, comments: operands.comments ?? "", acceptedWarning: operands.acceptedWarning === true }
    };
    case "park": case "unpark": case "pause": case "resume-run": case "start-now": case "erase":
      return { path: `${base}/${id}`, body: {} };
  }
}

export type DesktopCommandId = "reveal" | "open-file" | "vscode" | "ghostty" | "open-url";

export interface DesktopOperands {
  kind?: string;
  attemptId?: string;
  name?: string;
  targetId?: string;
}

/** The desktop actions act on the daemon's Mac; the browser only forwards the typed selectors. */
export function desktopCommandSpec(id: DesktopCommandId, runId: string, operands: DesktopOperands = {}): RunCommandSpec {
  switch (id) {
    case "reveal": return {
      path: "/api/v1/desktop/reveal",
      body: { runId, kind: operands.kind, ...(operands.attemptId === undefined ? {} : { attemptId: operands.attemptId }), ...(operands.name === undefined ? {} : { name: operands.name }), ...(operands.targetId === undefined ? {} : { targetId: operands.targetId }) }
    };
    case "open-file": return { path: "/api/v1/desktop/open-file", body: { runId, attemptId: operands.attemptId } };
    case "vscode": return { path: "/api/v1/desktop/vscode", body: { runId, targetId: operands.targetId } };
    case "ghostty": return { path: "/api/v1/desktop/ghostty", body: { runId, attemptId: operands.attemptId } };
    case "open-url": return { path: "/api/v1/desktop/open-url", body: { runId, targetId: operands.targetId } };
  }
}

/** The run commands that answer with the updated run resource, so its cache can be published. */
export function runCommandRequest(runId: string, id: RunCommandId, operands: RunCommandOperands = {}): CommandRequest<unknown, RunResponse> {
  const spec = runCommandSpec(id, runId, operands);
  return {
    path: spec.path,
    body: spec.body,
    affects: runCommandAffects,
    publish: (queryClient: QueryClient, result: RunResponse) => publishRun(queryClient, runId, result.run)
  };
}

/**
 * Continue starts the attempt's conversation and returns the session it attaches. The browser attaches to
 * that target; the command it also returns is for the daemon's own desktop use and is never parsed here.
 */
export function continueSessionRequest(runId: string, attempt: string) {
  const spec = runCommandSpec("continue-session", runId, { attempt });
  return {
    path: spec.path,
    body: spec.body,
    affects: runCommandAffects,
    publish: (queryClient: QueryClient, result: ContinueSessionResponse) => { publishRun(queryClient, runId, result.run); }
  };
}

export function eraseRequest(runId: string) {
  const spec = runCommandSpec("erase", runId);
  return { path: spec.path, body: spec.body, affects: runCommandAffects, publish: () => {} };
}

export function desktopRequest(id: DesktopCommandId, runId: string, operands: DesktopOperands = {}) {
  const spec = desktopCommandSpec(id, runId, operands);
  return { path: spec.path, body: spec.body, affects: [] as const, publish: () => {} };
}

/** A fresh document link for one artifact. The listing's cached link is never reused to open it. */
export function artifactLinkRequest(runId: string, attemptId: string, name: string) {
  return {
    path: "/api/v1/artifacts/link",
    body: { runId, attemptId, name },
    affects: [] as const,
    publish: () => {}
  };
}

function publishRun(queryClient: QueryClient, runId: string, run: RunResponse["run"]): void {
  queryClient.setQueryData(apiQueryKey(`/api/v1/runs/${encodeURIComponent(runId)}`), { run });
}
