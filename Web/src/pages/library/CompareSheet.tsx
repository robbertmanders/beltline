import { useQuery } from "@tanstack/react-query";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { libraryItemPath, type LibraryItemRef, type LibraryRepoRef } from "../../api/libraryCommands";
import type { LibraryItem, LibraryItemResponse } from "../../api/wire.gen";
import { useRuntime } from "../../app/runtime";
import { Sheet } from "../../components/Sheet";
import { ErrorProblems } from "../../components/ProblemList";

/** Each file of a shared item beside your own copy of it, as the daemon reads them; no diff is drawn. */
export function CompareSheet({ repo, item, onClose }: { repo: LibraryRepoRef; item: LibraryItem; onClose: () => void }) {
  const runtime = useRuntime();
  const ref: LibraryRepoRef & LibraryItemRef = { ...repo, kind: item.kind as LibraryItemRef["kind"], id: item.id };
  const query = new URLSearchParams({ repository: ref.repository, path: ref.path, kind: ref.kind, id: ref.id }).toString();
  const path = `${libraryItemPath}?${query}`;
  const files = useQuery({
    queryKey: apiQueryKey(libraryItemPath, ref),
    queryFn: ({ signal }) => apiRequest<LibraryItemResponse>(path, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(libraryItemPath) }
  });
  return <Sheet title={`Compare ${item.name}`} tag="Library" subtitle={`${repo.repository} · v${item.version}`} open onClose={onClose} width={920} className="library-compare"
    footer={<button type="button" className="btn" onClick={onClose}>Close</button>}>
    {files.isPending && <p className="loading-state" role="status">Reading the files…</p>}
    {files.error && <ErrorProblems error={files.error} />}
    {files.data?.files.map(file => <section key={file.path} className="library-compare-file">
      <span className="mono-note">{file.path}</span>
      <div className="library-compare-columns">
        <div><span className="paper-caps">In {repo.repository}</span><pre>{file.repo}</pre></div>
        <div><span className="paper-caps">Yours</span>{file.local === null ? <p className="muted-line">Not in your library yet.</p> : <pre>{file.local}</pre>}</div>
      </div>
    </section>)}
  </Sheet>;
}
