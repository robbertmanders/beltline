import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { libraryBeltPath, type LibraryRepoRef } from "../../api/libraryCommands";
import type { LibraryBeltDetail } from "../../api/wire.gen";
import { useRuntime } from "../../app/runtime";
import { LibraryDetailHeader } from "../../components/LibraryBrowser";
import { Tag } from "../../components/Paper";
import { ErrorProblems } from "../../components/ProblemList";
import { DraftEditorProvider, type DraftEditorValue } from "../belts/draftEditor";
import { locationKey } from "../belts/ProblemSummary";
import { PoliciesForm } from "../belts/PoliciesForm";
import { StageForm } from "../belts/StageForm";
import { StageLine } from "../belts/StageLine";
import { TriggerForm } from "../belts/TriggerForm";
import { useLibraryItem } from "./useLibraryItem";

/** Why nothing on a library belt can change; the read-only editor's controls are off for it. */
const readOnlyReason = "This belt is in a library repository. Install it to change it.";

/** The query string that names a library item, and the stage whose form is asked for. */
export function libraryItemQuery(repo: LibraryRepoRef, id: string, stage?: string | null): string {
  return new URLSearchParams({ repository: repo.repository, path: repo.path, id, ...(stage ? { stage } : {}) }).toString();
}

/**
 * A shared belt in the belt editor, read-only: its stage line, and the trigger and policies or the selected stage's
 * form, without anything a shared belt does not carry (repositories, base branches, pause, AFK, archive, share). The
 * belt and the editor's context, with the agents it pins, are the daemon's read of the repository.
 */
export function LibraryBeltView({ repo, id, onBack, onOpenAgent }: {
  repo: LibraryRepoRef;
  id: string;
  onBack: () => void;
  onOpenAgent: (id: string) => void;
}) {
  const runtime = useRuntime();
  const [stage, setStage] = useState<string | null>(null);
  const path = `${libraryBeltPath}?${libraryItemQuery(repo, id, stage)}`;
  const read = useQuery({
    queryKey: apiQueryKey(libraryBeltPath, { ...repo, id, stage }),
    queryFn: ({ signal }) => apiRequest<LibraryBeltDetail>(path, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    // A newly selected stage keeps the line on screen until its form's context arrives.
    placeholderData: keepPreviousData,
    meta: { resourceDeps: resourcesForPath(libraryBeltPath) }
  });
  const detail = read.data;
  const controls = useLibraryItem(repo, detail?.item);
  const source = `${repo.repository}${repo.path ? ` / ${repo.path}` : ""}`;

  const back = <LibraryDetailHeader backLabel="Back to Library" onBack={onBack} />;
  if (!detail) return <section className="belt-editor library-belt" aria-label="Library belt">
    {back}
    {read.isPending && <p className="loading-state" role="status">Loading the belt…</p>}
    {read.error && <ErrorProblems error={read.error} />}
  </section>;

  const belt = detail.belt;
  const stageById = (stageId: string) => belt.stages.find(entry => entry.id === stageId);
  const value: DraftEditorValue = {
    belt,
    blocked: readOnlyReason,
    structuralReason: readOnlyReason,
    pending: false,
    operate: () => false,
    typed: () => ({}),
    setTyped: () => undefined,
    clearTyped: () => undefined,
    addStage: async () => undefined,
    edit: () => undefined,
    stash: undefined,
    setStash: () => undefined,
    failure: null,
    clearFailure: () => undefined,
    problemTarget: (kind, targetId, field) => ({ "data-problem-target": locationKey(kind, targetId, field) }),
    selectedStageId: stage,
    selectStage: setStage,
    context: detail.context,
    routeName: route => belt.forks.flatMap(fork => fork.routes).find(item => item.id === route)?.name ?? route,
    stageById,
    readOnly: true
  };
  const selected = stage !== null ? stageById(stage) : undefined;

  return <section className="belt-editor library-belt" aria-label="Library belt" data-belt={belt.id}>
    {back}
    <header className="belt-editor-header">
      <div className="belt-editor-title">
        <Tag>{`Belt · v${belt.version} · ${source}`}</Tag>
        <h2>{belt.name}</h2>
      </div>
      <div className="belt-actions">{controls.actions}</div>
    </header>
    {controls.notice}
    <DraftEditorProvider value={value}>
      <StageLine line={detail.context.line ?? null} agents={[]} agentChoice="" onAgentChoice={() => undefined} onNeedAgents={() => undefined}
        onUpdateStage={() => undefined} updateStageEnabled={false} />
      <div className="belt-forms">
        {selected ? <StageForm key={selected.id} stage={selected} onEditAgent={agent => onOpenAgent(agent.id)}
          onUpdateStage={() => undefined} updateStageEnabled={false} />
          : <>
            {detail.context.policySection && <section className="belt-card belt-shared" aria-label="Belt"><PoliciesForm /></section>}
            <TriggerForm />
          </>}
      </div>
    </DraftEditorProvider>
  </section>;
}
