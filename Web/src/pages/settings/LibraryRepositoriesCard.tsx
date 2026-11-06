import { useEffect, useState } from "react";
import type { LibraryPublish, LibraryRepository, LibraryRole, SettingsResponse } from "../../api/wire.gen";
import { settingsPatchRequest } from "../../api/settingsCommands";
import { useCommand } from "../../api/useCommand";
import { ErrorProblems } from "../../components/ProblemList";
import { Select } from "../../components/Paper";
import "../library/library.css";

const roles = [{ value: "backup", label: "Backup" }, { value: "shared", label: "Shared" }] as const;
const publishes = [{ value: "pullRequest", label: "Pull request" }, { value: "push", label: "Push" }] as const;
const blankRepository: LibraryRepository = { repository: "", path: "", role: "shared", publish: "pullRequest" };

/**
 * The library repositories: one backup repository for your own belts and agents, and any number of shared ones.
 * The rows are saved together through one settings patch; the daemon checks each row.
 */
export function LibraryRepositoriesCard({ settings }: { settings: SettingsResponse }) {
  const command = useCommand();
  const [rows, setRows] = useState(settings.libraryRepositories);
  useEffect(() => setRows(settings.libraryRepositories), [settings.libraryRepositories]);
  const edit = (index: number, patch: Partial<LibraryRepository>) =>
    setRows(current => current.map((row, at) => at === index ? { ...row, ...patch } : row));
  return <section className="settings-card" aria-label="Library repositories">
    <p className="paper-tag paper-tag--strong">Library repositories</p>
    <p className="muted-line">A backup repository keeps a copy of your belts and agents. A shared repository is a team's library: install from it, and share your own to it. Each is a git repository with belts/ and agents/ folders, or the folder given as its path.</p>
    {rows.map((row, index) => <div className="library-repo-row" key={index}>
      <input className="paper-field" aria-label="Library repository" placeholder="owner/repository" value={row.repository}
        onChange={event => edit(index, { repository: event.currentTarget.value })} />
      <input className="paper-field" aria-label="Library path" placeholder="Folder (optional)" value={row.path}
        onChange={event => edit(index, { path: event.currentTarget.value })} />
      <Select label="Library role" value={row.role} options={roles} onChange={(role: LibraryRole) => edit(index, { role })} />
      {row.role === "shared" && <Select label="Share publishes as" value={row.publish} options={publishes}
        onChange={(publish: LibraryPublish) => edit(index, { publish })} />}
      <button type="button" className="btn sm" onClick={() => setRows(current => current.filter((_, at) => at !== index))}>Remove</button>
    </div>)}
    <div className="field-row">
      <button type="button" className="btn sm" onClick={() => setRows(current => [...current, blankRepository])}>Add repository</button>
      <button type="button" className="btn fill paper-button--small" disabled={command.pending}
        onClick={() => command.submit(settingsPatchRequest({ libraryRepositories: rows }))}>Save</button>
    </div>
    {command.error ? <ErrorProblems error={command.error} /> : null}
  </section>;
}
