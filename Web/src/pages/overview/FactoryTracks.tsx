import type { FactoryTrackNode, FloorLayout } from "../../api/wire.gen";
import { focusOpacity } from "./focus";

/** A node's key in the daemon's layout, which matches the node's kind and value. */
export function nodeKey(node: FactoryTrackNode): string {
  return `${node.kind}:${node.value}`;
}

/**
 * The tracks, the return lanes and their names, as the daemon drew them between the Floor's boxes. Each track is drawn once, at the daemon's width; a focused belt's tracks change colour in place and are
 * painted after the rest, so they are never stroked twice. Its lanes stay in full view while the rest dim.
 */
export function FactoryTracks({ layout, focus }: { layout: FloorLayout; focus: string | null }) {
  const isFocused = (belts: readonly string[]) => focus !== null && belts.includes(focus);
  const tracks = [...layout.tracks.filter(stroke => !isFocused(stroke.belts)), ...layout.tracks.filter(stroke => isFocused(stroke.belts))];
  return <>
    <svg className="factory-tracks" width={layout.width} height={layout.height} aria-hidden="true" focusable="false">
      {tracks.map(stroke => <path key={`${nodeKey(stroke.from)}>${nodeKey(stroke.to)}`} d={stroke.path}
        className={isFocused(stroke.belts) ? "factory-track factory-track-focus" : "factory-track"}
        data-belts={stroke.belts.join(" ")} strokeWidth={stroke.width} />)}
      {layout.lanes.map(lane => <g key={`lane:${nodeKey(lane.from)}>${nodeKey(lane.to)}`} className="factory-lane"
        opacity={focusOpacity(lane.belts, focus)} data-belts={lane.belts.join(" ")}>
        <path d={lane.path} className="factory-lane-line" />
        <path d={lane.arrow} className="factory-lane-arrow" />
      </g>)}
    </svg>
    {layout.lanes.map(lane => <span key={`lane-label:${nodeKey(lane.from)}>${nodeKey(lane.to)}`} className="factory-lane-label"
      style={{ left: lane.labelX, top: lane.labelY, opacity: focusOpacity(lane.belts, focus) }}>{lane.label}</span>)}
  </>;
}
