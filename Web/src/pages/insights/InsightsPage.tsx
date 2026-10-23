import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { apiRequest } from "../../api/client";
import { describeError } from "../../api/errors";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { insightsFigureIDValues, insightsPeriodValues, type InsightsEvolutionView, type InsightsFigureID, type InsightsPeriod, type InsightsView } from "../../api/wire.gen";
import { useRuntime } from "../../app/runtime";
import { Caps, Segmented, Select, Tag } from "../../components/Paper";
import { BeltList, Stages } from "./InsightsSections";
import { Evolution, evolutionRequestPath, type EvolutionChoice } from "./InsightsEvolution";
import { ShadowComparison } from "./InsightsShadow";
import { ShadowDecisionSheet, type ShadowDecision } from "../belts/ShadowSheets";
import "./insights.css";

/** The labels the period control shows before the first answer names them. */
const fallbackPeriods: { value: InsightsPeriod; label: string }[] = [
  { value: "7d", label: "7 days" }, { value: "30d", label: "30 days" }, { value: "90d", label: "90 days" }, { value: "all", label: "All time" }
];

/** The URL's period, or all time when it names none the daemon knows. */
function readPeriod(raw: string | null): InsightsPeriod {
  return (insightsPeriodValues as readonly string[]).includes(raw ?? "") ? raw as InsightsPeriod : "all";
}

/** The Evolution view's choices as the URL keeps them: by=week, separate=1, from=<column>, scale=linear and figure=<id>. */
function readChoice(params: URLSearchParams): EvolutionChoice {
  const from = Number(params.get("from") ?? "");
  const figure = params.get("figure") ?? "";
  return {
    by: params.get("by") === "week" ? "week" : "version",
    separate: params.get("separate") === "1",
    from: Number.isInteger(from) && from > 0 ? from : 0,
    log: params.get("scale") !== "linear",
    figure: (insightsFigureIDValues as readonly string[]).includes(figure) ? figure as InsightsFigureID : "lead"
  };
}

/** The version picker's value for a shadow test, beside the version numbers. */
const shadowPrefix = "shadow:";

/**
 * The request for a belt, one of its versions or shadow tests, a period and the archived belts; omitted parameters
 * are the daemon's defaults (every belt, every version, all time, no archived belts).
 */
export function insightsRequestPath(belt: string, version: string, period: InsightsPeriod, showArchived = false): string {
  const params = new URLSearchParams();
  if (belt) params.set("belt", belt);
  if (belt && version.startsWith(shadowPrefix)) params.set("shadow", version.slice(shadowPrefix.length));
  else if (belt && version) params.set("version", version);
  if (period !== "all") params.set("period", period);
  if (showArchived) params.set("showArchived", "1");
  const query = params.toString();
  return `/api/v1/views/insights${query ? `?${query}` : ""}`;
}

export function InsightsPage() {
  const runtime = useRuntime();
  const [params, setParams] = useSearchParams();
  const belt = params.get("belt") ?? "";
  const version = params.get("version") ?? "";
  const period = readPeriod(params.get("period"));
  // Only showArchived=1 turns the archived belts on; the choice lives in the URL, so a link restores it.
  const showArchived = params.get("showArchived") === "1";
  // A belt opens on Evolution; view=stages shows its stages for a version and period.
  const evolution = belt !== "" && params.get("view") !== "stages";
  const choice = readChoice(params);
  const path = insightsRequestPath(belt, version, period, showArchived);
  const result = useQuery({
    queryKey: apiQueryKey(path),
    queryFn: ({ signal }) => apiRequest<InsightsView>(path, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && !evolution,
    retry: false,
    placeholderData: previous => previous,
    meta: { resourceDeps: resourcesForPath(path) }
  });
  const evolutionPath = evolutionRequestPath(belt, choice, showArchived);
  const evolutionResult = useQuery({
    queryKey: apiQueryKey(evolutionPath),
    queryFn: ({ signal }) => apiRequest<InsightsEvolutionView>(evolutionPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && evolution,
    retry: false,
    placeholderData: previous => previous,
    meta: { resourceDeps: resourcesForPath(evolutionPath) }
  });
  const [decision, setDecision] = useState<ShadowDecision | null>(null);
  useEffect(() => { document.title = "Beltline — Insights"; }, []);
  if (runtime.authRequired || runtime.incompatible) return null;

  const change = (changes: { belt?: string; version?: string; period?: InsightsPeriod; showArchived?: boolean; view?: "evolution" | "stages" }) => {
    const next = new URLSearchParams(params);
    // Another belt has other versions and columns, so choosing one starts at every version and the default split.
    if (changes.belt !== undefined) { if (changes.belt) next.set("belt", changes.belt); else next.delete("belt"); next.delete("version"); next.delete("from"); }
    if (changes.view !== undefined) { if (changes.view === "stages") next.set("view", "stages"); else next.delete("view"); }
    if (changes.version !== undefined) { if (changes.version) next.set("version", changes.version); else next.delete("version"); }
    if (changes.period !== undefined) { if (changes.period !== "all") next.set("period", changes.period); else next.delete("period"); }
    if (changes.showArchived !== undefined) { if (changes.showArchived) next.set("showArchived", "1"); else next.delete("showArchived"); }
    setParams(next);
  };
  const changeChoice = (changes: Partial<EvolutionChoice>) => {
    const next = new URLSearchParams(params);
    const set = (key: string, value: string | null) => { if (value) next.set(key, value); else next.delete(key); };
    if (changes.by !== undefined) set("by", changes.by === "week" ? "week" : null);
    if (changes.separate !== undefined) set("separate", changes.separate ? "1" : null);
    if (changes.from !== undefined) set("from", changes.from > 0 ? String(changes.from) : null);
    if (changes.log !== undefined) set("scale", changes.log ? null : "linear");
    if (changes.figure !== undefined) set("figure", changes.figure === "lead" ? null : changes.figure);
    setParams(next);
  };
  const data = evolution ? undefined : result.data;
  const evolutionData = evolution ? evolutionResult.data : undefined;
  const heading = evolution ? evolutionData : data;
  const current = evolution ? evolutionResult : result;
  const beltOptions = [{ value: "", label: "All belts" }, ...(heading?.belts ?? []).map(item => ({ value: item.id, label: item.label }))];
  const versionOptions = (data?.versions ?? []).map(item => ({
    value: item.shadow ? `${shadowPrefix}${item.shadow}` : item.value ? String(item.value) : "", label: item.label
  }));
  const shadow = data?.shadow;
  const { message } = current.error ? describeError(current.error) : { message: "" };

  return <article className="page-content insights-page">
    <header className="page-heading work-heading-row">
      <div className="work-title-block">
        <Tag>{belt ? "Insights · belt" : "Insights"}</Tag>
        <h1>{heading?.title ?? "Insights"}</h1>
      </div>
      {heading && <div className="page-heading-actions"><Caps className="work-caption">{heading.caption}</Caps></div>}
    </header>
    <div className="work-toolbar insights-toolbar">
      <div className="work-toolbar-controls">
        <Caps>Belt</Caps>
        <Select pill width={230} label="Belt" options={beltOptions} value={belt} onChange={value => change({ belt: value })} />
        <label className="insights-archive"><input type="checkbox" checked={showArchived}
          onChange={event => change({ showArchived: event.currentTarget.checked })} />Show archived</label>
        {belt && !evolution && versionOptions.length > 0 && <>
          <Caps>Version</Caps>
          <Select pill width={shadow ? 260 : 200} label="Version" options={versionOptions} value={version} onChange={value => change({ version: value })} />
        </>}
      </div>
      <div className="work-toolbar-controls">
        {/* A shadow test has its own dates: it counts the runs that started during it. */}
        {!evolution && (shadow ? <Caps>{shadow.dates}</Caps>
          : <Segmented label="Period" options={data?.periods ?? fallbackPeriods} value={period} onChange={value => change({ period: value })} />)}
        {belt && <Segmented label="View" options={[{ value: "evolution", label: "Evolution" }, { value: "stages", label: "Stages" }] as const}
          value={evolution ? "evolution" : "stages"} onChange={value => change({ view: value })} />}
      </div>
    </div>
    {!runtime.canRead && <p className="loading-state" role="status">{runtime.versionLoading ? "Checking API version…" : "Waiting for the daemon connection…"}</p>}
    {runtime.canRead && current.isPending && <p className="loading-state" role="status">Loading Insights…</p>}
    {runtime.canRead && current.error && !heading && <section className="error-panel" role="alert"><h2>Unable to load Insights</h2><p>{message}</p></section>}
    {data && !belt && <BeltList rows={data.beltRows} onOpen={id => change({ belt: id })} />}
    {data && belt && shadow && <ShadowComparison shadow={shadow} onDecide={setDecision} />}
    {data && belt && !shadow && <Stages key={belt} stages={data.stages} />}
    {evolutionData && <Evolution view={evolutionData} choice={choice} onChange={changeChoice} />}
    {decision && shadow && data && <ShadowDecisionSheet beltId={belt} beltName={data.title} shadowLabel={shadow.shadow.label} test={shadow.test}
      decision={decision} onClose={() => setDecision(null)} />}
  </article>;
}
