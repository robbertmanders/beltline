import { useState } from "react";
import { Link } from "react-router-dom";
import type { LibraryAction, LibraryRepoResponse } from "../../api/wire.gen";
import { libraryBackupRequest, libraryRefreshRequest, libraryRestoreRequest, repoKey, type LibraryRepoRef } from "../../api/libraryCommands";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { ActionControl } from "../../components/ActionControl";
import { ErrorProblems } from "../../components/ProblemList";

const commandLabels: Record<string, string> = { refresh: "Refresh", backup: "Back up", restore: "Restore…" };

/** The configured library repositories, one row each with its sync state and its own commands. */
export function LibraryRepos({ repos }: { repos: readonly LibraryRepoResponse[] }) {
  if (repos.length === 0) return <p className="empty library-repos">No library repositories are set up yet.<br />
    <Link className="btn" to="/settings">Add one in Settings → Library repositories</Link></p>;
  return <div className="rows library-repos" role="group" aria-label="Library repositories">
    {repos.map(repo => <RepoRow key={repoKey(repo)} repo={repo} />)}
  </div>;
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

  return <section className="row rr library-repo" aria-label={`Library ${repo.repository}${folder}`}>
    <div><div className="t mono" style={{ fontSize: 17 }}>{repo.repository}{folder}</div>
      <div className="m"><span>{backup ? "Backups" : "Shared"}</span>
        {!backup && <span>{!repo.takesShares ? "Install only" : repo.publish === "push" ? "Share pushes" : "Share opens a pull request"}</span>}
        {backup && repo.changedSinceBackup && <span className="flag o">Changed since backup</span>}</div></div>
    <span className="c">{repo.commit ? `Synced ${repo.commit.slice(0, 7)} · ${repo.syncedAt ?? ""}` : "Not synced yet"}</span>
    <span className="c" style={{ whiteSpace: "normal" }}>{backup ? "Back up writes your belts and agents here; Restore reads them back." : "Install what you want from the tabs below."}</span>
    <div className="acts library-repo-actions">
      {repo.actions.map(action => <ActionControl key={action.id} id={action.id} fallbackLabel={commandLabels[action.id] ?? action.id} small
        action={action} canRead={runtime.canRead} connection={runtime.connection} pending={command.pending} supported kind={action.id === "backup" ? "slate" : "plain"}
        onActivate={() => action.id === "restore" ? setConfirmingRestore(true) : run(action)} />)}
    </div>
    {(repo.problem || confirmingRestore || command.error || result) && <div className="library-repo-foot" style={{ gridColumn: "1 / -1" }}>
      {repo.problem && <p className="why library-problem" role="alert">{repo.problem}</p>}
      {confirmingRestore && <div className="note confirm-row" role="alertdialog" aria-label="Confirm restore">
        <span><b>Restore the library from {repo.repository}?</b><br /><span className="help">Your belts and agents are replaced with the repository's. The current ones are kept in library-backups. Runs already going keep theirs.</span></span>
        <span className="acts">{repo.actions.filter(action => action.id === "restore").map(action => <ActionControl key="restore-confirm" id="restore" fallbackLabel="Restore"
          action={{ ...action, label: "Restore" }} canRead={runtime.canRead} connection={runtime.connection} pending={command.pending} supported kind="slate" onActivate={() => run(action)} />)}
        <button type="button" className="btn ghost" onClick={() => setConfirmingRestore(false)}>Cancel</button></span>
      </div>}
      {command.error ? <ErrorProblems error={command.error} /> : null}
      {result && <p className="help library-result" role="status">{result}</p>}
    </div>}
  </section>;
}
