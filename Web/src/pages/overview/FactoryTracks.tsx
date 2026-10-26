import { floorMetrics, nodeKey, type FloorLayout } from "./floorLayout";
import { focusOpacity } from "./focus";

/**
 * The tracks, the return lanes and their names between the Floor's boxes. Each track is drawn once; a focused belt's
 * tracks change colour in place and are painted after the rest, so they are never stroked twice. Its lanes stay in
 * full view while the rest dim.
 */
export function FactoryTracks({ layout, focus }: { layout: FloorLayout; focus: string | null }) {
  const isFocused = (belts: readonly string[]) => focus !== null && belts.includes(focus);
  const tracks = [...layout.tracks.filter(({ stretch }) => !isFocused(stretch.belts)), ...layout.tracks.filter(({ stretch }) => isFocused(stretch.belts))];
  return <>
    <svg className="factory-tracks" width={layout.width} height={layout.height} aria-hidden="true" focusable="false">
      {tracks.map(({ stretch, path }) => <path key={`${nodeKey(stretch.from)}>${nodeKey(stretch.to)}`} d={path}
        className={isFocused(stretch.belts) ? "factory-track factory-track-focus" : "factory-track"}
        data-belts={stretch.belts.join(" ")} strokeWidth={floorMetrics.thickness} />)}
      {layout.lanes.map(({ lane, path, arrow }) => <g key={`lane:${nodeKey(lane.from)}>${nodeKey(lane.to)}`} className="factory-lane"
        opacity={focusOpacity(lane.belts, focus)} data-belts={lane.belts.join(" ")}>
        <path d={path} className="factory-lane-line" />
        <path d={arrow} className="factory-lane-arrow" />
      </g>)}
    </svg>
    {layout.lanes.map(({ lane, labelX, labelY }) => <span key={`lane-label:${nodeKey(lane.from)}>${nodeKey(lane.to)}`} className="factory-lane-label"
      style={{ left: labelX, top: labelY, opacity: focusOpacity(lane.belts, focus) }}>{lane.label}</span>)}
  </>;
}
