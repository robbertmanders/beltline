import type { ReactNode } from "react";
import type { OverviewResponse, OverviewRun, OverviewSummaryRow } from "../../api/wire.gen";
import { Caps, Chip, Spinner, Tag, type Look } from "../../components/Paper";
import { shippedCurve } from "./shippedChart";

export interface SummaryProps {
  today: OverviewResponse["today"];
  runs: ReadonlyMap<string, OverviewRun>;
  onOpenRun: (runId: string, stage?: string | null) => void;
}

/** Shipped runs per week as a smooth line with its area, over twelve weeks, in a 300 × 150 space; the daemon names the weeks. */
function ShippedChart({ weeks, chart }: { weeks: readonly number[]; chart: OverviewResponse["today"]["shipped"]["chart"] }) {
  const curve = shippedCurve(weeks);
  return <svg className="overview-chart" viewBox="0 0 300 150" role="img" aria-label="Pull requests shipped per week over the last twelve weeks">
    {[20, 60, 100].map(y => <line key={y} x1={10} x2={290} y1={y} y2={y} className="overview-chart-rule" />)}
    <line x1={10} x2={290} y1={120} y2={120} className="overview-chart-base" />
    <path d={curve.area} className="overview-chart-area" />
    <path d={curve.line} className="overview-chart-line" />
    <circle cx={curve.dotX} cy={curve.dotY} r={8} className="overview-chart-halo" />
    <circle cx={curve.dotX} cy={curve.dotY} r={4} className="overview-chart-dot" />
    <text x={10} y={138} dominantBaseline="middle" className="overview-chart-axis">{chart.startLabel}</text>
    <text x={290} y={138} textAnchor="end" dominantBaseline="middle" className="overview-chart-axis">{chart.endLabel}</text>
    <text x={292} y={12} textAnchor="end" dominantBaseline="middle" className="overview-chart-latest">{chart.latestLabel}</text>
  </svg>;
}

function Row({ row, run, needsYou, onOpen }: { row: OverviewSummaryRow; run: OverviewRun | undefined; needsYou: boolean; onOpen: () => void }) {
  const chip = needsYou
    ? <Chip tone={row.look === "held" ? "problem" : undefined} className={row.look === "held" ? "" : "paper-chip--need"}>{row.chip}</Chip>
    : <Chip look={row.look as Look}>{row.chip}</Chip>;
  return <li><button type="button" className="overview-row" onClick={onOpen}>
    <span className={`overview-glyph overview-look-${row.look}`} aria-hidden="true">{row.glyph}</span>
    <span className="issue-number">{run?.ref ?? row.runId}</span>
    <span className="overview-row-title">{row.title}</span>
    {chip}
  </button></li>;
}

/** A list under a heading and its count, drawn as rows between hairlines. */
function Panel({ title, count, empty, className = "", children }: { title: string; count: number; empty: string; className?: string; children: ReactNode }) {
  return <section className={`overview-panel ${className}`} aria-label={title}>
    <header><h2>{title}</h2><span className="overview-count">{count}</span></header>
    {count === 0 ? <p className="overview-empty">{empty}</p> : children}
  </section>;
}

/** One of the day's figures: a label, the figure or drawing, and its caption, over a hairline. */
function Figure({ title, note, children }: { title: string; note: string; children: ReactNode }) {
  return <section className="overview-post" aria-label={title}>
    <header><h2>{title}</h2><Tag>{note}</Tag></header>
    {children}
  </section>;
}

/** The Summary: what needs you first, then what is moving, then the day's figures. Every figure, order, limit and drawing comes from the daemon. */
export function Summary({ today, runs, onOpenRun }: SummaryProps) {
  const { spend, sessions, running, shipped, inProgress, needsYou } = today;
  return <div className="overview-summary">
    <Panel title="Needs you" count={needsYou.count} empty={needsYou.empty} className="overview-panel--needs">
      <ul className="overview-rows">
        {needsYou.rows.map(row => <Row key={row.runId} row={row} run={runs.get(row.runId)} needsYou onOpen={() => onOpenRun(row.runId, row.openStage)} />)}
      </ul>
    </Panel>
    <div className="overview-lists">
      <Panel title="In progress" count={inProgress.count} empty={inProgress.empty}>
        <ul className="overview-rows">
          {inProgress.rows.map(row => <Row key={row.runId} row={row} run={runs.get(row.runId)} needsYou={false} onOpen={() => onOpenRun(row.runId, row.openStage)} />)}
        </ul>
      </Panel>
      <section className="overview-panel overview-running" aria-label="Running now">
        <header><h2>Running now</h2><Tag>{running.busy}</Tag></header>
        <ul className="overview-rows">
          {running.count === 0 && <li className="overview-empty">{running.empty}</li>}
          {running.runIds.map(id => {
            const run = runs.get(id);
            const title = run ? `${run.stageName} · ${run.title}` : id;
            return <li key={id}><button type="button" className="overview-row" onClick={() => onOpenRun(id)} title={title}>
              <span className="overview-glyph" aria-hidden="true"><Spinner /></span>
              <span className="issue-number">{run?.ref ?? ""}</span>
              <span className="overview-row-title">{title}</span>
            </button></li>;
          })}
        </ul>
      </section>
    </div>
    <div className="overview-posts">
      <Figure title="Spend" note="Today">
        <p className="overview-figure">{spend.amount}</p>
        <Caps>{spend.caption}</Caps>
      </Figure>
      <Figure title="Sessions" note="14 days">
        <p className="overview-figure">{sessions.today}<span className="overview-figure-unit">agent runs</span></p>
        <Caps>{sessions.runningCaption}</Caps>
        <div className="overview-grid" data-testid="session-grid">
          {sessions.shades.map((shade, cell) => <span key={cell} className={`overview-cell shade-${shade}`} />)}
        </div>
        <p className="overview-axis"><span>{sessions.startLabel}</span><span>1 sq = 6 hours · darker = more runs</span><span>Today</span></p>
      </Figure>
      <Figure title="Shipped" note="Pull requests, 12 weeks">
        <ShippedChart weeks={shipped.weeks} chart={shipped.chart} />
        <p className="overview-median"><span>Issue to pull request, median</span><span className="overview-muted">{shipped.median}</span></p>
      </Figure>
    </div>
  </div>;
}
