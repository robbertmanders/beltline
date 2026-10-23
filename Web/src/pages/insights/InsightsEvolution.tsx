// The Evolution view of one belt: its figures for the recent runs, then each figure, where the time goes,
// approvals and holds, sends back, cost by stage and how runs end, by version or by week. The daemon sends
// every figure, statistic and tick; this lays them out.
import type { InsightsEvolutionView, InsightsFigure, InsightsFigureID } from "../../api/wire.gen";
import { Caps, Segmented } from "../../components/Paper";
import { Cumulative, FigureChart, Histogram, SendsBackGrid, StackChart } from "./EvolutionCharts";

/** What the page keeps in its URL for the view. */
export interface EvolutionChoice {
  by: "version" | "week";
  separate: boolean;
  from: number;
  log: boolean;
  figure: InsightsFigureID;
}

/** The request for a belt's Evolution view; omitted parameters are the daemon's defaults. */
export function evolutionRequestPath(belt: string, choice: EvolutionChoice, showArchived = false): string {
  const params = new URLSearchParams({ belt });
  if (choice.by !== "version") params.set("by", choice.by);
  if (choice.separate) params.set("separate", "1");
  if (choice.from > 0) params.set("from", String(choice.from));
  if (showArchived) params.set("showArchived", "1");
  return `/api/v1/views/insights/evolution?${params.toString()}`;
}

const timeColors = ["var(--slate)", "var(--gold-lamp)", "var(--metro-line-5)", "var(--track)"] as const;
const touchColors = ["var(--slate-deep)", "var(--red)", "var(--shadow-ink)"] as const;
const costColors = Array.from({ length: 8 }, (_, i) => `var(--metro-line-${i})`);

function Section({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return <section className="insights-section" aria-label={title}>
    <div className="insights-section-head"><h2 className="paper-tag paper-tag--strong">{title}</h2>{hint && <span className="insights-hint">{hint}</span>}</div>
    {children}
  </section>;
}

function Legend({ items }: { items: { label: string; color?: string; className?: string }[] }) {
  return <div className="evolution-legend">{items.map(item => <span key={item.label}>
    <i className={`evolution-key ${item.className ?? ""}`} style={item.color ? { background: item.color } : undefined} />{item.label}
  </span>)}</div>;
}

/** A figure's statistic per column as a small line, recent shaded. */
function ColumnLine({ figure, from }: { figure: InsightsFigure; from: number }) {
  const width = 300, height = 44, pad = 3;
  const values = figure.columns.map(stat => stat.value ?? null);
  const known = values.filter((value): value is number => value !== null);
  if (known.length < 2) return <svg className="insights-spark" width="100%" height={height} aria-hidden="true" />;
  const low = Math.min(...known), high = Math.max(...known);
  const x = (i: number) => pad + i * (width - 2 * pad) / Math.max(values.length - 1, 1);
  const y = (value: number) => height - pad - (high === low ? 0.5 : (value - low) / (high - low)) * (height - 2 * pad);
  let line = "", area = "", run: number[] = [];
  const close = () => {
    if (run.length > 1) area += `M${x(run[0]!)} ${height - pad}${run.map(i => `L${x(i).toFixed(1)} ${y(values[i]!).toFixed(1)}`).join("")}L${x(run[run.length - 1]!)} ${height - pad}Z`;
    run = [];
  };
  values.forEach((value, i) => {
    if (value === null) { close(); return; }
    line += `${run.length ? "L" : "M"}${x(i).toFixed(1)} ${y(value).toFixed(1)}`;
    run.push(i);
  });
  close();
  const split = from > 0 ? x(from - 0.5) : width;
  return <svg className="insights-spark" width="100%" height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
    <rect className="evolution-recent" x={split} y={0} width={width - split} height={height} />
    <path className="insights-spark-area" d={area} />
    <path className="insights-spark-line" d={line} vectorEffect="non-scaling-stroke" />
  </svg>;
}

/** The six figures for the recent runs; picking one charts it below. */
function FigureCards({ view, chosen, onChoose }: { view: InsightsEvolutionView; chosen: InsightsFigureID; onChoose: (id: InsightsFigureID) => void }) {
  const first = view.columns[0]?.label ?? "", last = view.columns.at(-1)?.label ?? "";
  return <div className="insights-kpis evolution-figures">
    {view.figures.map(figure => <button key={figure.id} type="button" className="paper-card insights-kpi evolution-figure" aria-pressed={figure.id === chosen}
      aria-label={figure.label} onClick={() => onChoose(figure.id)}>
      <header><h2>{figure.label}</h2><span className="insights-delta is-flat">{figure.change}</span></header>
      <p className="insights-kpi-value">{figure.value}</p>
      <div className="insights-wide-spark">
        <ColumnLine figure={figure} from={view.from} />
        <span className="insights-axis"><span>{first}</span><span>{figure.share ? "share" : "median"} per {view.axis}</span><span>{last}</span></span>
      </div>
      <Caps>{figure.caption}</Caps>
    </button>)}
  </div>;
}

export function Evolution({ view, choice, onChange }: { view: InsightsEvolutionView; choice: EvolutionChoice; onChange: (changes: Partial<EvolutionChoice>) => void }) {
  const figure = view.figures.find(item => item.id === choice.figure) ?? view.figures[0];
  const columns = { columns: view.columns, from: view.from, onFrom: (from: number) => onChange({ from }) };
  return <div className="evolution">
    <div className="evolution-controls">
      <Caps>Group by</Caps>
      <Segmented label="Group by" options={[{ value: "version", label: "Version" }, { value: "week", label: "Week" }] as const} value={choice.by} onChange={by => onChange({ by, from: 0 })} />
      {choice.by === "version" && <>
        <Caps>Small versions</Caps>
        <Segmented label="Small versions" options={[{ value: "merge", label: "Merge under 5 runs" }, { value: "separate", label: "Keep apart" }] as const}
          value={choice.separate ? "separate" : "merge"} onChange={value => onChange({ separate: value === "separate", from: 0 })} />
      </>}
      <Caps>Scale</Caps>
      <Segmented label="Scale" options={[{ value: "log", label: "Log" }, { value: "linear", label: "Linear" }] as const}
        value={choice.log ? "log" : "linear"} onChange={value => onChange({ log: value === "log" })} />
    </div>
    <Section title="Before and recent" hint={view.split}>
      <FigureCards view={view} chosen={figure?.id ?? choice.figure} onChoose={id => onChange({ figure: id })} />
    </Section>
    {view.columns.length === 0 && <p className="empty-state" role="status">No finished runs on this belt yet.</p>}
    {figure && view.columns.length > 0 && <>
      <Section title={`${figure.label} by ${view.axis === "week" ? "week" : "belt version"}`} hint="One dot per finished run. Click a column to start recent there.">
        <div className="paper-card evolution-card">
          <Legend items={figure.share
            ? [{ label: "Shipped", color: "var(--green)" }, { label: "Not shipped", color: "var(--bar)" }, { label: "Belt changed", className: "is-change" }]
            : [{ label: "Shipped run", color: "var(--green)", className: "is-dot" }, { label: "Done run", color: "var(--muted)", className: "is-dot" },
              { label: "Dropped run", color: "var(--red)", className: "is-dot" },
              { label: "Middle half", className: "is-middle" }, { label: "Median", color: "var(--slate-text)", className: "is-line" }, { label: "Belt changed", className: "is-change" }]} />
          <FigureChart figure={figure} runs={view.runs} log={choice.log} {...columns} />
        </div>
      </Section>
      {figure.spread && <Section title={`Spread of ${figure.label.toLowerCase()}`}>
        <div className="evolution-two">
          <div className="paper-card evolution-card">
            <div className="evolution-card-head"><Caps>Runs per range</Caps><Legend items={[{ label: "Before", className: "is-before" }, { label: "Recent", color: "var(--slate)" }]} /></div>
            <Histogram figure={figure} spread={figure.spread} log={choice.log} />
          </div>
          <div className="paper-card evolution-card">
            <Caps>Share of runs at or under</Caps>
            <Cumulative figure={figure} spread={figure.spread} log={choice.log} />
          </div>
        </div>
        <table className="work-table evolution-table" aria-label="Percentiles">
          <thead><tr><th scope="col" /><th scope="col"><Caps>Runs</Caps></th>{figure.spread.headers.map(header => <th key={header} scope="col"><Caps>{header}</Caps></th>)}</tr></thead>
          <tbody>{figure.spread.rows.map(row => <tr key={row.label} className="work-row is-fixed">
            <td><span className="work-title">{row.label}</span></td><td>{row.runs}</td>{row.cells.map((cell, i) => <td key={i}>{cell}</td>)}
          </tr>)}</tbody>
        </table>
      </Section>}
      <Section title="Where the time goes" hint="Mean active hours per run.">
        <div className="paper-card evolution-card">
          <Legend items={view.time.series.map((series, i) => ({ label: series.label, color: timeColors[i % timeColors.length] }))} />
          <StackChart stack={view.time} label="Where the time goes" colors={timeColors} {...columns} />
        </div>
      </Section>
      <Section title="Approvals and holds" hint="Mean per run.">
        <div className="paper-card evolution-card">
          <Legend items={[...view.touches.series.map((series, i) => ({ label: series.label, color: touchColors[i % touchColors.length] })),
            ...(view.touches.line ? [{ label: view.touches.line.label, color: "var(--gold-lamp)", className: "is-line" }] : [])]} />
          <StackChart stack={view.touches} label="Approvals and holds" colors={touchColors} {...columns} />
        </div>
      </Section>
      <Section title="Sends back" hint="Mean per run.">
        <div className="paper-card evolution-card"><SendsBackGrid grid={view.sendsBack} {...columns} /></div>
      </Section>
      <Section title="Cost by stage" hint="Mean per priced run.">
        <div className="paper-card evolution-card">
          {view.cost.series.length === 0 ? <p className="evolution-note">No run on this belt reported usage.</p> : <>
            <Legend items={view.cost.series.map((series, i) => ({ label: series.label, color: costColors[i % costColors.length] }))} />
            <StackChart stack={view.cost} label="Cost by stage" colors={costColors} {...columns} />
          </>}
        </div>
      </Section>
      <Section title="How runs end">
        <div className="evolution-two">
          <div className="paper-card evolution-card evolution-funnel">
            <div className="evolution-funnel-row"><span /><Caps>Before</Caps><Caps>Recent</Caps></div>
            {view.endings.funnel.map(row => <div key={row.label} className="evolution-funnel-row">
              <span>{row.label}</span>
              {[row.before, row.recent].map((rate, i) => <span key={i} className={`evolution-meter ${i === 0 ? "is-before" : ""}`}>
                <span style={{ width: `${rate.of ? (100 * rate.count) / rate.of : 0}%` }} /><em>{`${rate.count} · ${rate.percent}`}</em>
              </span>)}
            </div>)}
          </div>
          {view.endings.drops.length === 0 ? <p className="evolution-note">No dropped runs.</p> :
            <table className="work-table evolution-table" aria-label="Where runs were dropped">
              <colgroup><col /><col style={{ width: 70 }} /><col style={{ width: 70 }} /><col style={{ width: "44%" }} /></colgroup>
              <thead><tr><th scope="col"><Caps>Dropped at</Caps></th><th scope="col"><Caps>Before</Caps></th><th scope="col"><Caps>Recent</Caps></th><th scope="col" className="evolution-left"><Caps>Usual reason</Caps></th></tr></thead>
              <tbody>{view.endings.drops.map(row => <tr key={row.stage} className="work-row is-fixed">
                <td><span className="work-title">{row.stage}</span></td><td>{row.before}</td><td>{row.recent}</td><td className="evolution-left insights-muted">{row.reason}</td>
              </tr>)}</tbody>
            </table>}
        </div>
      </Section>
    </>}
  </div>;
}
