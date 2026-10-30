import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { libraryReposPath, libraryShareRequest, repoKey, type LibraryItemRef, type LibraryRepoRef } from "../../api/libraryCommands";
import type { LibraryRepoResponse, LibraryReposResponse, LibraryShareResponse } from "../../api/wire.gen";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { Select } from "../../components/Paper";
import { Sheet } from "../../components/Sheet";
import { ErrorProblems } from "../../components/ProblemList";

/** The saved belt or agent a Share sheet sends: its kind, id, version and name. */
export interface ShareSource extends LibraryItemRef { version: number; name?: string }

/**
 * Share a saved belt or agent to one of the shared repositories. The daemon writes the files, and either opens
 * a pull request or pushes, as the repository's publish setting says; the sheet only shows its answer.
 */
export function ShareSheet({ source, onClose }: { source: ShareSource; onClose: () => void }) {
  const runtime = useRuntime();
  const command = useCommand();
  const repos = useQuery({
    queryKey: apiQueryKey(libraryReposPath),
    queryFn: ({ signal }) => apiRequest<LibraryReposResponse>(libraryReposPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(libraryReposPath) }
  });
  // The daemon says which repositories take shares: never the examples repository.
  const shared = (repos.data?.repos ?? []).filter(repo => repo.takesShares);
  const [picked, setPicked] = useState("");
  const chosen: LibraryRepoResponse | undefined = shared.find(repo => repoKey(repo) === picked) ?? shared[0];
  const [shareResult, setShareResult] = useState<LibraryShareResponse | null>(null);
  const share = () => {
    if (!chosen) return;
    const repo: LibraryRepoRef = { repository: chosen.repository, path: chosen.path };
    command.submit(libraryShareRequest(repo, { kind: source.kind, id: source.id }, setShareResult));
  };
  const name = source.name?.trim() || source.id;
  const footer = <>
    <button type="button" className="paper-button" onClick={onClose}>{shareResult ? "Done" : "Cancel"}</button>
    <button type="button" className="paper-button paper-button--primary" disabled={!chosen || command.pending || shareResult !== null} onClick={share}>
      {command.pending ? "Sharing…" : "Share"}</button>
  </>;
  return <Sheet title={`Share ${name}`} tag={`Share · ${source.kind}`} subtitle={`${source.id} · v${source.version}`} open onClose={onClose} width={560} footer={footer}>
    {repos.isPending && <p className="loading-state" role="status">Loading shared repositories…</p>}
    {repos.error && <ErrorProblems error={repos.error} />}
    {repos.data && shared.length === 0 && <p className="muted-line">No shared repository is set up. Add one in Settings → Library repositories first.</p>}
    {chosen && <div className="library-share-form">
      <Select label="Shared repository" value={repoKey(chosen)} onChange={setPicked}
        options={shared.map(repo => ({ value: repoKey(repo), label: repo.path ? `${repo.repository} / ${repo.path}` : repo.repository }))} />
      <p className="muted-line">{chosen.publish === "push"
        ? `Pushes straight to the default branch of ${chosen.repository}.`
        : `Opens a pull request on ${chosen.repository} for review.`}</p>
    </div>}
    {command.error ? <ErrorProblems error={command.error} /> : null}
    {shareResult && <section role="status" className="library-result">{shareResult.changed
      ? shareResult.pullRequest
        ? <><p>Pull request opened for review.</p><a href={shareResult.pullRequest} target="_blank" rel="noreferrer">Open the pull request</a></>
        : <p>Pushed{shareResult.commit ? ` as ${shareResult.commit.slice(0, 7)}` : ""}.</p>
      : <p>Already in {chosen?.repository ?? "the repository"} with these contents.</p>}</section>}
  </Sheet>;
}
