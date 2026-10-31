import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import type { LibraryRepoBrowser, LibraryReposResponse } from "../../api/wire.gen";
import { libraryBrowserPath, libraryReposPath, type LibraryRepoRef } from "../../api/libraryCommands";
import { useRuntime } from "../../app/runtime";
import { LibraryResourceCard } from "../../components/LibraryBrowser";
import { Segmented, Tag } from "../../components/Paper";
import { ErrorProblems } from "../../components/ProblemList";
import { LibraryAgentView } from "./LibraryAgentView";
import { LibraryBeltView } from "./LibraryBeltView";
import { LibraryRepos } from "./LibraryRepos";
import "./library.css";

type Kind = "belt" | "agent";

/**
 * The Library page: the configured repositories, then a Belts and an Agents tab that list what the shared ones hold
 * as the Belts and Agents pages list theirs. A card opens the belt or agent in its editor, read-only. The tab and the
 * open item are in the URL; the cards, counts, filters and states are the daemon's.
 */
export function LibraryPage() {
  const runtime = useRuntime();
  const [params, setParams] = useSearchParams();
  const kind: Kind = params.get("tab") === "agent" ? "agent" : "belt";
  const openID = params.get("id");
  const openRepo: LibraryRepoRef | null = openID !== null ? { repository: params.get("repository") ?? "", path: params.get("path") ?? "" } : null;
  const [search, setSearch] = useState<Record<Kind, string>>({ belt: "", agent: "" });
  const [filter, setFilter] = useState<Record<Kind, string>>({ belt: "all", agent: "all" });
  useEffect(() => { document.title = "Beltline — Library"; }, []);

  const repos = useQuery({
    queryKey: apiQueryKey(libraryReposPath),
    queryFn: ({ signal }) => apiRequest<LibraryReposResponse>(libraryReposPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(libraryReposPath) }
  });
  const browserQuery = new URLSearchParams({ kind, search: search[kind], filter: filter[kind] }).toString();
  const browser = useQuery({
    queryKey: apiQueryKey(libraryBrowserPath, browserQuery),
    queryFn: ({ signal }) => apiRequest<LibraryRepoBrowser>(`${libraryBrowserPath}?${browserQuery}`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && openRepo === null,
    retry: false,
    meta: { resourceDeps: resourcesForPath(libraryBrowserPath) }
  });

  const open = (next: Kind, repo: LibraryRepoRef, id: string) => setParams({ tab: next, repository: repo.repository, path: repo.path, id });
  const back = () => setParams({ tab: kind });

  if (openRepo && openID) return <article className="page-content library-page library-detail">
    <h1 className="screen-reader-only">Library</h1>
    {kind === "belt"
      ? <LibraryBeltView key={`${openRepo.repository}\n${openRepo.path}\n${openID}`} repo={openRepo} id={openID} onBack={back}
        onOpenAgent={agent => open("agent", openRepo, agent)} />
      : <LibraryAgentView key={`${openRepo.repository}\n${openRepo.path}\n${openID}`} repo={openRepo} id={openID} onBack={back}
        onOpenBelt={belt => open("belt", openRepo, belt)} />}
  </article>;

  const data = browser.data;
  const title = kind === "belt" ? "belts" : "agents";
  return <article className="page-content library-page">
    <header className="page-heading"><div className="work-title-block"><Tag>Workspace</Tag><h1>Library</h1></div></header>
    <p className="muted-line">Install or update the belts and agents your library repositories share, and back up your own.</p>
    {repos.isPending && <p className="loading-state" role="status">Loading library repositories…</p>}
    {repos.error && <ErrorProblems error={repos.error} />}
    {repos.data && <LibraryRepos repos={repos.data.repos} />}

    <div className="library-tabs" role="tablist" aria-label="Library contents">
      {(data?.tabs ?? [{ id: "belt", label: "Belts", count: 0, attention: false }, { id: "agent", label: "Agents", count: 0, attention: false }]).map(tab =>
        <button key={tab.id} type="button" role="tab" className="library-tab" aria-selected={tab.id === kind} onClick={() => setParams({ tab: tab.id })}>
          {tab.label}{data && <span className="nav-count">{tab.count}</span>}
          {tab.attention && <span className="library-tab-attention" title="An update or a conflict is waiting" aria-label="Needs a look" />}
        </button>)}
    </div>
    <div className="library-browser-toolbar">
      <label className="library-browser-search">
        <input className="paper-field" aria-label={`Search ${title}`} placeholder={`Search ${title}`} value={search[kind]}
          onChange={event => { const value = event.currentTarget.value; setSearch(current => ({ ...current, [kind]: value })); }} />
      </label>
      {data && <Segmented label={`${title} filter`} value={filter[kind]} onChange={value => setFilter(current => ({ ...current, [kind]: value }))}
        options={data.filters.map(option => ({ value: option.id, label: `${option.label} ${option.count}` }))} />}
    </div>
    {browser.isPending && <p className="loading-state" role="status">Loading the library…</p>}
    {browser.error && <ErrorProblems error={browser.error} />}
    {data && <section className="library-browser-results" aria-label={`Library ${title}`}>
      {data.items.length === 0 && <div className="library-browser-empty" role="status">
        <h2>{data.totalCount === 0 ? `No shared ${title} yet` : `No ${title} match`}</h2>
        <p>{data.totalCount === 0 ? "Refresh a shared repository to read what it holds." : "Try another search or choose a different filter."}</p>
      </div>}
      <div className={`library-card-grid library-card-grid--${kind}`}>
        {data.items.map(card => <LibraryResourceCard key={`${card.repository}\n${card.path}\n${card.id}`} card={card} kind={kind} source={card.detail}
          onOpen={() => open(kind, card, card.id)} />)}
      </div>
      <p className="library-browser-count">{data.matchCount} shown · {data.totalCount} in your library repositories</p>
    </section>}
  </article>;
}
