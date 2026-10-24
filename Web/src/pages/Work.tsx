import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { apiRequest } from "../api/client";
import { describeError } from "../api/errors";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import type { WorkGroupPresentation, WorkResponse, WorkRowPresentation } from "../api/wire.gen";
import { useRuntime } from "../app/runtime";
import { ProblemItems } from "../components/ProblemList";
import { Caps, Chip, NeedChip, Segmented, Select, Tag } from "../components/Paper";
import { usePreferences } from "../state/usePreferences";
import "./work.css";
import { foldedValues, readWorkQuery, toggleFolded, withWorkQuery, workRequestPath, type WorkUrlQuery } from "./workQuery";

const filters = [
  { value: "active", label: "Active" },
  { value: "you", label: "Needs you" },
  { value: "parked", label: "Parked" },
  { value: "shipped", label: "Shipped" },
  { value: "all", label: "All" }
] as const;

const groupings = [
  { value: "none", label: "None" },
  { value: "repo", label: "Repository" },
  { value: "status", label: "Status" },
  { value: "belt", label: "Belt" },
  { value: "epic", label: "Epic" }
] as const;

const columns = ["Issue", "Repository", "Belt", "Stage", "Updated"] as const;

/** A run's page, opened at the decision stage for a row that needs a person. The run link follows the run otherwise. */
export function workRunHref(row: WorkRowPresentation): string {
  const run = `/runs/${encodeURIComponent(row.run.id)}`;
  return row.openStage ? `${run}?stage=${encodeURIComponent(row.openStage)}` : run;
}

function RunRow({ row }: { row: WorkRowPresentation }) {
  const navigate = useNavigate();
  const held = row.chip === "Answer";
  const tone = row.needsYou ? (held ? "held" : "needs") : "passive";
  const pauseWord = row.pauseWord && !row.location.text.includes(row.pauseWord) ? row.pauseWord : null;
  const href = workRunHref(row);
  return <tr className={`work-row work-row-${tone}`} data-run-id={row.run.id} onClick={event => {
    if ((event.target as HTMLElement).closest("a")) return;
    navigate(href);
  }}>
    <td className="work-issue">
      <Link to={href} aria-label={`${row.reference} ${row.run.issue.title}, ${row.location.text}`}>
        <span className="issue-number">{row.reference}</span><span className="work-title">{row.run.issue.title}</span>
        {row.shadow && <Chip tone="shadow" className="work-shadow-chip">{row.shadow}</Chip>}
      </Link>
    </td>
    <td className="work-repository">{row.repository}</td>
    <td className="work-belt">{row.run.beltName}</td>
    <td className="work-stage"><span className="work-stage-line">
      <span className={`work-progress work-progress-${row.run.state}`} aria-hidden="true"><span style={{ width: `${Math.round(row.progress * 100)}%` }} /></span>
      <span className={`work-location overview-look-${row.location.look}`}>{row.location.text}{pauseWord && <> {pauseWord}</>}</span>
      {row.chip && <NeedChip held={held}>{row.chip}</NeedChip>}
    </span></td>
    <td className="work-updated"><time dateTime={row.run.updatedAt}>{row.updatedAgo}</time></td>
  </tr>;
}

interface RunTableProps {
  label: string;
  groups: readonly WorkGroupPresentation[];
  groupBy: string;
  folded: ReadonlySet<string>;
  onToggle: (value: string) => void;
  /** The copy a section with no row shows in its own table, below the column headings. */
  empty?: string;
}

/** One table for a section. Each daemon group keeps its own header and count; folding hides only its rows. */
function RunTable({ label, groups, groupBy, folded, onToggle, empty }: RunTableProps) {
  return <table className="work-table" aria-label={label}>
    <colgroup><col /><col style={{ width: 144 }} /><col style={{ width: 185 }} /><col style={{ width: 380 }} /><col style={{ width: 104 }} /></colgroup>
    <thead><tr>{columns.map(name => <th key={name} scope="col"><Caps>{name}</Caps></th>)}</tr></thead>
    {empty !== undefined && <tbody><tr><td className="empty-state" colSpan={columns.length} role="status">{empty}</td></tr></tbody>}
    {groups.map(group => {
      const grouped = groupBy !== "none";
      const isFolded = grouped && folded.has(group.value);
      return <tbody key={`${group.dimension}:${group.value}`} data-group={group.value}>
        {grouped && <tr className="work-group-heading">
          <th scope="colgroup" colSpan={columns.length}>
            <button type="button" className="work-fold" aria-expanded={!isFolded} onClick={() => onToggle(group.value)}>
              <svg className="work-fold-chevron" width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d={isFolded ? "M3.5 2L6.5 5L3.5 8" : "M2 3.5L5 6.5L8 3.5"} fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" /></svg><Caps className="work-fold-label">{group.label}</Caps> <Caps>· {group.count}</Caps>
            </button>
          </th>
        </tr>}
        {!isFolded && group.rows.map(row => <RunRow key={row.run.id} row={row} />)}
      </tbody>;
    })}
  </table>;
}

function Toolbar({ query, data, onChange }: {
  query: WorkUrlQuery;
  data: WorkResponse | undefined;
  onChange: (changes: Partial<WorkUrlQuery>) => void;
}) {
  const filter = (query.filter || "active") as (typeof filters)[number]["value"];
  const belts = data?.belts ?? [];
  const beltOptions = [{ value: "all", label: "All belts" }, ...belts.map(belt => ({ value: belt.id, label: belt.label }))];
  const filterOptions = filters.map(item => ({ value: item.value, label: item.value === "you" && data ? `${item.label} ${data.summary.needsYou}` : item.label }));
  return <div className="work-toolbar">
    <h2 className="paper-tag paper-tag--strong work-runs-tag">Runs</h2>
    <div className="work-toolbar-controls">
      <Caps>Group by</Caps>
      <Select pill width={130} label="Group by" options={groupings} value={query.groupBy as (typeof groupings)[number]["value"]} onChange={groupBy => onChange({ groupBy })} />
      <Caps>Belt</Caps>
      <Select pill width={165} label="Belt" options={beltOptions} value={query.belt} onChange={belt => onChange({ belt })} />
      <Segmented label="Show" options={filterOptions} value={filter} onChange={value => onChange({ filter: value })} />
    </div>
  </div>;
}

/** The set with value added, or removed when it was there. */
function toggleSet(set: ReadonlySet<string>, value: string): ReadonlySet<string> {
  const next = new Set(set);
  if (!next.delete(value)) next.add(value);
  return next;
}

export function WorkPage() {
  const runtime = useRuntime();
  const location = useLocation();
  const [params, setParams] = useSearchParams();
  const { preferences, update } = usePreferences();
  // The Shadow section's folds last for the visit only; they are not kept as a preference.
  const [shadowFolded, setShadowFolded] = useState<ReadonlySet<string>>(new Set());
  const query = readWorkQuery(params);
  const path = workRequestPath(query);
  const result = useQuery({
    queryKey: apiQueryKey(path),
    queryFn: ({ signal }) => apiRequest<WorkResponse>(path, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(path) }
  });

  useEffect(() => { document.title = "Beltline — Work"; }, []);

  if (runtime.authRequired) return null;
  if (runtime.incompatible) return null;

  // Typing a search replaces the entry the previous keystroke made; any other change adds one, so Back returns to it.
  const change = (changes: Partial<WorkUrlQuery>, searchEdit = false) => {
    const replace = searchEdit && location.state?.workSearch === true;
    setParams(withWorkQuery(params, changes), { replace, state: { workSearch: searchEdit } });
  };

  // An omitted parameter is shown at its default; the request still omits it.
  const shown: WorkUrlQuery = { ...query, filter: query.filter || "active", belt: query.belt || "all", groupBy: query.groupBy || "none" };
  const data = result.data;
  const main = data?.mainGroups ?? [];
  const watching = data?.watchingGroups ?? [];
  const shadow = data?.shadowGroups ?? [];
  const runsFolded = foldedValues(preferences.runsFolded, shown.groupBy);
  const watchingFolded = foldedValues(preferences.watchingFolded, shown.groupBy);
  const scope = query.search.trim() ? `Search · ${query.search}` : "All belts";
  const { message, problems } = result.error ? describeError(result.error) : { message: "", problems: [] as string[] };

  return <article className="page-content work-page">
    <header className="page-heading work-heading-row">
      <div className="work-title-block">
        <Tag>{scope}</Tag>
        <h1>{data?.stage?.name ?? "Work"}</h1>
      </div>
      {data && <div className="page-heading-actions">
        {data.stage && <button type="button" className="quiet-button" onClick={() => change({ stage: "" })}>Clear stage</button>}
        <Caps className="work-caption">{data.caption}</Caps>
      </div>}
    </header>
    <section className="work-section" aria-label="Work query">
      <Toolbar query={shown} data={data} onChange={changes => change(changes)} />
    </section>
    {!runtime.canRead && <p className="loading-state" role="status">{runtime.versionLoading ? "Checking API version…" : "Waiting for the daemon connection…"}</p>}
    {runtime.canRead && result.isPending && <p className="loading-state" role="status">Loading Work…</p>}
    {runtime.canRead && result.error && !data && <section className="error-panel" role="alert"><h2>Unable to load Work</h2><p>{message}</p><ProblemItems problems={problems} /></section>}
    {data && <>
      {result.isError && <div className="stale-error" role="status"><p>Showing the last loaded results; refresh failed.</p><p>{message}</p><ProblemItems problems={problems} /></div>}
      <p className="loaded-state screen-reader-only" role="status">Showing {data.shownRows} runs</p>
      <section className="work-section" aria-label="Runs">
        <RunTable label="Runs" groups={main} groupBy={shown.groupBy} folded={runsFolded} empty={data.mainEmptyMessage}
          onToggle={value => update({ runsFolded: toggleFolded(preferences.runsFolded, shown.groupBy, value) })} />
      </section>
      {data.waiting.length > 0 && <section className="work-section" aria-labelledby="work-waiting-heading">
        <h2 id="work-waiting-heading" className="paper-tag paper-tag--strong work-heading">Waiting to start</h2>
        <ul className="work-waiting" aria-label="Waiting to start">
          {data.waiting.map(item => <li key={`${item.beltId}:${item.reference}`} className="work-waiting-row">
            <span className="work-waiting-ref">{item.reference}</span><span className="work-title">{item.subject.title}</span><span className="work-reason">{item.reason}</span>
          </li>)}
        </ul>
      </section>}
      {shadow.length > 0 && <section className="work-section" aria-labelledby="work-shadow-heading">
        <h2 id="work-shadow-heading" className="paper-tag paper-tag--strong work-heading work-shadow-heading">Shadow · {data.shadowRows}</h2>
        <p className="field-note work-shadow-note">Runs on a shadow version. They are not on the Overview floor; any that need you still count in Needs you.</p>
        <RunTable label="Shadow" groups={shadow} groupBy={shown.groupBy} folded={shadowFolded}
          onToggle={value => setShadowFolded(current => toggleSet(current, value))} />
      </section>}
      {watching.length > 0 && <section className="work-section" aria-labelledby="work-watching-heading">
        <h2 id="work-watching-heading" className="paper-tag paper-tag--strong work-heading">Watching · {data.watchingRows}</h2>
        <RunTable label="Watching" groups={watching} groupBy={shown.groupBy} folded={watchingFolded}
          onToggle={value => update({ watchingFolded: toggleFolded(preferences.watchingFolded, shown.groupBy, value) })} />
      </section>}
    </>}
  </article>;
}
