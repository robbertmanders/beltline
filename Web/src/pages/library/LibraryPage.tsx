import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import type { LibraryAction, LibraryItem, LibraryRepoResponse, LibraryReposResponse } from "../../api/wire.gen";
import { libraryBackupRequest, libraryInstallRequest, libraryRefreshRequest, libraryReposPath, libraryRestoreRequest, repoKey, type LibraryItemRef, type LibraryRepoRef } from "../../api/libraryCommands";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { ActionControl } from "../../components/ActionControl";
import { Chip, Tag } from "../../components/Paper";
import { ErrorProblems } from "../../components/ProblemList";
import { CompareSheet } from "./CompareSheet";
import "./library.css";

const commandLabels: Record<string, string> = { refresh: "Refresh", backup: "Back up", restore: "Restore…" };
const itemStates: Record<LibraryItem["state"], { label: string; tone: "good" | "gold" | "problem" | "neutral" }> = {
  available: { label: "Available", tone: "neutral" },
  installed: { label: "Installed", tone: "good" },
  update: { label: "Update", tone: "gold" },
  conflict: { label: "Conflict", tone: "problem" }
};

/** The Library page: one section per repository in Settings, each with its own actions and items. */
export function LibraryPage() {
  const runtime = useRuntime();
  const repos = useQuery({
    queryKey: apiQueryKey(libraryReposPath),
    queryFn: ({ signal }) => apiRequest<LibraryReposResponse>(libraryReposPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(libraryReposPath) }
  });
  useEffect(() => { document.title = "Beltline — Library"; }, []);
  const list = repos.data?.repos ?? [];
  return <article className="page-content library-page">
    <header className="page-heading"><div className="work-title-block"><Tag>Workspace</Tag><h1>Library</h1></div></header>
    <p className="muted-line">Back up your belts and agents to a repository, and install or update the ones a shared repository holds.</p>
    {repos.isPending && <p className="loading-state" role="status">Loading library repositories…</p>}
    {repos.error && <ErrorProblems error={repos.error} />}
    {repos.data && list.length === 0 && <section className="resource-card">
      <p>No library repositories are set up yet.</p>
      <Link to="/settings">Add one in Settings → Library repositories</Link>
    </section>}
    {list.map(repo => <RepoSection key={repoKey(repo)} repo={repo} />)}
  </article>;
}

/** One repository: its role and sync state, its actions, and for a shared one, the items it holds. */
function RepoSection({ repo }: { repo: LibraryRepoResponse }) {
  const runtime = useRuntime();
  const command = useCommand();
  const [comparing, setComparing] = useState<LibraryItem | null>(null);
  const [confirmingRestore, setConfirmingRestore] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const ref: LibraryRepoRef = { repository: repo.repository, path: repo.path };
  const backup = repo.role === "backup";
  const folder = repo.path ? ` / ${repo.path}` : "";

  const run = (action: LibraryAction) => {
    setResult(null);
    if (action.id === "refresh") command.submit(libraryRefreshRequest(ref));
    if (action.id === "backup") command.submit(libraryBackupRequest(ref, backed => setResult(backed.changed
      ? `Backed up${backed.commit ? ` as ${backed.commit.slice(0, 7)}` : ""}.` : "Nothing changed since the last backup.")));
    if (action.id === "restore") {
      setConfirmingRestore(false);
      command.submit(libraryRestoreRequest(ref, restored => setResult(`The previous library was saved to ${restored.savedTo}.`)));
    }
  };

  return <section className="settings-card library-repo" aria-label={`Library ${repo.repository}${folder}`}>
    <div className="library-repo-head">
      <div className="library-repo-title">
        <Chip tone={backup ? "neutral" : "grey"}>{backup ? "Backup" : "Shared"}</Chip>
        <strong>{repo.repository}{folder}</strong>
        {!backup && <span className="mono-note">{!repo.takesShares ? "Install only" : repo.publish === "push" ? "Share pushes" : "Share opens a pull request"}</span>}
        {backup && repo.changedSinceBackup && <Chip tone="gold">Changed since backup</Chip>}
      </div>
      <span className="mono-note">{repo.commit ? `${repo.commit.slice(0, 7)} · ${repo.syncedAt ?? ""}` : "Not synced yet"}</span>
    </div>
    {repo.problem && <p className="library-problem" role="alert">{repo.problem}</p>}
    <div className="library-repo-actions">
      {repo.actions.map(action => <ActionControl key={action.id} id={action.id} fallbackLabel={commandLabels[action.id] ?? action.id}
        action={action} canRead={runtime.canRead} connection={runtime.connection} pending={command.pending} supported kind={action.id === "refresh" ? "plain" : "slate"}
        onActivate={() => action.id === "restore" ? setConfirmingRestore(true) : run(action)} />)}
    </div>
    {confirmingRestore && <div className="confirm-row" role="alertdialog" aria-label="Confirm restore">
      <span><strong>Restore the library from {repo.repository}?</strong> Your belts and agents are replaced with the repository's. The current ones are kept in library-backups. Runs already going keep theirs.</span>
      {repo.actions.filter(action => action.id === "restore").map(action => <ActionControl key="restore-confirm" id="restore" fallbackLabel="Restore"
        action={{ ...action, label: "Restore" }} canRead={runtime.canRead} connection={runtime.connection} pending={command.pending} supported kind="danger" onActivate={() => run(action)} />)}
      <button type="button" className="paper-button paper-button--small" onClick={() => setConfirmingRestore(false)}>Cancel</button>
    </div>}
    {command.error ? <ErrorProblems error={command.error} /> : null}
    {result && <p className="library-result" role="status">{result}</p>}
    {!backup && <ul className="library-items">
      {repo.items.length === 0 && <li className="muted-line">{repo.commit ? "This repository holds no belts or agents." : "Refresh to read this repository."}</li>}
      {repo.items.map(item => {
        const state = itemStates[item.state];
        const install = item.actions.find(action => action.id === "install");
        return <li key={`${item.kind}:${item.id}`} className="library-item" data-state={item.state}>
          <div className="library-item-text">
            <span className="library-item-name"><span className="paper-caps">{item.kind === "belt" ? "Belt" : "Agent"}</span> {item.name}</span>
            <span className="mono-note">{`${item.id} · v${item.version}${item.localVersion !== null ? ` · yours v${item.localVersion}` : ""}`}</span>
            {item.reason && <span className="muted-line">{item.reason}</span>}
          </div>
          <Chip tone={state.tone}>{state.label}</Chip>
          <button type="button" className="paper-button paper-button--small" onClick={() => setComparing(item)}>Compare</button>
          {install && <ActionControl id="install" fallbackLabel={item.state === "update" ? "Update" : "Install"} action={{ ...install, label: item.state === "update" ? "Update" : "Install" }}
            canRead={runtime.canRead} connection={runtime.connection} pending={command.pending} supported kind="slate"
            onActivate={() => command.submit(libraryInstallRequest(ref, { kind: item.kind as LibraryItemRef["kind"], id: item.id }))} />}
        </li>;
      })}
    </ul>}
    {comparing && <CompareSheet repo={ref} item={comparing} onClose={() => setComparing(null)} />}
  </section>;
}
