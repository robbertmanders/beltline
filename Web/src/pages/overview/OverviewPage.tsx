import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiRequest } from "../../api/client";
import { describeError } from "../../api/errors";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import type { OverviewFloorChip, OverviewResponse, OverviewRun, OverviewSummaryRow } from "../../api/wire.gen";
import { useRuntime } from "../../app/runtime";
import { useStateQuery } from "../../app/Shell";
import { ProblemItems } from "../../components/ProblemList";
import { usePreferences } from "../../state/usePreferences";
import { countWord, plural } from "../../words";
import { Floor } from "./Floor";
import "./overview.css";
import { beltWorkHref, hoveredAfter, stageWorkHref, resolveFocus, runHref, togglePin, validPin } from "./focus";
import { Today } from "./Summary";

export const overviewApiPath = "/api/v1/views/overview";

/** A daemon chip word ("DECIDE") as a button says it ("Decide"). */
export function sentence(word: string): string {
  return word.charAt(0) + word.slice(1).toLowerCase();
}

/** A run that waits for you: its title, where it is, and the decision it waits for. The whole row opens the run there. */
function NeedRow({ row, run, onOpen }: { row: OverviewSummaryRow; run: OverviewRun | undefined; onOpen: () => void }) {
  const held = row.look === "held";
  return <li className="row nr wait" data-run-id={row.runId}>
    <button type="button" className="row-hit" onClick={onOpen} aria-label={`${row.title}, ${sentence(row.chip)}`} />
    <div><div className="t">{run?.title ?? row.title}</div>
      <div className="m">{run && <><span>{run.repository} {run.ref}</span><span>{run.beltName}</span></>}<span>{run?.location.text ?? row.title}</span>{held && <span className="flag">Held</span>}</div></div>
    <div className="c">{run?.stageName ?? ""}</div>
    <div className="acts"><button type="button" className={`btn ${held ? "" : "fill"}`} tabIndex={-1} aria-hidden="true" onClick={onOpen}>{sentence(row.chip)}</button></div>
    <span className="pr"><i /></span>
  </li>;
}

/**
 * The Overview: today's figures, how many runs wait for you and which, and the floor every belt shares. The
 * daemon sends every list, count and drawing; this page words the headline from its counts, lays the floor out
 * at its width, and pins or traces a belt.
 */
export function OverviewPage() {
  const runtime = useRuntime();
  const navigate = useNavigate();
  const { preferences, update } = usePreferences();
  const state = useStateQuery().data;
  // Hover is transient: it lives only while the pointer is on a belt, and leaving the page clears it.
  const [hovered, setHovered] = useState<string | null>(null);
  const result = useQuery({
    queryKey: apiQueryKey(overviewApiPath),
    queryFn: ({ signal }) => apiRequest<OverviewResponse>(overviewApiPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(overviewApiPath) }
  });
  const data = result.data;
  const runs = useMemo(() => new Map((data?.runs ?? []).map(run => [run.id, run])), [data]);

  useEffect(() => { document.title = "Beltline — Overview"; }, []);

  if (runtime.authRequired) return null;
  if (runtime.incompatible) return null;

  const pinned = data ? validPin(preferences.floorPinnedBeltId, data.floor.belts) : null;
  const focus = resolveFocus(pinned, hovered);
  const { message, problems } = result.error ? describeError(result.error) : { message: "", problems: [] as string[] };

  const openRun = (chip: OverviewFloorChip) => navigate(runHref(chip.runId, chip.openStage));
  const openRunById = (runId: string, stage?: string | null) => navigate(runHref(runId, stage));
  const hover = (belt: string, pointing: boolean) => setHovered(current => hoveredAfter(current, belt, pointing));
  const pin = (belt: string | null) => update({ floorPinnedBeltId: belt === null ? null : togglePin(pinned, belt) });

  const needs = data?.today.needsYou;
  const count = needs?.count ?? 0;
  const running = data?.summary.running ?? 0;
  const watching = data?.summary.watching ?? 0;

  return <div className="pin calm overview-page">
    <h1 className="screen-reader-only">Overview</h1>
    {!runtime.canRead && <p className="help" role="status">{runtime.versionLoading ? "Checking API version…" : "Waiting for the daemon connection…"}</p>}
    {runtime.canRead && result.isPending && <p className="help" role="status">Loading Overview…</p>}
    {runtime.canRead && result.error && !data && <section className="note" role="alert"><div><h2 className="h3">Unable to load Overview</h2><p>{message}</p><ProblemItems problems={problems} /></div></section>}
    {data && needs && <>
      <p className="loaded-state screen-reader-only" role="status">Overview loaded</p>
      {result.isError && <div className="note" role="status"><p>Showing the last loaded view; refresh failed.</p><p>{message}</p><ProblemItems problems={problems} /></div>}
      <Today today={data.today} />
      <section className="ov1 sec" aria-label="Needs you">
        <p className="greet">{data.greeting}</p>
        <p className="hero">{countWord(count)} {plural(count, "run")} <span className="soft">{count === 1 ? "needs you." : "need you."}</span></p>
        <p className="lede">{countWord(running)} {plural(running, "agent")} at work, {countWord(watching, true)} {plural(watching, "pull request")} watched.{state?.settings.afkEnabled && <> <b>AFK is on</b>: it decides for a run that waits.</>}</p>
        <p className="screen-reader-only">{data.caption}</p>
        {count === 0 ? <p className="empty">{needs.empty}</p> : <ol className="rows">
          {needs.rows.map(row => <NeedRow key={row.runId} row={row} run={runs.get(row.runId)} onOpen={() => openRunById(row.runId, row.openStage)} />)}
        </ol>}
      </section>
      <Floor floor={data.floor} runs={runs} focus={focus} pinned={pinned}
        onHover={hover} onPin={pin} onOpenRun={openRun} onOpenStage={stage => navigate(stageWorkHref(stage))}
        onOpenBelt={belt => navigate(beltWorkHref(belt))} />
    </>}
  </div>;
}
