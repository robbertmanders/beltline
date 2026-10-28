import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { artifactLinkRequest, desktopRequest } from "../../api/runCommands";
import type { ArtifactLinkResponse, AttemptArtifactsResponse, AttemptOutputResponse, InlineSpan, Run, RunPresentationAttemptEntry, RunStage, RunPresentationWatchNote, Stage } from "../../api/wire.gen";
import { MarkdownBlocks } from "../../components/MarkdownBlocks";
import { Caps, Chip, Select, Tag } from "../../components/Paper";
import { ErrorProblems } from "../../components/ProblemList";
import { useRevealLabel, useRuntime } from "../../app/runtime";
import type { CommandController } from "../../api/useCommand";
import type { ConnectionStatus } from "../../api/eventConnection";

type StageFacts = NonNullable<RunStage["facts"]>;
type AttemptFacts = StageFacts["attempts"][number];

/** Parsed inline spans from the daemon: strong text and code. The browser draws them; it never parses Markdown. */
function Spans({ spans }: { spans: readonly InlineSpan[] }) {
  return <>{spans.map((span, index) => span.type === "strong" ? <strong key={index}>{span.text}</strong>
    : span.type === "code" ? <code key={index}>{span.text}</code>
      : <span key={index}>{span.text}</span>)}</>;
}

/** The AFK assistant's note on a stage, worded by the daemon from the AFK log. */
function AfkNote({ note }: { note: NonNullable<StageFacts["deciding"]> }) {
  return <div className="stage-afk-note">
    <p className="stage-afk-title">{note.title}</p>
    {note.overruleLine && <p>{note.overruleLine}</p>}
    {note.comments && <p>{note.comments}</p>}
    {note.why && <p>{note.why}</p>}
  </div>;
}

function Artifacts({ runId, attemptId, onReveal }: { runId: string; attemptId: string; onReveal: () => void }) {
  const revealLabel = useRevealLabel();
  const runtime = useRuntime();
  const [error, setError] = useState<unknown>(null);
  const query = useQuery({
    queryKey: apiQueryKey(`/api/v1/runs/${encodeURIComponent(runId)}/attempts/${encodeURIComponent(attemptId)}/artifacts`),
    queryFn: ({ signal }) => apiRequest<AttemptArtifactsResponse>(`/api/v1/runs/${encodeURIComponent(runId)}/attempts/${encodeURIComponent(attemptId)}/artifacts`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(`/api/v1/runs/${encodeURIComponent(runId)}/attempts/${encodeURIComponent(attemptId)}/artifacts`) }
  });
  // Signs a fresh link per activation. The listing's cached URL is never opened, and the isolated tab is
  // closed again when signing fails or the popup is blocked.
  const sign = useMutation({
    retry: false,
    mutationFn: ({ name }: { name: string }) => apiRequest<ArtifactLinkResponse>(artifactLinkRequest(runId, attemptId, name).path, {
      method: "POST", body: artifactLinkRequest(runId, attemptId, name).body, onUnauthorized: runtime.requireSignIn
    })
  });
  const openArtifact = async (name: string) => {
    setError(null);
    const tab = window.open("", "_blank");
    if (tab) tab.opener = null;
    try {
      const link = await sign.mutateAsync({ name });
      if (tab) tab.location.href = link.url;
      else setError(new Error("The browser blocked the new tab. Allow pop-ups for this page and try again."));
    } catch (failure) {
      tab?.close();
      setError(failure);
    }
  };
  const data: AttemptArtifactsResponse | undefined = query.data;
  if (!data || data.artifacts.length === 0) return query.error ? <ErrorProblems error={query.error} /> : null;
  return <section className="run-artifacts" aria-label="Artifacts">
    <Caps>Artifacts</Caps>
    <p className="run-muted">{data.warning}</p>
    {data.artifacts.map(artifact => <div key={artifact.name} className="artifact-row">
      <span className="artifact-name">{artifact.name}</span>
      <button type="button" className="action-button small" disabled={sign.isPending} title={data.warning}
        onClick={() => void openArtifact(artifact.name)}>Open in browser</button>
      <button type="button" className="action-button small" onClick={onReveal}>{revealLabel}</button>
    </div>)}
    {error !== null && <ErrorProblems error={error} />}
  </section>;
}

function Output({ runId, attempt, stage }: { runId: string; attempt: string; stage: Stage }) {
  const runtime = useRuntime();
  const query = useQuery({
    queryKey: apiQueryKey(`/api/v1/runs/${encodeURIComponent(runId)}/attempts/${encodeURIComponent(attempt)}/output`),
    queryFn: ({ signal }) => apiRequest<AttemptOutputResponse>(`/api/v1/runs/${encodeURIComponent(runId)}/attempts/${encodeURIComponent(attempt)}/output`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && stage.type === "agent",
    retry: false,
    staleTime: 0,
    meta: { resourceDeps: resourcesForPath(`/api/v1/runs/${encodeURIComponent(runId)}/attempts/${encodeURIComponent(attempt)}/output`) }
  });
  if (stage.type !== "agent") return null;
  if (query.isPending) return <p className="run-muted" role="status">Loading output…</p>;
  if (query.error) return <ErrorProblems error={query.error} />;
  const data = query.data;
  if (!data) return null;
  if (data.availability !== "available") return <p className="run-muted" role="status">{data.message ?? `This attempt wrote no ${stage.writes ?? "output"}.`}</p>;
  return <div className="run-output" data-attempt={data.attemptId}><MarkdownBlocks blocks={data.blocks} /></div>;
}

export interface RunStagePanelProps {
  runId: string;
  run: Run;
  panel: RunStage;
  stage: Stage;
  selectedAttempt: string | null;
  onSelectAttempt: (attemptId: string) => void;
  watchNotes: readonly RunPresentationWatchNote[];
  command: CommandController;
  canRead: boolean;
  connection: ConnectionStatus;
  sessionsOpen: boolean;
  onWatchSession: () => void;
  onAdjust: () => void;
  answer?: ReactNode;
  decision?: ReactNode;
}

/** The chip that says how the stage stands, with a running agent stage's clock ticking beside it. */
function StateChip({ chip }: { chip: NonNullable<RunStage["facts"]>["state"] }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!chip.clockSince) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [chip.clockSince]);
  const clock = chip.clockSince ? ` · ${clockText((now - new Date(chip.clockSince).getTime()) / 1000)}` : "";
  const tone = chip.tone === "running" ? { look: "running" as const } : chip.tone === "you" ? {} : { tone: chip.tone as "good" | "problem" | "grey" };
  return <Chip {...tone}>{chip.text}{clock}</Chip>;
}

/** Minutes and seconds, as Status.clock writes them. */
export function clockText(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  return `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
}

/** A gold bar that sweeps while a stage works; how long it takes isn't known. */
function WorkingBar() {
  return <span className="working-bar" role="img" aria-label="Working"><span /></span>;
}

/**
 * The selected stage of a run (RunStage): what it produced and what the outcome did, or, when the run
 * waits here, your decision.
 */
export function RunStagePanel({ runId, panel, stage, selectedAttempt, onSelectAttempt, watchNotes, command, canRead, connection, sessionsOpen, onWatchSession, onAdjust, answer, decision }: RunStagePanelProps) {
  const revealLabel = useRevealLabel();
  const attempt: RunPresentationAttemptEntry | undefined = panel.attempts.find(item => item.id === selectedAttempt) ?? panel.attempts.find(item => item.id === panel.defaultAttempt);
  const facts = panel.facts;
  const attemptFacts: AttemptFacts | undefined = facts?.attempts.find(item => item.id === attempt?.id);
  const online = canRead && connection === "connected";
  const reveal = () => { if (attempt) command.submit(desktopRequest("reveal", runId, { kind: "attempt", attemptId: attempt.id }) as never); };
  const showsDecision = attempt?.content === "decision" || (!attempt && panel.step === "waiting" && panel.kind === "gate");
  return <section className="stage-panel" aria-label={`Stage ${panel.number}: ${panel.name}`} data-stage-panel={panel.id}>
    <header className="stage-panel-head">
      <Tag>{String(panel.number).padStart(2, "0")} — {panel.name}</Tag>
      {facts && <Chip tone="grey">{attemptFacts && panel.kind === "agent" ? attemptFacts.ranOn.toUpperCase() : facts.kindChip}</Chip>}
      {facts && <StateChip chip={facts.state} />}
      <span className="stage-panel-spacer" />
      {facts?.canAdjust && <button type="button" className="action-button small" onClick={onAdjust}>Adjust</button>}
      {panel.attempts.length > 1 && <Select pill label={`Select attempt of ${panel.name}`} value={attempt?.id ?? ""}
        options={panel.attempts.map(item => ({ value: item.id, label: `Attempt ${item.number}` }))} onChange={onSelectAttempt} />}
      {attempt?.content === "historicalOutput" && <Caps>Output</Caps>}
    </header>
    <div className="stage-panel-body">
      {attempt && <p className="stage-panel-caption">{attempt.caption}</p>}
      {facts?.deciding && <AfkNote note={facts.deciding} />}
      {attemptFacts?.afk && <AfkNote note={attemptFacts.afk} />}
      {attempt && stage.type === "agent" && <Artifacts runId={runId} attemptId={attempt.id} onReveal={reveal} />}
      {answer}
      {showsDecision && facts?.approval?.prompt ? <>
        {attemptFacts?.result && <Result result={attemptFacts.result} />}
        {decision}
      </> : showsDecision ? <div className={`stage-approval${sessionsOpen ? " is-stacked" : ""}`}>
        {attemptFacts?.result && <Result result={attemptFacts.result} />}
        <div className="stage-approval-doc">
          {facts?.approval?.blocks ? <MarkdownBlocks blocks={facts.approval.blocks} /> : <p className="run-muted">{facts?.approval?.emptyDocument}</p>}
        </div>
        <div className="stage-approval-side">
          {facts?.approval?.willHeading && <div className="will-box">
            <Tag>{facts.approval.willHeading}</Tag>
            <ol>{facts.approval.will.map((line, index) => <li key={index}><Spans spans={line} /></li>)}</ol>
          </div>}
          {decision}
        </div>
      </div>
        : attempt?.content === "running" ? <div className="stage-running">
          <span>{facts?.working ?? `${panel.name} is working on it.`}</span>
          <WorkingBar />
          {stage.type === "agent" && !sessionsOpen && <button type="button" className="action-button small" onClick={onWatchSession}>Watch the session</button>}
          {stage.type === "watch" && facts?.watchLooking && <p className="run-muted">{facts.watchLooking}</p>}
          {stage.type === "watch" && facts?.watchCards.map(card => <div key={card.id} className="stage-watch-card">
            <span className="stage-watch-title">{card.title}</span>
            <span className="run-muted"><Spans spans={card.subtitle} /></span>
            <span className="run-muted">{card.state}</span>
            <a className="action-button small" href={card.url} target="_blank" rel="noopener noreferrer">Open it</a>
          </div>)}
        </div>
        : attempt?.content === "historicalOutput" ? <>
          {attempt.runningAttempt ? <p className="run-muted">Attempt {attempt.runningAttempt} is running now.</p> : null}
          {attemptFacts?.result && <Result result={attemptFacts.result} />}
          {stage.type === "agent" && <>
            <div className="stage-file-line">
              <button type="button" className="stage-file" title={revealLabel} disabled={!online} onClick={reveal}>{attemptFacts?.file}</button>
              <span>{[attemptFacts?.ranOn, attemptFacts?.duration].filter(Boolean).join(" · ")}</span>
            </div>
            <Output runId={runId} attempt={attempt.id} stage={stage} />
          </>}
          {stage.type === "approve" && <div className="stage-gate-output">
            <p className="stage-gate-decision">{attemptFacts?.decision}</p>
            <p>{attemptFacts?.decisionLine}</p>
          </div>}
          {stage.type === "watch" && <p className="stage-watch-output">{attempt.isOpen ? panel.liveWatchSummary ?? facts?.working : attempt.detail ?? attempt.outcome ?? "Done."}</p>}
        </>
        : <p className="run-muted stage-empty">{panel.emptyMessage ?? "Not started."}</p>}
      {stage.type === "watch" && watchNotes.length > 0 && <ul className="run-watch-notes" aria-label="Watch notes">
        {watchNotes.map(note => <li key={note.id} className="run-watch-note"><span className="run-watch-reference">{note.reference}</span><span>{note.note}</span></li>)}
      </ul>}
    </div>
    {facts?.adjustments.map((adjustment, index) => <p key={index} className="stage-adjustment">{adjustment.text}</p>)}
  </section>;
}

/** What the outcome was, and the note or detail that went with it, above a dashed rule. The daemon words the lines. */
function Result({ result }: { result: NonNullable<AttemptFacts["result"]> }) {
  const problem = result.problem;
  return <div className="stage-result">
    {result.outcome && <div className="stage-result-outcome">
      <Caps>Outcome</Caps>
      <Chip tone={(result.tone === "good" ? "good" : result.tone === "problem" ? "problem" : result.tone === "you" ? undefined : "grey") as "good" | "problem" | "grey" | undefined}>{result.outcome}</Chip>
      {result.route && <span>→ {result.route}</span>}
    </div>}
    {result.note && <p className={problem ? "is-problem" : ""}>{result.note}</p>}
    {result.detail && <p className={problem ? "is-problem" : ""}>{result.detail}</p>}
  </div>;
}
