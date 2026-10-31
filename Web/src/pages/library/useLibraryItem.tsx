import { useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import type { LibraryItem } from "../../api/wire.gen";
import { libraryInstallRequest, type LibraryItemRef, type LibraryRepoRef } from "../../api/libraryCommands";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { ActionControl } from "../../components/ActionControl";
import { Chip } from "../../components/Paper";
import { ErrorProblems } from "../../components/ProblemList";
import { CompareSheet } from "./CompareSheet";

const itemStates: Record<LibraryItem["state"], { label: string; tone: "good" | "gold" | "problem" | "neutral" }> = {
  available: { label: "Available", tone: "neutral" },
  installed: { label: "Installed", tone: "good" },
  update: { label: "Update", tone: "gold" },
  conflict: { label: "Conflict", tone: "problem" }
};

/** What a library item's read-only editor shows in its header: the actions, and the notice under them. */
export interface LibraryItemControls {
  actions: ReactNode;
  notice: ReactNode;
}

/**
 * The header of a shared belt's or agent's read-only editor: its state, Open yours when the library has it, Compare,
 * and Install or Update as the daemon's action allows; under them, the reason for a conflict, the version you have
 * and the answer of an install.
 */
export function useLibraryItem(repo: LibraryRepoRef, item: LibraryItem | undefined): LibraryItemControls {
  const runtime = useRuntime();
  const command = useCommand();
  const navigate = useNavigate();
  const [comparing, setComparing] = useState(false);
  const [installed, setInstalled] = useState<string | null>(null);
  if (!item) return { actions: null, notice: null };
  const kind = item.kind as LibraryItemRef["kind"];
  const state = itemStates[item.state];
  const install = item.actions.find(action => action.id === "install");
  const label = item.state === "update" ? `Update to v${item.version}` : "Install";
  const openYours = () => navigate(kind === "belt" ? `/belts?belt=${encodeURIComponent(item.id)}`
    : `/agents?agent=${encodeURIComponent(`${item.id}@${item.localVersion ?? item.version}`)}`);
  const actions = <>
    <Chip tone={state.tone}>{state.label}</Chip>
    {item.localVersion !== null && <button type="button" className="action-button" onClick={openYours}>Open yours</button>}
    <button type="button" className="action-button" onClick={() => setComparing(true)}>Compare</button>
    {install && <ActionControl id="install" fallbackLabel={label} action={{ ...install, label }} canRead={runtime.canRead} connection={runtime.connection}
      pending={command.pending} supported kind="slate"
      onActivate={() => {
        setInstalled(null);
        const wasUpdate = item.state === "update";
        command.submit({ ...libraryInstallRequest(repo, { kind, id: item.id }),
          publish: () => setInstalled(`${wasUpdate ? "Updated to" : "Installed"} v${item.version}.`) });
      }} />}
    {comparing && <CompareSheet repo={repo} item={item} onClose={() => setComparing(false)} />}
  </>;
  const notice = <>
    {command.error ? <ErrorProblems error={command.error} /> : null}
    {installed ? <p className="library-result library-notice" role="status">{installed}{" "}
      <button type="button" className="quiet-button" onClick={openYours}>{kind === "belt" ? "Open it in Belts" : "Open it in Agents"}</button></p>
      : item.state === "conflict" && item.reason ? <p className="library-problem library-notice" role="alert">{item.reason}</p>
      : item.state === "update" && item.localVersion !== null ? <p className="library-notice library-notice--update">Yours is v{item.localVersion}.</p>
      : null}
  </>;
  return { actions, notice };
}
