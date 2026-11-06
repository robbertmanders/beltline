import { Caps, Chip } from "../../components/Paper";
import type { InsightsFigures, InsightsShadow, InsightsShadowFigure, InsightsShadowStage, InsightsStage, InsightsDeltaTone } from "../../api/wire.gen";

/** A share as a short bar with its label, in the version's colour. */
function Share({ figures, shadow, tone }: { figures: InsightsFigures; shadow?: boolean; tone?: InsightsDeltaTone }) {
  const rate = figures.firstTry;
  return <span className={`insights-share shadow-pair-value ${shadow ? "is-shadow" : ""} ${tone ? `is-${tone}` : ""}`} title={`${rate.count} of ${rate.of}`}>
    <span className="insights-share-track" aria-hidden="true"><span style={{ width: `${rate.of ? (100 * rate.count) / rate.of : 0}%` }} /></span>
    <span className="insights-share-label">{rate.percent}</span>
  </span>;
}

/** Live over shadow in one cell; the shadow's value carries which way it differs. */
function Pair({ live, shadow, tone }: { live: string | number; shadow: string | number; tone?: InsightsDeltaTone }) {
  return <span className="shadow-pair">
    <span className="shadow-pair-value">{live}</span>
    <span className={`shadow-pair-value is-shadow ${tone ? `is-${tone}` : ""}`}>{shadow}</span>
  </span>;
}

const cost = (figures: InsightsFigures) => figures.cost === "not tracked" ? "–" : figures.cost;

/** One of the four figures for both versions, with their bars and the shadow's change against live. */
function FigureCard({ figure }: { figure: InsightsShadowFigure }) {
  return <section className="paper-card insights-kpi shadow-kpi" aria-label={figure.label}>
    <header><h2>{figure.label}</h2><span className={`insights-delta is-${figure.delta.tone}`} title={figure.delta.caption}>{figure.delta.label}</span></header>
    <div className="shadow-kpi-pair">
      <div><p className="insights-kpi-value">{figure.live.value}</p><Caps>{`Live · ${figure.live.caption}`}</Caps></div>
      <div><p className="insights-kpi-value is-shadow">{figure.shadow.value}</p><Caps>{`Shadow · ${figure.shadow.caption}`}</Caps></div>
    </div>
    <div className="shadow-kpi-bars" aria-hidden="true">
      <span className="shadow-meter"><i style={{ width: `${Math.round(figure.live.bar * 100)}%` }} /></span>
      <span className="shadow-meter is-shadow"><i style={{ width: `${Math.round(figure.shadow.bar * 100)}%` }} /></span>
    </div>
  </section>;
}

const columns = ["Stage", "Visits", "Visits / run", "Tries / visit", "First try", "Median time", "Cost"];

function SharedStages({ stages }: { stages: InsightsShadowStage[] }) {
  return <section className="insights-section" aria-labelledby="insights-shadow-stages">
    <div className="insights-section-head">
      <h2 id="insights-shadow-stages" className="paper-tag paper-tag--strong">Shared stages</h2>
      <span className="insights-hint">Stages in both versions, live over shadow. A visit is one entry of a run into the stage; tries and cost are per visit, all its attempts together.</span>
    </div>
    <table className="work-table insights-table shadow-table" aria-label="Shared stages">
      <colgroup><col /><col style={{ width: 64 }} /><col style={{ width: 84 }} /><col style={{ width: 84 }} /><col style={{ width: 150 }} /><col style={{ width: 110 }} /><col style={{ width: 96 }} /></colgroup>
      <thead><tr>{columns.map(name => <th key={name} scope="col"><Caps>{name}</Caps></th>)}</tr></thead>
      <tbody>
        {stages.length === 0 && <tr><td className="empty-state" colSpan={columns.length} role="status">No finished runs to compare yet.</td></tr>}
        {stages.map((stage, index) => <tr key={stage.key} className="work-row is-fixed" data-stage={stage.key}>
          <td className="insights-name"><span className="insights-name-line">
            <span className="insights-order">{String(index + 1).padStart(2, "0")}</span>
            <span className="work-title">{stage.name}</span>
            {stage.changed && <Chip tone="shadow">Changed</Chip>}
            {stage.kindLabel && <Chip tone="grey">{stage.kindLabel}</Chip>}
          </span></td>
          <td><Pair live={stage.live.visits} shadow={stage.shadow.visits} /></td>
          <td><Pair live={stage.live.visitsPerRun} shadow={stage.shadow.visitsPerRun} tone={stage.tones.visitsPerRun} /></td>
          <td><Pair live={stage.live.tries} shadow={stage.shadow.tries} tone={stage.tones.tries} /></td>
          <td><span className="shadow-pair"><Share figures={stage.live} /><Share figures={stage.shadow} shadow tone={stage.tones.firstTry} /></span></td>
          <td><Pair live={stage.live.medianTime} shadow={stage.shadow.medianTime} tone={stage.tones.medianTime} /></td>
          <td><Pair live={cost(stage.live)} shadow={cost(stage.shadow)} tone={stage.tones.cost} /></td>
        </tr>)}
      </tbody>
    </table>
  </section>;
}

function OnlyStages({ title, stages, shadow }: { title: string; stages: InsightsStage[]; shadow: boolean }) {
  if (stages.length === 0) return null;
  const id = shadow ? "insights-only-shadow" : "insights-only-live";
  return <section className="insights-section" aria-labelledby={id}>
    <div className="insights-section-head">
      <h2 id={id} className="paper-tag paper-tag--strong">{title}</h2>
      <span className="insights-hint">The other version has no such stage to set beside these.</span>
    </div>
    <table className="work-table insights-table shadow-table" aria-label={title}>
      <colgroup><col /><col style={{ width: 64 }} /><col style={{ width: 84 }} /><col style={{ width: 84 }} /><col style={{ width: 150 }} /><col style={{ width: 110 }} /><col style={{ width: 96 }} /></colgroup>
      <thead><tr>{columns.map(name => <th key={name} scope="col"><Caps>{name}</Caps></th>)}</tr></thead>
      <tbody>{stages.map(stage => <tr key={stage.key} className="work-row is-fixed">
        <td className="insights-name"><span className="insights-name-line">
          <span className="work-title">{stage.name}</span>
          <Chip tone={shadow ? "shadow" : "grey"}>{shadow ? "Added" : "Removed"}</Chip>
        </span></td>
        <td>{stage.figures.visits}</td>
        <td>{stage.figures.visitsPerRun}</td>
        <td>{stage.figures.tries}</td>
        <td><Share figures={stage.figures} shadow={shadow} /></td>
        <td>{stage.figures.medianTime}</td>
        <td>{cost(stage.figures)}</td>
      </tr>)}</tbody>
    </table>
  </section>;
}

/** A shadow test: the two versions, their four figures, the stages both have, and those only one has. */
export function ShadowComparison({ shadow, onDecide }: { shadow: InsightsShadow; onDecide?: (decision: "accept" | "discard") => void }) {
  return <>
    <section className="paper-card shadow-arms" aria-label="The two versions">
      <div className="shadow-arm"><span className="shadow-key" aria-hidden="true" /><span><strong>{shadow.live.label}</strong><Caps>{shadow.live.caption}</Caps></span></div>
      <div className="shadow-arm"><span className="shadow-key is-shadow" aria-hidden="true" /><span><strong>{shadow.shadow.label}</strong><Caps>{shadow.shadow.caption}</Caps></span></div>
      <p className="shadow-arms-note">{shadow.caption}</p>
      {shadow.running && onDecide && <div className="shadow-arms-actions">
        <button type="button" className="btn" onClick={() => onDecide("discard")}>Discard shadow…</button>
        <button type="button" className="btn shadow-primary" onClick={() => onDecide("accept")}>Accept shadow…</button>
      </div>}
      {!shadow.running && shadow.result && <Chip tone={shadow.result === "accepted" ? "good" : "grey"}>{shadow.result}</Chip>}
    </section>
    <div className="insights-kpis shadow-kpis">{shadow.figures.map(figure => <FigureCard key={figure.id} figure={figure} />)}</div>
    <SharedStages stages={shadow.stages} />
    <OnlyStages title={`Only in ${shadow.shadow.label.toLowerCase()}`} stages={shadow.onlyShadow} shadow />
    <OnlyStages title={`Only in ${shadow.live.label.toLowerCase()}`} stages={shadow.onlyLive} shadow={false} />
  </>;
}
