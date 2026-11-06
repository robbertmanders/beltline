import { Icon } from "../../components/Icon";
import { ErrorProblems } from "../../components/ProblemList";
import { desktopRequest, type DesktopCommandId, type DesktopOperands } from "../../api/runCommands";
import type { RunLink, RunWorktree } from "../../api/wire.gen";
import type { CommandController } from "../../api/useCommand";
import type { ConnectionStatus } from "../../api/eventConnection";

export interface RunLinksProps {
  runId: string;
  links: readonly RunLink[];
  worktrees: readonly RunWorktree[];
  worktreeAction: string;
  worktreeUnavailable: string;
  command: CommandController;
  canRead: boolean;
  connection: ConnectionStatus;
  /** Closes the run menu that holds these rows, once a link opens or a desktop action is accepted. */
  onDone: () => void;
}

/** Whether a refused command came from a desktop action, so its error shows here and not in the controls. */
export function isDesktopCommand(path: string | undefined): boolean {
  return path?.startsWith("/api/v1/desktop") === true;
}

/**
 * The daemon's link targets and the desktop actions that act on its Mac, as the run page's row of links. The daemon lists only the links it has a URL for. A failed launch keeps the menu open with the error
 * beside the action and never changes the selected attempt.
 */
export function RunLinkRows({ runId, links, worktrees, worktreeAction, worktreeUnavailable, command, canRead, connection, onDone }: RunLinksProps) {
  const online = canRead && connection === "connected";
  const failed = command.error !== null && isDesktopCommand(command.lastRequest?.path);
  const act = (id: DesktopCommandId, operands: DesktopOperands) => {
    command.clearError();
    command.submit({ ...desktopRequest(id, runId, operands), publish: () => onDone() });
  };
  // As in the app: the subject and pull requests, a hairline, then the worktrees, each one menu row.
  return <>
    {links.map(link => <a key={link.id} href={link.url} target="_blank" rel="noreferrer noopener"
      title={link.detail} aria-label={link.detail} onClick={onDone}><Icon name={/pull/i.test(link.title) ? "pr" : "issue"} /><span>{link.title}</span></a>)}
    {worktrees.length === 0
      ? <span className="run-worktree-empty" title={worktreeUnavailable}>
        <button type="button" disabled><Icon name="code" />{worktreeAction}</button>
        <span className="why">{worktreeUnavailable}</span>
      </span>
      : worktrees.map(worktree => <button key={worktree.id} type="button" title={worktree.detail}
        aria-label={`${worktree.title}, ${worktree.detail}`} disabled={!online || command.pending}
        onClick={() => act("vscode", { targetId: worktree.id })}><Icon name="code" /><span>{worktree.title}</span></button>)}
    {failed && <span className="run-menu-error">
      <ErrorProblems error={command.error} />
      <button type="button" className="link" onClick={command.retry}>Try again</button>
    </span>}
  </>;
}
