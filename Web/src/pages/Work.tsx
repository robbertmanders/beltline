import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { apiRequest } from "../api/client";
import { describeError } from "../api/errors";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import type { WorkGroupPresentation, WorkResponse, WorkRowPresentation } from "../api/wire.gen";
import { useRuntime } from "../app/runtime";
import { Icon } from "../components/Icon";
import { ProblemItems } from "../components/ProblemList";
import { usePreferences } from "../state/usePreferences";
import { countWord, plural } from "../words";
import { useChipIndicator } from "./overview/Floor";
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
  { value: "none", label: "Nothing" },
  { value: "repo", label: "Repository" },
  { value: "status", label: "Status" },
  { value: "belt", label: "Belt" },
  { value: "epic", label: "Epic" }
] as const;

/** A run's page, opened at the decision stage for a row that needs a person. The run link follows the run otherwise. */
export function workRunHref(row: WorkRowPresentation): string {
  const run = `/runs/${encodeURIComponent(row.run.id)}`;
  return row.openStage ? `${run}?stage=${encodeURIComponent(row.openStage)}` : run;
}

/** One run: its title over its repository, number and belt, where it is, what it waits for, and when it last moved; a hairline under it fills as it goes. */
function RunRow({ row }: { row: WorkRowPresentation }) {
  const held = row.chip === "Answer";
  const pauseWord = row.pauseWord && !row.location.text.includes(row.pauseWord) ? row.pauseWord : null;
  const href = workRunHref(row);
  const active = row.run.state !== "done" && row.run.state !== "parked";
  const tone = row.needsYou ? (held ? "held" : "needs") : "passive";
  return <li className={`row wr work-row work-row-${tone} ${row.needsYou ? "wait" : ""}`} data-run-id={row.run.id}>
    <Link className="row-hit" to={href} aria-label={`${row.reference} ${row.run.issue.title}, ${row.location.text}`} />
    <div><div className="t work-title">{row.run.issue.title}</div>
      <div className="m"><span>{row.repository} {row.reference}</span><span>{row.run.beltName}</span>
        {row.shadow && <span className="flag f work-shadow-chip">{row.shadow}</span>}</div></div>
    <div className="c at work-location" style={{ whiteSpace: "normal" }}>{row.location.text}{pauseWord && <> {pauseWord}</>}</div>
    <div className="c">{row.chip && <span className={`flag ${held ? "o" : ""} work-chip`}>{row.chip}</span>}</div>
    <div className="c r work-updated"><time dateTime={row.run.updatedAt}>{row.updatedAgo}</time></div>
    {active && <span className="pr"><i style={{ "--p": row.progress } as React.CSSProperties} /></span>}
  </li>;
}

interface RunGroupsProps {
  label: string;
  groups: readonly WorkGroupPresentation[];
  groupBy: string;
  folded: ReadonlySet<string>;
  onToggle: (value: string) => void;
  /** The copy a section with no row shows. */
  empty?: string;
}

/** A section's runs. Grouped, each daemon group keeps its heading and count, and folding hides only its rows. */
function RunGroups({ label, groups, groupBy, folded, onToggle, empty }: RunGroupsProps) {
  const grouped = groupBy !== "none";
  return <div className="work-groups" role="region" aria-label={label}>
    {empty !== undefined && <p className="empty empty-state" role="status">{empty}</p>}
    {groups.map(group => {
      const isFolded = grouped && folded.has(group.value);
      return <div key={`${group.dimension}:${group.value}`} className="grp" data-group={group.value}>
        {grouped && <h3 className="gh work-fold-heading"><button type="button" className="work-fold" aria-expanded={!isFolded} onClick={() => onToggle(group.value)}>
          <span className="gn">{group.count}</span>{group.label}<Icon name={isFolded ? "open" : "dn"} className="work-fold-chevron" />
        </button></h3>}
        {!isFolded && <ol className="rows" aria-label={grouped ? group.label : label}>{group.rows.map(row => <RunRow key={row.run.id} row={row} />)}</ol>}
      </div>;
    })}
  </div>;
}

/** The search over runs. Typing replaces the history entry the previous keystroke made; Escape clears it. */
function SearchField({ value, onChange }: { value: string; onChange: (value: string, typing: boolean) => void }) {
  const [text, setText] = useState(value);
  useEffect(() => { setText(value); }, [value]);
  return <div className="work-search">
    <label className="vh" htmlFor="work-search">Search issues, runs and belts</label>
    <input className="inp" id="work-search" type="search" value={text} placeholder="Search by title, repository or number" autoComplete="off"
      onChange={event => { setText(event.currentTarget.value); onChange(event.currentTarget.value, true); }}
      onKeyDown={event => { if (event.key === "Escape") { setText(""); onChange("", false); } }} />
  </div>;
}

/** The set with value added, or removed when it was there. */
function toggleSet(set: ReadonlySet<string>, value: string): ReadonlySet<string> {
  const next = new Set(set);
  if (!next.delete(value)) next.add(value);
  return next;
}

/**
 * Work: every run on the belts, as the daemon filters, groups and orders them for the query in the URL. The page
 * words the headline from the daemon's counts and keeps the folds as a display preference.
 */
export function WorkPage() {
  const runtime = useRuntime();
  const location = useLocation();
  const navigate = useNavigate();
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
    placeholderData: previous => previous,
    meta: { resourceDeps: resourcesForPath(path) }
  });
  const shown: WorkUrlQuery = { ...query, filter: query.filter || "active", belt: query.belt || "all", groupBy: query.groupBy || "none" };
  const { box, line } = useChipIndicator(shown.belt);

  useEffect(() => { document.title = "Beltline — Work"; }, []);

  if (runtime.authRequired) return null;
  if (runtime.incompatible) return null;

  // Typing a search replaces the entry the previous keystroke made; any other change adds one, so Back returns to it.
  const change = (changes: Partial<WorkUrlQuery>, searchEdit = false) => {
    const replace = searchEdit && location.state?.workSearch === true;
    setParams(withWorkQuery(params, changes), { replace, state: { workSearch: searchEdit } });
  };

  const data = result.data;
  const main = data?.mainGroups ?? [];
  const watching = data?.watchingGroups ?? [];
  const shadow = data?.shadowGroups ?? [];
  const runsFolded = foldedValues(preferences.runsFolded, shown.groupBy);
  const watchingFolded = foldedValues(preferences.watchingFolded, shown.groupBy);
  const { message, problems } = result.error ? describeError(result.error) : { message: "", problems: [] as string[] };
  const active = data?.summary.active ?? 0;
  const needs = data?.summary.needsYou ?? 0;
  const belts = (data?.belts ?? []).filter(belt => !belt.archived || belt.id === shown.belt);

  return <div className="pin calm work-page">
    <header className="ph">
      {data?.stage ? <h1 className="h1" aria-label={data.stage.name}>{data.stage.name}<br /><span className="soft">{countWord(data.shownRows)} {plural(data.shownRows, "run")} at this stage.</span></h1>
        : <h1 className="h1" aria-label="Work">{countWord(active)} {plural(active, "run")} on the belts.<br /><span className="soft">{needs ? `${countWord(needs)} ${needs === 1 ? "needs" : "need"} you.` : "None need you."}</span></h1>}
      {data && <p className="help work-caption">{data.caption}</p>}
      <div className="ph-row" style={{ gap: "18px 28px" }}>
        <SearchField value={query.search} onChange={(value, typing) => change({ search: value }, typing)} />
        <div className="seg" role="group" aria-label="Show">
          {filters.map(item => <button key={item.value} type="button" aria-pressed={shown.filter === item.value} onClick={() => change({ filter: item.value })}>
            {item.value === "you" && data ? `${item.label} ${data.summary.needsYou}` : item.label}</button>)}
        </div>
        <label className="work-group-by"><span className="lab">Group by</span>
          <select className="sel inline" aria-label="Group by" value={shown.groupBy} onChange={event => change({ groupBy: event.currentTarget.value })}>
            {groupings.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
          </select>
        </label>
      </div>
      <div className="chips" style={{ marginTop: 18 }} ref={box} role="group" aria-label="Belt">
        <span className="cind" ref={line} aria-hidden="true" />
        <button type="button" className={`chip ${shown.belt === "all" ? "on" : ""}`} aria-pressed={shown.belt === "all"} onClick={() => change({ belt: "all" })}>All belts</button>
        {belts.map(belt => <button key={belt.id} type="button" className={`chip ${shown.belt === belt.id ? "on" : ""}`} aria-pressed={shown.belt === belt.id}
          onClick={() => change({ belt: belt.id })}>{belt.label}</button>)}
        {data?.stage && <button type="button" className="chip on" data-stage onClick={() => change({ stage: "" })}>Clear stage <Icon name="x" /></button>}
      </div>
    </header>
    {!runtime.canRead && <p className="help" role="status">{runtime.versionLoading ? "Checking API version…" : "Waiting for the daemon connection…"}</p>}
    {runtime.canRead && result.isPending && <p className="help" role="status">Loading Work…</p>}
    {runtime.canRead && result.error && !data && <section className="note error-panel" role="alert"><div><h2 className="h3">Unable to load Work</h2><p>{message}</p><ProblemItems problems={problems} /></div></section>}
    {data && <>
      {result.isError && <div className="note" role="status"><p>Showing the last loaded results; refresh failed.</p><p>{message}</p><ProblemItems problems={problems} /></div>}
      <p className="loaded-state screen-reader-only" role="status">Showing {data.shownRows} runs</p>
      <section className="grp work-section" aria-labelledby="work-runs-heading">
        <h2 id="work-runs-heading" className="gh"><span className="gn">{data.shownRows}</span>Runs</h2>
        <RunGroups label="Runs" groups={main} groupBy={shown.groupBy} folded={runsFolded} empty={data.mainEmptyMessage}
          onToggle={value => update({ runsFolded: toggleFolded(preferences.runsFolded, shown.groupBy, value) })} />
      </section>
      {data.waiting.length > 0 && <section className="grp work-section" aria-labelledby="work-waiting-heading">
        <h2 id="work-waiting-heading" className="gh"><span className="gn">{data.waiting.length}</span>Waiting to start</h2>
        <ul className="rows" aria-label="Waiting to start">
          {data.waiting.map(item => <li key={`${item.beltId}:${item.reference}`} className="row off work-waiting-row" style={{ gridTemplateColumns: "minmax(0,1fr) auto" }}>
            <div><div className="t">{item.subject.title}</div><div className="m"><span>{item.reference}</span></div></div><span className="c r">{item.reason}</span>
          </li>)}
        </ul>
      </section>}
      {shadow.length > 0 && <section className="grp work-section" aria-labelledby="work-shadow-heading">
        <h2 id="work-shadow-heading" className="gh"><span className="gn">{data.shadowRows}</span>Shadow</h2>
        <p className="help work-shadow-note">Runs on a shadow version. They are not on the Overview floor; any that need you still count in Needs you.</p>
        <RunGroups label="Shadow" groups={shadow} groupBy={shown.groupBy} folded={shadowFolded}
          onToggle={value => setShadowFolded(current => toggleSet(current, value))} />
      </section>}
      {watching.length > 0 && <section className="grp work-section" aria-labelledby="work-watching-heading">
        <h2 id="work-watching-heading" className="gh"><span className="gn">{data.watchingRows}</span>Watching</h2>
        <RunGroups label="Watching" groups={watching} groupBy={shown.groupBy} folded={watchingFolded}
          onToggle={value => update({ watchingFolded: toggleFolded(preferences.watchingFolded, shown.groupBy, value) })} />
      </section>}
      {data.shownRows === 0 && data.watchingRows === 0 && (query.search || shown.filter !== "active" || shown.belt !== "all") &&
        <p className="empty">Nothing here{query.search ? ` for “${query.search}”` : ""}.<br /><button type="button" className="btn" onClick={() => navigate("/work")}>Clear the filters</button></p>}
    </>}
  </div>;
}
