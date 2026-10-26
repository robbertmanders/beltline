import { useMemo, type CSSProperties, type ReactNode } from "react";
import type { BoxMap, BoxMapEdge, BoxMapNode, BoxMapRoutePill, StageIcon, Tint } from "../../api/wire.gen";
import { placeBoxMap, runBoxMetrics, type BoxGridMetrics } from "./boxGrid";
import { Glyph } from "./glyphs";
import { arcTrack } from "./tracks";
import "./stages.css";

/** A box's state, which sets its border and fill (the mockup's st-* classes). */
export type BoxState = "idle" | "done" | "todo" | "skip" | "needs" | "held" | "running" | "watching" | "paused";
/** How a track is drawn: travelled, still to come, or a route not taken. */
export type TrackState = "done" | "todo" | "skip";

/** The square icon tile at a box's left, in its agent's colour; the trigger's is dark. */
export function StageIconTile({ icon, tint, size = 28 }: { icon: StageIcon; tint?: Tint | null; size?: number }) {
  const style: CSSProperties = { width: size, height: size, background: tint ? `var(--tint-${tint})` : undefined };
  return <span className={`stage-icon${tint ? "" : " stage-icon--dark"}`} style={style}><Glyph icon={icon} size={Math.round(size * 0.54)} /></span>;
}

export interface StageBoxProps {
  icon: StageIcon;
  tint?: Tint | null;
  /** The small caps line, such as "02 · Agent". */
  caption: string;
  title: string;
  /** Lines under the title, such as the runner or the output. */
  details?: readonly string[];
  state?: BoxState;
  trigger?: boolean;
  selected?: boolean;
  /** The editor's tall box: icon and caption on one row, the words under it. */
  tall?: boolean;
  /** The corner badge: ! when it waits for you, ? when it's held. */
  dot?: "needs" | "held" | null;
  style?: CSSProperties;
  onClick?: () => void;
  /** The stage this box draws, exposed as data-stage so a page can find it. */
  dataStage?: string;
  label?: string;
  hint?: string;
  children?: ReactNode;
}

/** One stage as a box: a tinted icon, its kind and number, its name and a line or two under it. */
export function StageBox({ icon, tint, caption, title, details = [], state = "idle", trigger, selected, tall, dot, style, onClick, dataStage, label, hint, children }: StageBoxProps) {
  const className = ["stage-box", `stage-box--${state}`, trigger ? "stage-box--trigger" : "", selected ? "stage-box--selected" : "", tall ? "stage-box--tall" : ""].filter(Boolean).join(" ");
  const words = <span className="stage-box__words">
    {!tall && <span className="stage-box__caption">{caption}</span>}
    <span className="stage-box__title">{title}</span>
    {details.map((line, index) => <span key={index} className="stage-box__detail">{line}</span>)}
  </span>;
  const body = <>
    {tall ? <span className="stage-box__head"><StageIconTile icon={icon} tint={trigger ? null : tint} size={24} /><span className="stage-box__caption">{caption}</span></span>
      : <StageIconTile icon={icon} tint={trigger ? null : tint} />}
    {words}
    {dot && <span className={`stage-box__dot stage-box__dot--${dot}`} aria-hidden="true">{dot === "needs" ? "!" : "?"}</span>}
    {children}
  </>;
  return onClick
    ? <button type="button" className={className} style={style} onClick={onClick} aria-pressed={selected} aria-label={label} title={hint} data-stage={dataStage}>{body}</button>
    : <div className={className} style={style} aria-label={label} title={hint} data-stage={dataStage}>{body}</div>;
}

/** What a page says about each node of a BoxMap: its words and state. */
export interface BoxContent {
  caption: string;
  title: string;
  details?: readonly string[];
  state?: BoxState;
  dot?: "needs" | "held" | null;
  /** Drawn centred above the box, such as a status chip. */
  above?: ReactNode;
  label?: string;
}

export interface BoxMapViewProps {
  map: BoxMap;
  content: (node: BoxMapNode) => BoxContent;
  track?: (edge: BoxMapEdge) => TrackState;
  /** Whether a route's pill is lit (taken) or faded. */
  routeTaken?: (route: BoxMapRoutePill) => boolean;
  selected?: string | null;
  onSelect?: (nodeId: string) => void;
  label: string;
  metrics?: BoxGridMetrics;
}

/**
 * A belt as stage boxes on hooked tracks. The daemon says which column and row each box takes and which corners each
 * track turns; this places them in points (placeBoxMap), and the page supplies words and states.
 */
export function BoxMapView({ map, content, track = () => "todo", routeTaken = () => true, selected, onSelect, label, metrics = runBoxMetrics }: BoxMapViewProps) {
  const placed = useMemo(() => placeBoxMap(map, metrics), [map, metrics]);
  const order: Record<TrackState, number> = { skip: 0, todo: 1, done: 2 };
  const edges = placed.edges.map(item => ({ ...item, state: track(item.edge) })).sort((a, b) => order[a.state] - order[b.state]);
  return <div className="box-map-scroll" data-scroll-viewport>
    <div className="box-map" style={{ width: placed.width, height: placed.height }} role="group" aria-label={label}>
      <svg className="box-map__tracks" width={placed.width} height={placed.height} aria-hidden="true">
        {edges.map(({ edge, points, state }) => <path key={`${edge.from}>${edge.to}>${edge.route ?? ""}`} className={`box-track box-track--${state}`} d={arcTrack(points, metrics.corner)} />)}
      </svg>
      {placed.routes.map(({ route, x, y }) => <span key={`${route.fork}:${route.route}`} className={`box-map__route${routeTaken(route) ? "" : " box-map__route--off"}`}
        style={{ left: x, top: y }}>{route.name}</span>)}
      {placed.nodes.map(({ node, ...box }) => {
        const words = content(node);
        return <div key={node.id} className="box-map__node">
          {words.above && <div className="box-map__above" style={{ left: box.x + box.width / 2, top: box.y - 7 }}>{words.above}</div>}
          <StageBox icon={node.icon} tint={node.tint} caption={words.caption} title={words.title} details={words.details} state={words.state}
            trigger={node.kind === "trigger"} dot={words.dot} selected={selected === node.id} label={words.label} dataStage={node.id}
            style={{ left: box.x, top: box.y, width: box.width, height: box.height }}
            onClick={onSelect ? () => onSelect(node.id) : undefined} />
        </div>;
      })}
    </div>
  </div>;
}
