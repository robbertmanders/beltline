import type { QueryClient } from "@tanstack/react-query";
import type { AfkMode, Belt, BeltDraftResponse, BeltResourceResponse, EventResource } from "../../api/wire.gen";
import { apiQueryKey } from "../../api/resources";
import type { CommandRequest } from "../../api/useCommand";
import { beltRecord, type BeltList } from "./beltRecords";

export const beltsQueryKey = apiQueryKey("/api/v1/belts");

/** Every structural edit the daemon applies to a draft. Each takes the whole current draft and returns the draft it accepted. */
export type DraftOperation =
  | "stage-add" | "stage-remove" | "stage-move" | "stage-update"
  | "feed-into"
  | "fork-add" | "fork-remove"
  | "route-add" | "route-remove" | "route-update"
  | "trigger-add" | "trigger-remove" | "trigger-move" | "trigger-update" | "trigger-kind"
  | "repository-add" | "repository-remove"
  | "policies-update"
  | "goto-pick" | "goto-route";

/** The operands a draft operation names, as the daemon's `BeltDraftRequest` reads them. Absent fields are omitted from the body. */
export interface DraftOperands {
  stageId?: string;
  stage?: unknown;
  index?: number;
  route?: string;
  detach?: boolean;
  targetId?: string;
  step?: number;
  update?: Record<string, unknown>;
  forkId?: string;
  routeId?: string;
  routeName?: string;
  entry?: { id?: string; trigger?: unknown; repos?: string[] };
  entryId?: string;
  offset?: number;
  trigger?: unknown;
  kind?: string;
  repository?: string;
  stash?: unknown;
  helpFor?: string;
  outcomeId?: string;
  gotoTarget?: string;
  feeds?: boolean;
  policies?: { retriageOnEdit: boolean; stopOnClose: boolean; commentOnHold: boolean; waitForBlockers: boolean };
}

type Publish<TResult> = (queryClient: QueryClient, result: TResult) => void;

/** Draft edits change only unsent input, so they refresh no saved library read. */
const draftOnly: readonly EventResource[] = [];
/** Saved belt commands change the library, and the lists and editors built from it. */
const libraryChanged: readonly EventResource[] = ["library"];

/**
 * Writes a saved belt's answer into the list in place, adding it when it is new. The live ids stay as they were until the
 * daemon's read: an archive or restore changes them there, and the library refresh the command makes brings them.
 */
export function publishBeltResource(queryClient: QueryClient, response: BeltResourceResponse): void {
  queryClient.setQueryData<BeltList>(beltsQueryKey, list => {
    if (!list) return list;
    const saved = beltRecord(response.resource);
    const exists = list.belts.some(record => record.belt.id === saved.belt.id);
    const belts = exists
      ? list.belts.map(record => record.belt.id === saved.belt.id ? saved : record)
      : [...list.belts, saved];
    return { ...list, belts };
  });
}

export function draftRequest(
  operation: DraftOperation,
  draft: Belt,
  operands: DraftOperands,
  publish: Publish<BeltDraftResponse>
): CommandRequest<unknown, BeltDraftResponse> {
  return {
    path: `/api/v1/belts/draft/${operation}`,
    body: { draft, op: operation, ...operands },
    affects: draftOnly,
    publish
  };
}

/**
 * A stage the daemon builds for a draft, so the browser never makes stage defaults. Read-only: the
 * stage-add or feed-into that uses it is the command that changes the draft.
 */
export function stageTemplateRequest(draft: Belt, type: "agent" | "approve" | "prompt" | "watch", agent: { id: string; version: number } | undefined, route: string | undefined, index?: number) {
  return {
    path: "/api/v1/belts/stage-template",
    body: { draft, type, ...(agent ? { agent } : {}), ...(route ? { route } : {}), ...(index !== undefined ? { index } : {}) }
  };
}

export function saveRequest(belt: Belt, publish: Publish<BeltResourceResponse>): CommandRequest<unknown, BeltResourceResponse> {
  return { path: "/api/v1/belts/save", body: { belt }, affects: libraryChanged, publish };
}

export function newRequest(publish: Publish<BeltDraftResponse>): CommandRequest<unknown, BeltDraftResponse> {
  return { path: "/api/v1/belts/new", body: {}, affects: draftOnly, publish };
}

export function copyRequest(draft: Belt, publish: Publish<BeltDraftResponse>): CommandRequest<unknown, BeltDraftResponse> {
  return { path: "/api/v1/belts/copy", body: { draft }, affects: draftOnly, publish };
}

export function archiveRequest(id: string, archived: boolean, publish: Publish<BeltResourceResponse>): CommandRequest<unknown, BeltResourceResponse> {
  return {
    path: `/api/v1/belts/${encodeURIComponent(id)}/${archived ? "archive" : "unarchive"}`,
    body: {},
    affects: libraryChanged,
    publish
  };
}

export function pauseRequest(id: string, paused: boolean, publish: Publish<BeltResourceResponse>): CommandRequest<unknown, BeltResourceResponse> {
  return { path: `/api/v1/belts/${encodeURIComponent(id)}/pause`, body: { paused }, affects: libraryChanged, publish };
}

export function afkRequest(id: string, afk: AfkMode, publish: Publish<BeltResourceResponse>): CommandRequest<unknown, BeltResourceResponse> {
  return { path: `/api/v1/belts/${encodeURIComponent(id)}/afk`, body: { afk }, affects: libraryChanged, publish };
}

/** The editor's context for a draft, or for a shadow draft against its live version. A read: it sends nothing that changes the library or the draft. */
export function contextRequest(draft: Belt, stage: string | null, shadow = false) {
  return {
    path: "/api/v1/belts/context",
    body: { draft, ...(stage !== null ? { stageId: stage } : {}), ...(shadow ? { shadow } : {}) }
  };
}

/** Moves one stage to its pinned agent's latest version. The daemon keeps the stage's own overrides. */
export function updateStageRequest(draft: Belt, stageId: string, publish: Publish<BeltDraftResponse>): CommandRequest<unknown, BeltDraftResponse> {
  return { path: "/api/v1/belts/update-stage", body: { draft, stageId }, affects: draftOnly, publish };
}

export function updateAllStagesRequest(draft: Belt, publish: Publish<BeltDraftResponse>): CommandRequest<unknown, BeltDraftResponse> {
  return { path: "/api/v1/belts/update-all-stages", body: { draft }, affects: draftOnly, publish };
}

/** Saves the draft as the belt's next shadow version, starting a shadow test when none runs. */
export function shadowSaveRequest(id: string, belt: Belt, publish: Publish<BeltResourceResponse>): CommandRequest<unknown, BeltResourceResponse> {
  return { path: `/api/v1/belts/${encodeURIComponent(id)}/shadow/save`, body: { belt }, affects: libraryChanged, publish };
}

/** Accepts the shadow as the live version, or discards it; either ends the test. */
export function shadowDecisionRequest(id: string, decision: "accept" | "discard", publish: Publish<BeltResourceResponse>): CommandRequest<unknown, BeltResourceResponse> {
  return { path: `/api/v1/belts/${encodeURIComponent(id)}/shadow/${decision}`, body: {}, affects: [...libraryChanged, "runs"], publish };
}
