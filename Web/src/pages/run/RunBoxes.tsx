import { useMemo, useRef, type CSSProperties } from "react";
import type { RunBoxes as RunBoxesData, RunBoxesNode } from "../../api/wire.gen";
import { placeBoxMap, type BoxGridMetrics } from "../../theme/stages/boxGrid";
import { arcTrack } from "../../theme/stages/tracks";
import { useCentreOnStage } from "./centerOnStage";
import "../../theme/stages/stages.css";

export interface RunBoxesProps {
  boxes: RunBoxesData;
  metadata: string;
  selectedStageId: string | null;
  onSelectStage: (stageId: string) => void;
  /** The run and its current stage, which the line opens centred on. */
  opening?: { run: string; stage: string };
}

/**
 * The sizes the run's line is placed at, as the Floor draws a stage: a cell as wide as the Floor's, the line through its
 * middle with the name above it and the status under it, and a fork's routes a row apart.
 */
export const runLineMetrics: BoxGridMetrics = { nodeWidth: 150, nodeHeight: 60, gap: 36, rowHeight: 78, padX: 0, padTop: 6, padBottom: 6, corner: 10 };

/** The Floor's looks for a stage from the daemon's state: travelled, where the run is, waiting for you, to come, or not taken. */
export function nodeLook(state: RunBoxesNode["state"] | undefined): string {
  switch (state) {
    case "done": return "is-done";
    case "running": case "watching": case "paused": return "is-here";
    case "needs": case "held": return "is-here is-waiting";
    case "skip": return "is-skip";
    default: return "is-todo";
  }
}

/** A track's identity: the boxes it joins and the route it runs on, since two routes can join the same boxes. */
function trackKey(edge: { from: string; to: string; route?: string }): string {
  return `${edge.from}>${edge.to}>${edge.route ?? ""}`;
}

/**
 * The run's belt drawn the way the Floor draws one: each stage a dot on a hairline with its name above and its status
 * under it, and the tracks between them hairlines that turn on round corners, in ink where the run has travelled. The
 * daemon puts every stage in its column and row and says which corners each track turns; the page places them in
 * points and reports a click on a stage. The trigger is where the line starts, so selecting it does nothing.
 */
export function RunBoxes({ boxes, metadata, selectedStageId, onSelectStage, opening }: RunBoxesProps) {
  const section = useRef<HTMLElement>(null);
  useCentreOnStage(section, opening);
  const placed = useMemo(() => placeBoxMap(boxes.map, runLineMetrics), [boxes.map]);
  const words = new Map(boxes.nodes.map(node => [node.id, node]));
  const tracks = new Map(boxes.edges.map(edge => [trackKey(edge), edge.state]));
  const taken = new Set(boxes.takenRoutes);
  const order = { skip: 0, todo: 1, done: 2 } as const;
  const edges = placed.edges.map(item => ({ ...item, state: tracks.get(trackKey(item.edge)) ?? "todo" })).sort((a, b) => order[a.state] - order[b.state]);
  const line = runLineMetrics.nodeHeight / 2;
  const empty = boxes.map.routes.filter(route => route.stageIds.length === 0);

  return <section ref={section} className="run-line" aria-label="Stage line">
    <div className="run-line-scroll scroll" data-scroll-viewport>
      <div className="run-map" role="list" aria-label="The run's belt" style={{ width: placed.width, height: placed.height }}>
        <svg className="run-map-tracks" width={placed.width} height={placed.height} aria-hidden="true">
          {edges.map(({ edge, points, state }) => <path key={trackKey(edge)} className={`run-track is-${state}`} d={arcTrack(points, runLineMetrics.corner)} />)}
        </svg>
        {placed.routes.map(({ route, x, y }) => {
          // A route the run took is in ink; one it passed by is struck through; one still ahead is only soft.
          const look = taken.has(`${route.fork}:${route.route}`) ? " is-taken" : route.stageIds.some(id => words.get(id)?.state === "skip") ? " is-off" : "";
          return <span key={`${route.fork}:${route.route}`} className={`run-map-route${look}`} style={{ left: x, top: y }}>{route.name}</span>;
        })}
        {placed.nodes.map(({ node, x, y, width, height }) => {
          const word = words.get(node.id);
          const title = word?.title ?? node.id;
          const chip = word?.chip?.text;
          const status = chip ?? word?.details.join(" · ") ?? "";
          const selected = selectedStageId === node.id;
          const trigger = node.kind === "trigger";
          const look = trigger ? "is-trigger" : nodeLook(word?.state);
          const style = { left: x, top: y, width, height, "--line": `${line}px` } as CSSProperties;
          return <div key={node.id} role="listitem" className={`floor-box run-node ${look}${selected ? " is-selected" : ""}`} data-stage={node.id} style={style}>
            <i className="ln h" aria-hidden="true" />
            <span className="dot" aria-hidden="true" />
            {trigger
              ? <span className="fh"><b>{title}</b></span>
              : <button type="button" className="fh" aria-pressed={selected} aria-label={chip ? `${title} — ${chip}` : title} title={`${word?.caption ?? ""} · ${status}`}
                onClick={() => onSelectStage(node.id)}>
                <span className="n">{String(node.number).padStart(2, "0")}</span><b>{title}</b>
              </button>}
            <span className="fstatus">{status}</span>
          </div>;
        })}
      </div>
    </div>
    <div className="side run-line-header">
      <span>{metadata}</span>
      {empty.map(route => <span key={`${route.fork}:${route.route}`}>{route.name} route <b>{taken.has(`${route.fork}:${route.route}`) ? "taken, no stages" : "has no stages"}</b></span>)}
      <span className="run-line-caption">{boxes.caption}</span>
    </div>
  </section>;
}
