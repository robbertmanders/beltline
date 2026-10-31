import { useState } from "react";
import { Link } from "react-router-dom";
import type { LibraryAction, LibraryRepoResponse } from "../../api/wire.gen";
import { libraryBackupRequest, libraryRefreshRequest, libraryRestoreRequest, repoKey, type LibraryRepoRef } from "../../api/libraryCommands";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { ActionControl } from "../../components/ActionControl";
import { Chip } from "../../components/Paper";
import { ErrorProblems } from "../../components/ProblemList";

const commandLabels: Record<string, string> = { refresh: "Refresh", backup: "Back up", restore: "Restore…" };

/** The configured library repositories, one row each with its sync state and its own commands. */
export function LibraryRepos({ repos }: { repos: readonly LibraryRepoResponse[] }) {
  if (repos.length === 0) return <section className="settings-card library-repos">
    <p className="muted-line">No library repositories are set up yet.</p>
    <Link to="/settings">Add one in Settings → Library repositories</Link>
  </section>;
  return <section className="settings-card library-repos" aria-label="Library repositories">
    {repos.map(repo => <RepoRow key={repoKey(repo)} repo={repo} />)}
  </section>;
}

/** One repository: its role and sync state, its actions, and the answer of the last one. */
function RepoRow({ repo }: { repo: LibraryRepoResponse }) {
  const runtime = useRuntime();
  const command = useCommand();
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

  return <section className="library-repo" aria-label={`Library ${repo.repository}${folder}`}>
    <div className="library-repo-head">
      <div className="library-repo-title">
        <Chip tone={backup ? "neutral" : "grey"}>{backup ? "Backup" : "Shared"}</Chip>
        <strong>{repo.repository}{folder}</strong>
        {!backup && <span className="mono-note">{!repo.takesShares ? "Install only" : repo.publish === "push" ? "Share pushes" : "Share opens a pull request"}</span>}
        {backup && repo.changedSinceBackup && <Chip tone="gold">Changed since backup</Chip>}
      </div>
      <span className="mono-note">{repo.commit ? `${repo.commit.slice(0, 7)} · ${repo.syncedAt ?? ""}` : "Not synced yet"}</span>
      <div className="library-repo-actions">
        {repo.actions.map(action => <ActionControl key={action.id} id={action.id} fallbackLabel={commandLabels[action.id] ?? action.id}
          action={action} canRead={runtime.canRead} connection={runtime.connection} pending={command.pending} supported kind={action.id === "refresh" ? "plain" : "slate"}
          onActivate={() => action.id === "restore" ? setConfirmingRestore(true) : run(action)} />)}
      </div>
    </div>
    {repo.problem && <p className="library-problem" role="alert">{repo.problem}</p>}
    {confirmingRestore && <div className="confirm-row" role="alertdialog" aria-label="Confirm restore">
      <span><strong>Restore the library from {repo.repository}?</strong> Your belts and agents are replaced with the repository's. The current ones are kept in library-backups. Runs already going keep theirs.</span>
      {repo.actions.filter(action => action.id === "restore").map(action => <ActionControl key="restore-confirm" id="restore" fallbackLabel="Restore"
        action={{ ...action, label: "Restore" }} canRead={runtime.canRead} connection={runtime.connection} pending={command.pending} supported kind="danger" onActivate={() => run(action)} />)}
      <button type="button" className="paper-button paper-button--small" onClick={() => setConfirmingRestore(false)}>Cancel</button>
    </div>}
    {command.error ? <ErrorProblems error={command.error} /> : null}
    {result && <p className="library-result" role="status">{result}</p>}
  </section>;
}
