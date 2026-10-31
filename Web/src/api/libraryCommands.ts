import type { EventResource, LibraryBackupResponse, LibraryRepoResponse, LibraryRestoreResponse, LibraryShareResponse } from "./wire.gen";
import type { CommandRequest } from "./useCommand";

/** The repository list: each configured repository with its items and actions. */
export const libraryReposPath = "/api/v1/library/repos";
/** One item's files: the repository's text beside the local text. */
export const libraryItemPath = "/api/v1/library/item";
/** One tab of the Library page: the belts or agents every shared repository holds, as cards. */
export const libraryBrowserPath = "/api/v1/library/browser";
/** A shared belt for the read-only belt editor, with the stage form's context for `stage`. */
export const libraryBeltPath = "/api/v1/library/belt";
/** A shared agent for the read-only agent editor. */
export const libraryAgentPath = "/api/v1/library/agent";

/** The key that tells two configured repositories apart: one repository can hold several folders. */
export const repoKey = (repo: LibraryRepoRef) => `${repo.repository}\n${repo.path}`;

/** The repository and folder a library command acts on, as the repository list names them. */
export interface LibraryRepoRef { repository: string; path: string }
/** The belt or agent a library item command acts on. */
export interface LibraryItemRef { kind: "belt" | "agent"; id: string }

/** Backups, restores and installs change the belts and agents the library read shows; the state reads them too. */
const libraryAffects: readonly EventResource[] = ["library", "state"];

/** Every library command ends with the daemon's answer; the cached reads that depend on `library` refetch after it. */
function libraryRequest<TBody, TResult>(command: string, body: TBody,
  done: (result: TResult) => void = () => undefined, affects: readonly EventResource[] = libraryAffects): CommandRequest<TBody, TResult> {
  return { path: `/api/v1/library/${command}`, body, affects, publish: (_queryClient, result) => done(result) };
}

/** Syncs one repository from its remote and returns it as the repository list shows it. */
export function libraryRefreshRequest(repo: LibraryRepoRef): CommandRequest<LibraryRepoRef, LibraryRepoResponse> {
  return libraryRequest("refresh", repo, () => undefined, ["library"]);
}

/** Copies the library into a backup repository and pushes it, or reports that nothing changed. */
export function libraryBackupRequest(repo: LibraryRepoRef, done: (result: LibraryBackupResponse) => void): CommandRequest<LibraryRepoRef, LibraryBackupResponse> {
  return libraryRequest("backup", repo, done);
}

/** Replaces the library with a backup's copy; the current definitions are kept in a dated folder. */
export function libraryRestoreRequest(repo: LibraryRepoRef, done: (result: LibraryRestoreResponse) => void): CommandRequest<LibraryRepoRef, LibraryRestoreResponse> {
  return libraryRequest("restore", repo, done);
}

/** Installs a shared item, or updates it, with the versions the repository holds. */
export function libraryInstallRequest(repo: LibraryRepoRef, item: LibraryItemRef): CommandRequest<LibraryRepoRef & LibraryItemRef, { written: string[] }> {
  return libraryRequest("install", { ...repo, ...item });
}

/** Shares a saved belt or agent to a shared repository: a pull request, or a push, as the repository's setting says. */
export function libraryShareRequest(repo: LibraryRepoRef, item: LibraryItemRef, done: (result: LibraryShareResponse) => void): CommandRequest<LibraryRepoRef & LibraryItemRef, LibraryShareResponse> {
  return libraryRequest("share", { ...repo, ...item }, done, ["library"]);
}
