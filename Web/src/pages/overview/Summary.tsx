import type { OverviewResponse } from "../../api/wire.gen";
import { shippedCurve } from "./shippedChart";

export interface TodayProps {
  today: OverviewResponse["today"];
}

/** Shipped runs per week as a line with its area, over twelve weeks, in a 300 × 150 space; the daemon names the weeks. */
function ShippedChart({ weeks, chart }: { weeks: readonly number[]; chart: OverviewResponse["today"]["shipped"]["chart"] }) {
  const curve = shippedCurve(weeks);
  return <svg className="overview-chart" viewBox="0 0 300 150" role="img" aria-label="Pull requests shipped per week over the last twelve weeks">
    <line x1={10} x2={290} y1={120} y2={120} className="overview-chart-base" />
    <path d={curve.area} className="overview-chart-area" />
    <path d={curve.line} className="overview-chart-line" />
    <circle cx={curve.dotX} cy={curve.dotY} r={3.5} className="overview-chart-dot" />
    <text x={10} y={140} dominantBaseline="middle" className="overview-chart-axis">{chart.startLabel}</text>
    <text x={290} y={140} textAnchor="end" dominantBaseline="middle" className="overview-chart-axis">{chart.endLabel}</text>
  </svg>;
}

/** Today, so far: spend, sessions and what shipped as the mockup's figures. Every figure comes from the daemon. */
export function Today({ today }: TodayProps) {
  const { spend, sessions, shipped } = today;
  return <section className="sec overview-today" aria-label="Today">
    <div className="sec-h"><h2 className="h3">Today, so far</h2></div>
    <div className="figs">
      <section className="fig" aria-label="Spend">
        <div className="v">{spend.amount}</div>
        <div className="l">{spend.caption}</div>
      </section>
      <section className="fig" aria-label="Sessions">
        <div className="v">{sessions.today}</div>
        <div className="l">agent runs today · {sessions.running} running</div>
        <div className="overview-grid" data-testid="session-grid" title="1 square is 6 hours; darker is more runs">
          {sessions.shades.map((shade, cell) => <span key={cell} className={`overview-cell shade-${shade}`} />)}
        </div>
        <p className="overview-axis"><span>{sessions.startLabel}</span><span>Today</span></p>
      </section>
      <section className="fig" aria-label="Shipped">
        <div className="v">{shipped.latest}</div>
        <div className="l">pull requests shipped this week · {shipped.median} from issue to pull request, median</div>
        <ShippedChart weeks={shipped.weeks} chart={shipped.chart} />
      </section>
    </div>
  </section>;
}
