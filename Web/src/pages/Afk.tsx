import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiRequest } from "../api/client";
import { describeError } from "../api/errors";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import { backendValues } from "../api/wire.gen";
import type { AfkDecision, AfkDecisionsResponse, AfkSettings, ModelChoice, ModelRunnerResource, SettingsResponse, StateResponse } from "../api/wire.gen";

/** The decisions list's filter: the ones waiting for review, or all of them. */
type AfkFilter = "review" | "all";
import { settingsPatchRequest } from "../api/settingsCommands";

import { useCommand } from "../api/useCommand";
import { ErrorProblems } from "../components/ProblemList";
import { backendLabels } from "../components/backendLabels";
import { ModelPicker, type ModelPickerState } from "../components/ModelPicker";
import { useRuntime } from "../app/runtime";
import { formatAgo } from "../format";
import { Icon } from "../components/Icon";
import { GlobalControl } from "../app/Shell";
import { afkReviewRequest } from "../api/afkCommands";
import { desktopRequest } from "../api/runCommands";
import { OverruleSheet } from "./AfkOverrule";

/** An effort's name as the app wrote it: Low, Medium, High, Extra high, Max. */
function thinkingLabel(thinking: string): string {
  return thinking === "xhigh" ? "Extra high" : thinking.charAt(0).toUpperCase() + thinking.slice(1);
}

const thinkingOptions = ["low", "medium", "high", "xhigh", "max"] as const;

function useAfkQueries(filter: AfkFilter) {
  const runtime = useRuntime();
  const decisionsPath = `/api/v1/afk/decisions?filter=${filter}`;
  const decisions = useQuery({
    queryKey: apiQueryKey("/api/v1/afk/decisions", filter),
    queryFn: ({ signal }) => apiRequest<AfkDecisionsResponse>(decisionsPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/afk/decisions") }
  });
  const settings = useQuery({
    queryKey: apiQueryKey("/api/v1/settings"),
    queryFn: ({ signal }) => apiRequest<SettingsResponse>("/api/v1/settings", { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/settings") }
  });
  const state = useQuery({
    queryKey: apiQueryKey("/api/v1/state"),
    queryFn: ({ signal }) => apiRequest<StateResponse>("/api/v1/state", { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/state") }
  });
  return { decisions, settings, state };
}

/** The five allowances, the per-stage limit, guidance and the runner/model/thinking selection. */
function Allowances({ settings }: { settings: AfkSettings }) {
  const runtime = useRuntime();
  const command = useCommand();
  const [guidance, setGuidance] = useState(settings.guidance);
  const [guidanceDirty, setGuidanceDirty] = useState(false);
  useEffect(() => {
    if (!guidanceDirty || guidance === settings.guidance) {
      setGuidance(settings.guidance);
      setGuidanceDirty(false);
    }
  }, [guidance, guidanceDirty, settings.guidance]);

  const models = useQuery({
    queryKey: apiQueryKey(`/api/v1/models/${settings.backend}`),
    queryFn: ({ signal }) => apiRequest<ModelRunnerResource>(`/api/v1/models/${settings.backend}`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/models/:runner") }
  });

  const patch = (afk: Record<string, unknown>) => command.submit(settingsPatchRequest({ afk }));
  const error = command.error && !(command.error instanceof Error && command.error.name === "SignInRequiredError") ? command.error : null;
  const thinking = settings.thinking ?? "";
  const choices: readonly ModelChoice[] = models.data?.choices ?? [];
  const modelState: ModelPickerState = models.isPending && !models.data ? "loading" : models.error && !models.data ? "failed" : "ready";

  const toggle = (key: keyof AfkSettings, label: string) => <label className="tg"><input type="checkbox" checked={Boolean(settings[key])}
    onChange={event => patch({ [key]: event.currentTarget.checked })} /><span className="t" aria-hidden="true" />{label}</label>;

  return <section className="ed afk-allowances-section" aria-label="What it may do">
    <div className="lab">It may<small>It can always leave a decision for you, and never touches a run you stopped yourself.</small></div>
    <div className="tgl afk-allowances">
      {toggle("answerQuestions", "Answer an agent’s open questions")}
      {toggle("rerunFailures", "Run failed stages again")}
      {toggle("approve", "Decide at approval stages")}
      {toggle("resolveHolds", "Carry on after other holds")}
      {toggle("drop", "Drop runs")}
    </div>
    <hr />
    <div className="lab">Decisions per stage<small>Per stage of one run. Past that it leaves the run for you, so it never goes round in circles.</small></div>
    <div className="afk-limit-row" role="group" aria-label="Decisions per stage">
      <div className="stepper">
        <button type="button" aria-label="Fewer decisions per stage" disabled={settings.limit <= 1} onClick={() => patch({ limit: settings.limit - 1 })}><Icon name="minus" /></button>
        <span>{settings.limit}</span>
        <button type="button" aria-label="More decisions per stage" disabled={settings.limit >= 10} onClick={() => patch({ limit: settings.limit + 1 })}><Icon name="plus" /></button>
      </div>
      <p className="help" style={{ margin: "10px 0 0" }}>At most {settings.limit} {settings.limit === 1 ? "decision" : "decisions"} per stage of a run</p>
    </div>
    <hr />
    <div className="lab">Keep in mind<small>Anything it should know, such as “never approve a change to billing” or “answer in English”.</small></div>
    <div><textarea className="ta afk-guidance" aria-label="Keep in mind" value={guidance}
      onChange={event => { setGuidanceDirty(true); setGuidance(event.currentTarget.value); }}
      onBlur={() => { if (guidance !== settings.guidance) patch({ guidance }); }} /></div>
    <hr />
    <div className="lab">Runs on<small>The CLI its short sessions use, and the model and thinking they ask for.</small></div>
    <div className="stack">
      <div className="cols2">
        <div className="fld"><label htmlFor="afk-harness">Harness</label>
          <select className="sel" id="afk-harness" aria-label="Harness" value={settings.backend} onChange={event => patch({ backend: event.currentTarget.value })}>
            {backendValues.map(runner => <option key={runner} value={runner}>{backendLabels[runner]}</option>)}
          </select></div>
        <div className="fld"><label htmlFor="afk-thinking">Thinking</label>
          <select className="sel" id="afk-thinking" aria-label="Thinking" value={thinking} disabled={models.data !== undefined && models.data.thinking.length === 0} onChange={event => patch({ thinking: event.currentTarget.value === "" ? null : event.currentTarget.value })}>
            <option value="">Default</option>
            {thinkingOptions.map(option => <option key={option} value={option}>{thinkingLabel(option)}</option>)}
          </select></div>
      </div>
      <ModelPicker id="afk-model" label="Model" runnerLabel={backendLabels[settings.backend]} choices={choices} allowedModels={models.data?.allowedModels}
        value={settings.model} state={modelState} errorMessage={models.error ? describeError(models.error).message : null}
        onChange={id => patch({ model: id })} />
      {error ? <ErrorProblems error={error} /> : null}
    </div>
  </section>;
}

function DecisionRow({ decision, onReview, pending }: { decision: AfkDecision; onReview: () => void; pending: boolean }) {
  const command = useCommand();
  const [overruling, setOverruling] = useState(false);
  const offer = decision.overruleOffer;
  const tone = decision.overrule ? (decision.overrule.phase === "held" ? "problem" : "you") : decision.tone;
  const runHref = `/runs/${encodeURIComponent(decision.runId)}${decision.stageExists ? `?stage=${encodeURIComponent(decision.stageId)}` : ""}`;
  return <li className="row dr afk-row" data-tone={tone} data-needs-review={decision.needsReview ? "true" : undefined}>
    <span className="c num"><time dateTime={decision.decidedAt ?? decision.startedAt}>{formatAgo(decision.decidedAt ?? decision.startedAt)}</time></span>
    <div>
      {decision.runExists ? <Link className="t link afk-subject" to={runHref}>{decision.subjectTitle}</Link> : <div className="t afk-subject">{decision.subjectTitle}</div>}
      <div className="m"><span className="afk-ref">{decision.subjectRef}</span><span>{decision.beltName}</span><span>at {decision.stageName}</span><span>{decision.situationLabel}</span></div>
    </div>
    <span className="ch2">{decision.label}</span>
    <div className="c afk-detail-col" style={{ whiteSpace: "normal" }}>
      <p className="afk-headline">{decision.headline}</p>
      {decision.overruleLine && <p className="afk-overrule-line">{decision.overruleLine}</p>}
      {decision.reason && <p className="afk-detail">It was waiting because: {decision.reason}</p>}
      {decision.message && <blockquote className="afk-quote"><b>Its message</b> {decision.message}</blockquote>}
      {decision.why && <blockquote className="afk-quote"><b>Why</b> {decision.why}</blockquote>}
      {decision.overrule && <blockquote className="afk-quote"><b>Overruled</b> {decision.overrule.comments} <span className="help">{decision.overrule.stageLabel} · {formatAgo(decision.overrule.at)}</span></blockquote>}
      {offer && !offer.available && offer.reason && <p className="why afk-unavailable">{offer.reason}</p>}
      {decision.runExists && decision.folder !== "" && !decision.folderExists && decision.revealUnavailable && <p className="why afk-unavailable">{decision.revealUnavailable}</p>}
      {command.error ? <ErrorProblems error={command.error} /> : null}
    </div>
    <div className="acts afk-row-foot">
      {decision.needsReview && <button type="button" className="btn sm" disabled={pending} onClick={onReview}><Icon name="check" />Looks right</button>}
      {decision.reviewed && <span className="flag f">Checked</span>}
      {offer && <button type="button" className="btn sm ghost" disabled={!offer.available} title={offer.reason} onClick={() => setOverruling(true)}>Overrule</button>}
      {decision.runExists && <Link className="btn sm ghost" to={runHref}>Open run</Link>}
      {decision.runExists && decision.folder !== "" && <button type="button" className="btn sm ghost" disabled={command.pending || !decision.folderExists}
        title={decision.revealUnavailable} onClick={() => command.submit(desktopRequest("reveal", decision.runId, { kind: "afk", targetId: decision.id }) as never)}>Show session files</button>}
    </div>
    {overruling && offer?.available && <OverruleSheet decision={decision} offer={offer} onClose={() => setOverruling(false)} />}
  </li>;
}

function Decisions({ decisions, filter, counts, onFilter }: { decisions: readonly AfkDecision[]; filter: AfkFilter; counts: { review: number; all: number }; onFilter: (filter: AfkFilter) => void }) {
  const review = useCommand();
  const toCheck = counts.review > 0;
  return <section className="sec afk-decisions" aria-label="Decisions" data-column="afk-decisions" data-ready="true">
    <div className="sec-h">
      <h2 className="gh" style={{ margin: 0 }}><span className="gn">{filter === "review" ? counts.review : counts.all}</span>{filter === "review" ? "To check" : "Every decision"}</h2>
      <div className="acts">
        <label className="vh" htmlFor="afk-filter">Decision filter</label>
        <select className="sel inline" id="afk-filter" aria-label="Decision filter" value={filter} onChange={event => onFilter(event.currentTarget.value as AfkFilter)}>
          <option value="review">To review</option><option value="all">All</option>
        </select>
        {toCheck && <button type="button" className="btn sm" disabled={review.pending} onClick={() => review.submit(afkReviewRequest())}><Icon name="check" />Mark all as checked</button>}
      </div>
    </div>
    {review.error ? <ErrorProblems error={review.error} /> : null}
    {decisions.length === 0 ? <p className="empty empty-state" role="status">{filter === "review"
      ? "Nothing to check. Decisions the assistant makes appear here until you mark them as checked."
      : "The assistant hasn’t decided anything yet."}</p>
      : <ol className="rows">{decisions.map(decision => <DecisionRow key={decision.id} decision={decision} pending={review.pending} onReview={() => review.submit(afkReviewRequest([decision.id]))} />)}</ol>}
  </section>;
}

export function AfkPage() {
  const [filter, setFilter] = useState<AfkFilter>("review");
  const { decisions, settings } = useAfkQueries(filter);
  useEffect(() => { document.title = "Beltline — AFK"; }, []);
  const loaded = decisions.data !== undefined;
  const notFound = decisions.error && typeof decisions.error === "object" && "status" in decisions.error && decisions.error.status === 404;
  const state = useAfkQueries(filter).state.data;
  const on = state?.settings.afkEnabled ?? false;
  const afkAction = state?.actions.find(action => action.id === "afk");
  return <article className="pin afk-page">
    <header className="ph afk-heading">
      <h1 className="h1" aria-label="AFK">AFK is {on ? "on." : "off."}<br /><span className="soft">{on ? "It decides for you." : "Runs wait for you."}</span></h1>
      <p className="lede afk-lead">A run in AFK mode has an assistant take the steps that wait for you: it answers questions, runs failed stages again, approves and carries on after holds, within what you allow below. Each run is in AFK mode always, never, or while this switch is on (Global). A run starts with its belt's mode, and you can change it on the run's page. Every decision is logged here with its reasons, so you can check them when you are back.</p>
      {afkAction && state && <div className="ph-row"><GlobalControl action={afkAction} state={state} small={false} label={on ? "Turn AFK off" : "Turn AFK on"} /></div>}
    </header>
    {settings.data && <Allowances settings={settings.data.afk} />}
    {settings.isPending && <p className="help" role="status">Loading settings…</p>}
    {settings.error && !settings.data && <section className="error-panel"><h2>Unable to load the AFK settings</h2><ErrorProblems error={settings.error} /></section>}
    {decisions.isPending && <p className="help" role="status">Loading decisions…</p>}
    {decisions.error && (!decisions.data || notFound) && <section className="error-panel"><h2>Unable to load this page</h2><ErrorProblems error={decisions.error} /></section>}
    {loaded && !notFound && <Decisions decisions={decisions.data!.decisions} filter={filter} counts={decisions.data!.counts} onFilter={setFilter} />}
    {loaded && !notFound && <p className="loaded-state screen-reader-only" role="status">AFK decisions loaded</p>}
    {decisions.isError && decisions.data && <p className="why" role="status">Showing the last loaded decisions; refresh failed.</p>}
  </article>;
}
