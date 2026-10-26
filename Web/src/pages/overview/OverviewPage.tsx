import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiRequest } from "../../api/client";
import { describeError } from "../../api/errors";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import type { OverviewFloorChip, OverviewResponse, OverviewRun } from "../../api/wire.gen";
import { useRuntime } from "../../app/runtime";
import { ProblemItems } from "../../components/ProblemList";
import { usePreferences } from "../../state/usePreferences";
import { Floor } from "./Floor";
import "./overview.css";
import { beltWorkHref, hoveredAfter, stageWorkHref, resolveFocus, runHref, togglePin, validPin } from "./focus";
import { Summary } from "./Summary";
import { Caps, Segmented } from "../../components/Paper";

export const overviewApiPath = "/api/v1/views/overview";

/** The two views the page has. A fresh page load lands on Summary; the view is not kept between visits. */
export const overviewModes = [
  { value: "summary", label: "Summary" },
  { value: "floor", label: "Floor" }
] as const;
export type OverviewMode = (typeof overviewModes)[number]["value"];

/** The Overview page. The daemon sends the Summary and the Floor; this page shows them, lays the Floor out at its width, pins and dims them. */
export function OverviewPage() {
  const runtime = useRuntime();
  const navigate = useNavigate();
  const { preferences, update } = usePreferences();
  const [mode, setMode] = useState<OverviewMode>("summary");
  // Hover is transient: it lives only while the pointer is on a belt, and a view change or leaving the page clears it.
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
  const changeMode = (next: OverviewMode) => {
    setMode(next);
    setHovered(null);
  };
  const { message, problems } = result.error ? describeError(result.error) : { message: "", problems: [] as string[] };

  const openRun = (chip: OverviewFloorChip) => navigate(runHref(chip.runId, chip.openStage));
  const openRunById = (runId: string, stage?: string | null) => navigate(runHref(runId, stage));
  const hover = (belt: string, pointing: boolean) => setHovered(current => hoveredAfter(current, belt, pointing));
  const pin = (belt: string) => update({ floorPinnedBeltId: togglePin(pinned, belt) });

  return <article className="page-content overview-page">
    <header className="page-heading overview-heading">
      <h1 className="screen-reader-only">Overview</h1>
      <p className="page-title" aria-hidden="true">{data?.greeting ?? ""}</p>
      <div className="page-heading-actions">
        {data && <Caps className="overview-caption">{data.caption}</Caps>}
        <Segmented label="Overview view" options={overviewModes} value={mode} onChange={changeMode} title="Switch views" />
      </div>
    </header>
    {!runtime.canRead && <p className="loading-state" role="status">{runtime.versionLoading ? "Checking API version…" : "Waiting for the daemon connection…"}</p>}
    {runtime.canRead && result.isPending && <p className="loading-state" role="status">Loading Overview…</p>}
    {runtime.canRead && result.error && !data && <section className="error-panel" role="alert"><h2>Unable to load Overview</h2><p>{message}</p><ProblemItems problems={problems} /></section>}
    {data && <>
      <p className="loaded-state screen-reader-only" role="status">Overview loaded</p>
      {result.isError && <div className="stale-error" role="status"><p>Showing the last loaded view; refresh failed.</p><p>{message}</p><ProblemItems problems={problems} /></div>}
      <OverviewBody data={data} mode={mode} runs={runs} focus={focus} pinned={pinned} hover={hover} pin={pin}
        openRun={openRun} openRunById={openRunById} openStage={stage => navigate(stageWorkHref(stage))}
        openBelt={belt => navigate(beltWorkHref(belt))} />
    </>}
  </article>;
}

interface BodyProps {
  data: OverviewResponse;
  mode: OverviewMode;
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  pinned: string | null;
  hover: (belt: string, pointing: boolean) => void;
  pin: (belt: string) => void;
  openRun: (chip: OverviewFloorChip) => void;
  openRunById: (runId: string, stage?: string | null) => void;
  openStage: (stageId: string) => void;
  openBelt: (beltId: string) => void;
}

function OverviewBody({ data, mode, runs, focus, pinned, hover, pin, openRun, openRunById, openStage, openBelt }: BodyProps) {
  if (mode === "summary") return <Summary today={data.today} runs={runs} onOpenRun={openRunById} />;
  return <Floor floor={data.floor} runs={runs} focus={focus} pinned={pinned}
    onHover={hover} onPin={pin} onOpenRun={openRun} onOpenStage={openStage} onOpenBelt={openBelt} />;
}
