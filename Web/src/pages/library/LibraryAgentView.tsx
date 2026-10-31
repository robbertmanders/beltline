import { useQuery } from "@tanstack/react-query";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { libraryAgentPath, type LibraryRepoRef } from "../../api/libraryCommands";
import type { LibraryAgentDetail } from "../../api/wire.gen";
import { useRuntime } from "../../app/runtime";
import { LibraryDetailHeader } from "../../components/LibraryBrowser";
import { ErrorProblems } from "../../components/ProblemList";
import { AgentEditor } from "../AgentEditor";
import { libraryItemQuery } from "./LibraryBeltView";
import { useLibraryItem } from "./useLibraryItem";

const nothing = () => undefined;

/**
 * A shared agent in the agent editor, read-only: its settings, brief, outcomes and the repository's belts that use it,
 * without Share, Archive, Save or its folder. The agent and its presentation are the daemon's read of the repository.
 */
export function LibraryAgentView({ repo, id, onBack, onOpenBelt }: {
  repo: LibraryRepoRef;
  id: string;
  onBack: () => void;
  onOpenBelt: (id: string) => void;
}) {
  const runtime = useRuntime();
  const path = `${libraryAgentPath}?${libraryItemQuery(repo, id)}`;
  const read = useQuery({
    queryKey: apiQueryKey(libraryAgentPath, { ...repo, id }),
    queryFn: ({ signal }) => apiRequest<LibraryAgentDetail>(path, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(libraryAgentPath) }
  });
  const detail = read.data;
  const controls = useLibraryItem(repo, detail?.item);
  const source = `${repo.repository}${repo.path ? ` / ${repo.path}` : ""}`;
  return <div className="library-agent">
    <LibraryDetailHeader backLabel="Back to Library" onBack={onBack} />
    {!detail && read.isPending && <p className="loading-state" role="status">Loading the agent…</p>}
    {read.error && <ErrorProblems error={read.error} />}
    {detail && <AgentEditor agent={detail.agent} saved={null} actions={[]} presentation={detail.presentation} isNew={false} dirty={false}
      canCommand={false} commandError={null} onRetry={nothing} onChange={nothing} onChangeRunner={nothing} onOpenBelt={onOpenBelt}
      onNew={nothing} onSave={nothing} onDiscard={nothing} onArchive={nothing} onShare={nothing}
      library={{ tag: `Agent · v${detail.agent.version} · ${source}`, actions: controls.actions, notice: controls.notice }} />}
  </div>;
}
