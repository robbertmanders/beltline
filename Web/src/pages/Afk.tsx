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
import { Select, Tag } from "../components/Paper";
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

  return <section className="settings-card" aria-label="What it may do">
    <p className="paper-tag paper-tag--strong">What it may do</p>
    <div className="afk-allowances">
      <label><input type="checkbox" checked={settings.answerQuestions} onChange={event => patch({ answerQuestions: event.currentTarget.checked })} />Answer an agent’s open questions</label>
      <label><input type="checkbox" checked={settings.rerunFailures} onChange={event => patch({ rerunFailures: event.currentTarget.checked })} />Run failed stages again</label>
      <label><input type="checkbox" checked={settings.approve} onChange={event => patch({ approve: event.currentTarget.checked })} />Decide at approval stages</label>
      <label><input type="checkbox" checked={settings.resolveHolds} onChange={event => patch({ resolveHolds: event.currentTarget.checked })} />Carry on after other holds</label>
      <label><input type="checkbox" checked={settings.drop} onChange={event => patch({ drop: event.currentTarget.checked })} />Drop runs</label>
    </div>
    {/* The app's stepper: the sentence carries the number, and the arrows move it between 1 and 10. */}
    <div className="afk-limit-row" role="group" aria-label="Decisions per stage">
      <span>At most {settings.limit} {settings.limit === 1 ? "decision" : "decisions"} per stage of a run</span>
      <span className="paper-stepper">
        <button type="button" aria-label="More decisions per stage" disabled={settings.limit >= 10} onClick={() => patch({ limit: settings.limit + 1 })}>
          <svg width="10" height="6" viewBox="0 0 10 6" aria-hidden="true"><path d="M1 5l4-4 4 4" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" /></svg>
        </button>
        <button type="button" aria-label="Fewer decisions per stage" disabled={settings.limit <= 1} onClick={() => patch({ limit: settings.limit - 1 })}>
          <svg width="10" height="6" viewBox="0 0 10 6" aria-hidden="true"><path d="M1 1l4 4 4-4" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" /></svg>
        </button>
      </span>
    </div>
    <p className="muted-line">Past that it leaves the run for you, so it never goes round in circles. It always may leave a run for you, and never touches a run you stopped yourself.</p>
    <p className="paper-caps">Keep in mind</p>
    <textarea className="paper-field afk-guidance" aria-label="Keep in mind" value={guidance}
      onChange={event => { setGuidanceDirty(true); setGuidance(event.currentTarget.value); }}
      onBlur={() => { if (guidance !== settings.guidance) patch({ guidance }); }} />
    <p className="muted-line">Anything it should know, such as “never approve a change to billing” or “answer in English”.</p>
    <p className="paper-caps">Harness</p>
    <div className="field-row afk-runs-on">
      <select className="paper-field paper-native-select" aria-label="Harness" value={settings.backend} onChange={event => patch({ backend: event.currentTarget.value })}>
        {backendValues.map(runner => <option key={runner} value={runner}>{backendLabels[runner]}</option>)}
      </select>
      <label>Thinking
        <select className="paper-field paper-native-select" aria-label="Thinking" value={thinking} disabled={models.data !== undefined && models.data.thinking.length === 0} onChange={event => patch({ thinking: event.currentTarget.value === "" ? null : event.currentTarget.value })}>
          <option value="">Default</option>
          {thinkingOptions.map(option => <option key={option} value={option}>{thinkingLabel(option)}</option>)}
        </select>
      </label>
    </div>
    <ModelPicker id="afk-model" label="Model" runnerLabel={backendLabels[settings.backend]} choices={choices} allowedModels={models.data?.allowedModels}
      value={settings.model} state={modelState} errorMessage={models.error ? describeError(models.error).message : null}
      onChange={id => patch({ model: id })} />
    {error ? <ErrorProblems error={error} /> : null}
  </section>;
}

function DecisionRow({ decision, onReview, pending }: { decision: AfkDecision; onReview: () => void; pending: boolean }) {
  const command = useCommand();
  const [overruling, setOverruling] = useState(false);
  const offer = decision.overruleOffer;
  const tone = decision.overrule ? (decision.overrule.phase === "held" ? "problem" : "you") : decision.tone;
  const runHref = `/runs/${encodeURIComponent(decision.runId)}${decision.stageExists ? `?stage=${encodeURIComponent(decision.stageId)}` : ""}`;
  return <article className="afk-row" data-tone={tone} data-needs-review={decision.needsReview ? "true" : undefined}>
    <header className="afk-row-head">
      <span className={`paper-chip paper-chip--${tone}`}>{decision.label}</span>
      <span className="paper-chip paper-chip--grey">{decision.situationLabel.toUpperCase()}</span>
      <span className="mono-note">{decision.beltName} · {decision.stageName}</span>
      <span className="afk-row-spacer" />
      <time className="mono-note" dateTime={decision.decidedAt ?? decision.startedAt}>{formatAgo(decision.decidedAt ?? decision.startedAt)}</time>
    </header>
    <p className="afk-subject"><span className="afk-ref">{decision.subjectRef}</span> {decision.subjectTitle}</p>
    <p className="afk-headline">{decision.headline}</p>
    {decision.overruleLine && <p className="afk-overrule-line">{decision.overruleLine}</p>}
    {decision.reason && <p className="afk-detail">It was waiting because: {decision.reason}</p>}
    {decision.message && <blockquote className="afk-quote"><span className="paper-caps">Its message</span>{decision.message}</blockquote>}
    {decision.why && <blockquote className="afk-quote"><span className="paper-caps">Why</span>{decision.why}</blockquote>}
    {decision.overrule && <blockquote className="afk-quote">
      <span className="paper-caps">Overruled</span>{decision.overrule.comments}
      <span className="mono-note">{decision.overrule.stageLabel} · {formatAgo(decision.overrule.at)}</span>
    </blockquote>}
    <footer className="afk-row-foot">
      {decision.runExists && <Link className="action-button small" to={runHref}>Open run</Link>}
      {decision.runExists && decision.folder !== "" && <button type="button" className="action-button small" disabled={command.pending || !decision.folderExists}
        title={decision.revealUnavailable} onClick={() => command.submit(desktopRequest("reveal", decision.runId, { kind: "afk", targetId: decision.id }) as never)}>Show session files</button>}
      <span className="afk-row-spacer" />
      {offer && <button type="button" className="action-button small" disabled={!offer.available} title={offer.reason} onClick={() => setOverruling(true)}>Overrule</button>}
      {decision.needsReview && <button type="button" className="action-button small slate" disabled={pending} onClick={onReview}>Looks right</button>}
      {decision.reviewed && <span className="paper-caps">Checked</span>}
    </footer>
    {offer && !offer.available && offer.reason && <p className="afk-unavailable">{offer.reason}</p>}
    {decision.runExists && decision.folder !== "" && !decision.folderExists && decision.revealUnavailable && <p className="afk-unavailable">{decision.revealUnavailable}</p>}
    {command.error ? <ErrorProblems error={command.error} /> : null}
    {overruling && offer?.available && <OverruleSheet decision={decision} offer={offer} onClose={() => setOverruling(false)} />}
  </article>;
}

function Decisions({ decisions, filter, counts, onFilter }: { decisions: readonly AfkDecision[]; filter: AfkFilter; counts: { review: number; all: number }; onFilter: (filter: AfkFilter) => void }) {
  const review = useCommand();
  const toCheck = counts.review > 0;
  return <section className="settings-card afk-decisions" aria-label="Decisions" data-column="afk-decisions" data-ready="true">
    <header className="afk-decisions-head">
      <span className="paper-tag paper-tag--strong">Decisions</span>
      <Select pill label="Decision filter" value={filter} onChange={onFilter} width={150}
        options={[{ value: "review", label: "To review" }, { value: "all", label: "All" }]} />
      <span className="afk-row-spacer" />
      {toCheck && <button type="button" className="action-button small" disabled={review.pending} onClick={() => review.submit(afkReviewRequest())}>Mark all as checked</button>}
    </header>
    {review.error ? <ErrorProblems error={review.error} /> : null}
    {decisions.length === 0 && <p className="empty-state" role="status">{filter === "review"
      ? "Nothing to check. Decisions the assistant makes appear here until you mark them as checked."
      : "The assistant hasn’t decided anything yet."}</p>}
    {decisions.map(decision => <DecisionRow key={decision.id} decision={decision} pending={review.pending} onReview={() => review.submit(afkReviewRequest([decision.id]))} />)}
  </section>;
}

export function AfkPage() {
  const [filter, setFilter] = useState<AfkFilter>("review");
  const { decisions, settings } = useAfkQueries(filter);
  useEffect(() => { document.title = "Beltline — AFK"; }, []);
  const loaded = decisions.data !== undefined;
  const notFound = decisions.error && typeof decisions.error === "object" && "status" in decisions.error && decisions.error.status === 404;
  return <article className="page-content">
    <header className="page-heading afk-heading">
      <div className="work-title-block"><Tag>Beltline</Tag><h1>AFK</h1></div>
    </header>
    <p className="page-lead afk-lead">A run in AFK mode has an assistant take the steps that wait for you: it answers questions, runs failed stages again, approves and carries on after holds, within what you allow below. Each run is in AFK mode always, never, or while this switch is on (Global). A run starts with its belt's mode, set with the AFK button beside Pause belt, and you can change it on the run's page. The assistant decides in a short agent session of its own and logs every decision here, with its reasons, so you can check them when you are back.</p>
    <div className="afk-layout">
      {decisions.isPending && <section className="settings-card"><p className="loading-state" role="status">Loading decisions…</p></section>}
      {decisions.error && (!decisions.data || notFound) && <section className="error-panel"><h2>Unable to load this page</h2><ErrorProblems error={decisions.error} /></section>}
      {loaded && !notFound && <Decisions decisions={decisions.data!.decisions} filter={filter} counts={decisions.data!.counts} onFilter={setFilter} />}
      {settings.data && <Allowances settings={settings.data.afk} />}
      {settings.isPending && <section className="settings-card"><p className="loading-state" role="status">Loading settings…</p></section>}
      {settings.error && !settings.data && <section className="error-panel"><h2>Unable to load the AFK settings</h2><ErrorProblems error={settings.error} /></section>}
      {loaded && !notFound && <p className="loaded-state screen-reader-only" role="status">AFK decisions loaded</p>}
    </div>
    {decisions.isError && decisions.data && <p className="stale-error" role="status">Showing the last loaded decisions; refresh failed.</p>}
  </article>;
}
