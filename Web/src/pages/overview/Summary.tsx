import type { ReactNode } from "react";
import type { OverviewResponse, OverviewRun, OverviewSummaryRow } from "../../api/wire.gen";
import { Caps, Chip, Spinner, Tag, type Look } from "../../components/Paper";
import { shippedCurve } from "./shippedChart";

export interface SummaryProps {
  today: OverviewResponse["today"];
  runs: ReadonlyMap<string, OverviewRun>;
  onOpenRun: (runId: string, stage?: string | null) => void;
}

/** The Running post's faint background: rows of glyphs, as the app draws them. */
const runningPattern = (() => {
  const glyphs = ["01", "+-", "//", "·→", "◐", "✓"];
  return Array.from({ length: 40 }, (_, r) => Array.from({ length: 24 }, (_, c) => glyphs[(r * 7 + c * 3) % glyphs.length]!.padEnd(4, " ")).join("")).join("\n");
})();

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
    ? <Chip tone={row.look === "held" ? "problem" : undefined}>{row.chip}</Chip>
    : <Chip look={row.look as Look}>{row.chip}</Chip>;
  return <li><button type="button" className="overview-row" onClick={onOpen}>
    <span className={`overview-glyph overview-look-${row.look}`} aria-hidden="true">{row.glyph}</span>
    <span className="issue-number">{run?.ref ?? row.runId}</span>
    <span className="overview-row-title">{row.title}</span>
    {chip}
  </button></li>;
}

function Panel({ title, count, empty, children }: { title: string; count: number; empty: string; children: ReactNode }) {
  return <section className="overview-panel paper-card" aria-label={title}>
    <header><h3 className="paper-tag paper-tag--strong">{title}</h3><Caps>{count}</Caps></header>
    {count === 0 ? <p className="overview-empty">{empty}</p> : children}
  </section>;
}

/** The Summary: four posts over two lists. Every figure, order, limit and drawing comes from the daemon. */
export function Summary({ today, runs, onOpenRun }: SummaryProps) {
  const { spend, sessions, running, shipped, inProgress, needsYou } = today;
  return <div className="overview-summary">
    <div className="overview-posts">
      <section className="overview-post paper-card paper-card--raised" aria-label="Spend">
        <header><h2>Spend</h2><Tag>Today</Tag></header>
        <p className="overview-figure">{spend.amount}</p>
        <Caps>{spend.caption}</Caps>
      </section>
      <section className="overview-post paper-card paper-card--raised" aria-label="Sessions">
        <header><h2>Sessions</h2><Tag>14 days</Tag></header>
        <Tag>Agent runs</Tag>
        <p className="overview-figure overview-figure-large">{sessions.today}</p>
        <p className="overview-mono">{sessions.runningCaption}</p>
        <div className="overview-grid" data-testid="session-grid">
          {sessions.shades.map((shade, cell) => <span key={cell} className={`overview-cell shade-${shade}`} />)}
        </div>
        <p className="overview-axis"><span>{sessions.startLabel}</span><span>Today</span></p>
        <p className="overview-axis">1 sq = 6 hours · darker = more runs</p>
      </section>
      <section className="overview-post overview-running" aria-label="Running now">
        <pre className="overview-running-pattern" aria-hidden="true">{runningPattern}</pre>
        <header><h2>Running now</h2><Tag className="on-slate">Live</Tag></header>
        <ul className="overview-running-list">
          {running.count === 0 && <li className="overview-empty-running">{running.empty}</li>}
          {running.runIds.map(id => {
            const run = runs.get(id);
            const title = run ? `${run.stageName} · ${run.title}` : id;
            return <li key={id}><button type="button" className="overview-pill-run" onClick={() => onOpenRun(id)} title={title}>
              <Spinner />
              <span className="overview-running-title">{title}</span>
              <span className="overview-running-ref">{run?.ref ?? ""}</span>
            </button></li>;
          })}
        </ul>
        <Tag className="on-slate overview-running-all">All belts</Tag>
        <p className="overview-running-foot"><span>Click one to open it</span><span className="overview-mono">{running.busy}</span></p>
      </section>
      <section className="overview-post paper-card paper-card--raised" aria-label="Shipped">
        <header><h2>Shipped</h2><Tag>12 weeks</Tag></header>
        <Tag>Pull requests</Tag>
        <ShippedChart weeks={shipped.weeks} chart={shipped.chart} />
        <p className="overview-mono">SHIPPED PER WEEK — ALL BELTS</p>
        <p className="overview-median"><span>Issue to pull request, median</span><span className="overview-mono overview-muted">{shipped.median}</span></p>
      </section>
    </div>
    <div className="overview-lists">
      <Panel title="In progress" count={inProgress.count} empty={inProgress.empty}>
        <ul className="overview-rows">
          {inProgress.rows.map(row => <Row key={row.runId} row={row} run={runs.get(row.runId)} needsYou={false} onOpen={() => onOpenRun(row.runId, row.openStage)} />)}
        </ul>
      </Panel>
      <Panel title="Needs you" count={needsYou.count} empty={needsYou.empty}>
        <ul className="overview-rows">
          {needsYou.rows.map(row => <Row key={row.runId} row={row} run={runs.get(row.runId)} needsYou onOpen={() => onOpenRun(row.runId, row.openStage)} />)}
        </ul>
      </Panel>
    </div>
  </div>;
}
