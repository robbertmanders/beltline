import type { FactoryTrackNode, FactoryTrackTrack, FloorTrack, FloorTrackReturn, OverviewFloorBox } from "../../api/wire.gen";
import { arcTrack, lineTrack, pathNumber, type Point } from "../../theme/stages/tracks";

/** A node's key: its kind and value, as the daemon names it. */
export function nodeKey(node: FactoryTrackNode): string {
  return `${node.kind}:${node.value}`;
}

/** The sizes the Floor is placed with, in points. */
export const floorMetrics = {
  /** The belts' entrance cards on the left. */
  entranceWidth: 220,
  entranceHeight: 112,
  /** The narrowest column: a narrower page scrolls the Floor rather than squeezing it. */
  minColumn: 216,
  /** A stage box, the run page's size. */
  boxWidth: 148,
  boxHeight: 52,
  /** The room above a box for its status. */
  statusHeight: 22,
  /** One run tag's row, and the gap above the tags. */
  tagHeight: 23,
  tagsGap: 6,
  waypoint: 14,
  /** The space between two nodes in a layer. */
  gap: 22,
  /** Every track's width, however many belts share it: a shared stretch is told apart by its colour, not a heavier line. */
  thickness: 8,
  corner: 10,
  /** The space between lanes that go back under the same stages. */
  lane: 26,
  /** Room between a stage's bottom and the lanes under it, beyond the track's half width, so a lane's label clears it. */
  clearance: 12
} as const;

/** Where the track runs, from the top of a stage's cell: the middle of its box, under its status. */
const trackOffset = floorMetrics.statusHeight + floorMetrics.boxHeight / 2;
/** Slack for the spacing between lanes, so a step that rounds a hair short never matches the same lane again. */
const laneTolerance = 1e-6;

/** A node as placed: X is the centre of its column, Anchor the height its track runs at. */
export interface FloorPlaced { node: FactoryTrackNode; x: number; top: number; height: number; anchor: number }
export interface FloorStroke { stretch: FactoryTrackTrack; path: string }
/** A dotted lane back to an earlier stage: down at Right, back along LabelY and up into the earlier stage at Left. */
export interface FloorLane { lane: FloorTrackReturn; left: number; right: number; path: string; arrow: string; labelX: number; labelY: number }

/** Where everything on the Floor goes, in points. */
export interface FloorLayout {
  width: number;
  height: number;
  column: number;
  placed: FloorPlaced[];
  tracks: FloorStroke[];
  lanes: FloorLane[];
}

/** A box's tag rows: its shown tags, and one more when some are left over. */
export function floorTagRows(box: Pick<OverviewFloorBox, "chips" | "more">): number {
  return box.chips.length + (box.more > 0 ? 1 : 0);
}

/** A box's cell height: its status, its box and its run tags. */
export function floorCellHeight(box: Pick<OverviewFloorBox, "chips" | "more">): number {
  const rows = floorTagRows(box);
  return floorMetrics.statusHeight + floorMetrics.boxHeight + (rows > 0 ? floorMetrics.tagsGap + rows * floorMetrics.tagHeight : 0);
}

/** The box a placed cell draws: its column's centre is the box's middle, and its top is the cell's. */
export function floorBoxFrame(placed: FloorPlaced): { x: number; y: number; width: number; height: number } {
  return { x: placed.x - floorMetrics.boxWidth / 2, y: placed.top, width: floorMetrics.boxWidth, height: placed.height };
}

/** The width the Floor needs at its narrowest column. */
export function floorMinimumWidth(track: FloorTrack): number {
  return floorMetrics.entranceWidth + Math.max(track.layers.length - 1, 0) * floorMetrics.minColumn;
}

/**
 * The Floor laid out at width (or at its narrowest column when width is missing or too narrow). Each layer keeps its
 * order and spacing and sits as near as it can to its tracks; the tracks run straight out of one node and into the
 * next, turning in the gap where they change rows; and each return lane drops below the stages it passes under.
 */
export function layOutFloor(track: FloorTrack, boxes: readonly OverviewFloorBox[], width?: number): FloorLayout {
  const m = floorMetrics;
  const byId = new Map(boxes.map(box => [box.id, box]));
  const heights = new Map<string, number>();
  for (const layer of track.layers) {
    for (const node of layer) {
      const box = node.kind === "stage" ? byId.get(String(node.value)) : undefined;
      heights.set(nodeKey(node), node.kind === "entrance" ? m.entranceHeight
        : node.kind === "waypoint" ? m.waypoint
        : box ? floorCellHeight(box) : trackOffset);
    }
  }
  const height = (node: FactoryTrackNode) => heights.get(nodeKey(node)) ?? 0;
  const offset = (node: FactoryTrackNode) => node.kind === "stage" ? trackOffset : height(node) / 2;
  const layers = track.layers.length;
  const column = width === undefined ? m.minColumn : Math.max(m.minColumn, (width - m.entranceWidth) / Math.max(layers - 1, 1));
  const tops = floorTops(track, height, offset);

  const placed: FloorPlaced[] = [];
  const positions = new Map<string, FloorPlaced>();
  track.layers.forEach((layer, index) => {
    const x = index === 0 ? m.entranceWidth / 2 : m.entranceWidth + (index - 0.5) * column;
    for (const node of layer) {
      const top = tops.get(nodeKey(node)) ?? 0;
      const box = { node, x, top, height: height(node), anchor: top + offset(node) };
      placed.push(box);
      positions.set(nodeKey(node), box);
    }
  });
  const lanes = floorLanes(track, placed, positions, column);
  const bottom = placed.reduce((lowest, box) => Math.max(lowest, box.top + box.height), 0);
  const lowestLane = lanes.reduce((lowest, lane) => Math.max(lowest, lane.y), 0);
  return {
    width: m.entranceWidth + Math.max(layers - 1, 0) * column,
    height: Math.max(bottom, lowestLane + 10),
    column,
    placed,
    tracks: track.tracks.map(stretch => ({ stretch, path: floorTrackPath(stretch, positions) })),
    lanes: lanes.map(lane => ({ lane: lane.stretch, left: lane.left, right: lane.right, ...lanePath(lane), labelX: (lane.left + lane.right) / 2, labelY: lane.y }))
  };
}

/**
 * Every node's top. Each layer starts stacked in order; then, sweeping forwards and back a few times, each layer moves
 * as near as it can to the average height of the tracks into it (forwards) or out of it (back), without overlapping.
 */
function floorTops(track: FloorTrack, height: (node: FactoryTrackNode) => number, offset: (node: FactoryTrackNode) => number): Map<string, number> {
  const top = new Map<string, number>();
  for (const layer of track.layers) {
    let y = 0;
    for (const node of layer) {
      top.set(nodeKey(node), y);
      y += height(node) + floorMetrics.gap;
    }
  }
  const count = track.layers.length;
  for (let pass = 0; pass < 5 && count > 1; pass += 1) {
    const forwards = pass % 2 === 0;
    const order: number[] = [];
    if (forwards) for (let l = 1; l < count; l += 1) order.push(l);
    else for (let l = count - 2; l >= 0; l -= 1) order.push(l);
    for (const l of order) {
      const layer = track.layers[l]!;
      const desired = layer.map(node => {
        const key = nodeKey(node);
        const near: number[] = [];
        for (const stretch of track.tracks) {
          const other = forwards ? (nodeKey(stretch.to) === key ? stretch.from : null) : (nodeKey(stretch.from) === key ? stretch.to : null);
          if (other) near.push((top.get(nodeKey(other)) ?? 0) + offset(other));
        }
        const want = near.length === 0 ? (top.get(key) ?? 0) + offset(node) : near.reduce((sum, value) => sum + value, 0) / near.length;
        return want - offset(node);
      });
      spread(desired, layer.map(height)).forEach((y, index) => top.set(nodeKey(layer[index]!), y));
    }
  }
  const least = top.size > 0 ? Math.min(...top.values()) : 0;
  return new Map([...top].map(([key, value]) => [key, value - least]));
}

/** Positions as near the desired ones as they can be without overlapping: overlapping neighbours move together, centred on where they want to be. */
export function spread(desired: readonly number[], heights: readonly number[], gap: number = floorMetrics.gap): number[] {
  type Block = { first: number; last: number; top: number };
  const blockTop = (first: number, last: number) => {
    let offset = 0, sum = 0;
    for (let i = first; i <= last; i += 1) {
      sum += desired[i]! - offset;
      offset += heights[i]! + gap;
    }
    return sum / (last - first + 1);
  };
  const bottom = (block: Block) => {
    let total = 0;
    for (let i = block.first; i <= block.last; i += 1) total += heights[i]! + gap;
    return block.top + total;
  };
  const blocks: Block[] = [];
  desired.forEach((want, i) => {
    blocks.push({ first: i, last: i, top: want });
    while (blocks.length > 1 && blocks[blocks.length - 1]!.top < bottom(blocks[blocks.length - 2]!)) {
      const last = blocks.pop()!;
      const first = blocks.pop()!.first;
      blocks.push({ first, last: last.last, top: blockTop(first, last.last) });
    }
  });
  const out: number[] = [];
  for (const block of blocks) {
    let y = block.top;
    for (let i = block.first; i <= block.last; i += 1) {
      out.push(y);
      y += heights[i]! + gap;
    }
  }
  return out;
}

/** A return lane before it is drawn: its stretch, its horizontal extent, the heights it leaves and joins at, and its row. */
interface LanePlan { stretch: FloorTrackReturn; right: number; left: number; from: number; to: number; y: number }

/** Each return lane, just below the stages it passes under, in the first gap it fits and below the lanes it overlaps. */
function floorLanes(track: FloorTrack, placed: readonly FloorPlaced[], positions: ReadonlyMap<string, FloorPlaced>, column: number): LanePlan[] {
  const half = floorMetrics.thickness / 2;
  const stages = placed.filter(box => box.node.kind === "stage");
  const lanes: LanePlan[] = [];
  for (const stretch of track.returns) {
    const a = positions.get(nodeKey(stretch.from)), b = positions.get(nodeKey(stretch.to));
    if (!a || !b) continue;
    const right = a.x + column / 2, left = b.x - column / 2;
    const under = stages.filter(cell => cell.x >= b.x && cell.x <= a.x);
    const overlapping = lanes.filter(lane => lane.left < right && left < lane.right);
    const below = half + floorMetrics.clearance;
    let y = Math.max(a.anchor, b.anchor) + half;
    let moved = true;
    while (moved) {
      moved = false;
      for (const cell of under) {
        if (y > cell.top - half && y < cell.top + cell.height + below) {
          y = cell.top + cell.height + below;
          moved = true;
        }
      }
      for (const lane of overlapping) {
        if (Math.abs(y - lane.y) < floorMetrics.lane - laneTolerance) {
          y = lane.y + floorMetrics.lane;
          moved = true;
        }
      }
    }
    lanes.push({ stretch, right, left, from: a.anchor, to: b.anchor, y });
  }
  return lanes;
}

/** Where a track runs at a node: an entrance card's right part, or the node's anchor. */
function trackPoint(node: FactoryTrackNode, positions: ReadonlyMap<string, FloorPlaced>): Point | null {
  const at = positions.get(nodeKey(node));
  if (!at) return null;
  if (node.kind === "entrance") return { x: at.x - 10 + (floorMetrics.entranceWidth - 20) / 2, y: at.anchor };
  return { x: at.x, y: at.anchor };
}

/** Straight out of one node and into the next, turning a rounded corner each way halfway across the gap where it changes rows. */
function floorTrackPath(stretch: FactoryTrackTrack, positions: ReadonlyMap<string, FloorPlaced>): string {
  const a = trackPoint(stretch.from, positions), b = trackPoint(stretch.to, positions);
  if (!a || !b) return "";
  if (!(Math.abs(a.y - b.y) > 0.5)) return lineTrack([a, b]);
  // A box covers the track at its middle, so the track starts at each box's edge and turns in the gap.
  const halfBox = (node: FactoryTrackNode) => node.kind === "stage" ? floorMetrics.boxWidth / 2 : 0;
  const x = (a.x + halfBox(stretch.from) + b.x - halfBox(stretch.to)) / 2;
  const radius = Math.max(0, Math.min(floorMetrics.corner, Math.abs(b.y - a.y) / 2, x - a.x, b.x - x));
  return arcTrack([a, { x, y: a.y }, { x, y: b.y }, b], radius);
}

/** A lane's dotted path, and the arrow head that marks where it ends. */
function lanePath(lane: LanePlan): { path: string; arrow: string } {
  const radius = Math.min(floorMetrics.corner * 2, (lane.y - lane.from) / 2, (lane.y - lane.to) / 2, (lane.right - lane.left) / 4);
  const tip = { x: lane.left + radius + 12, y: lane.to };
  const body = arcTrack([
    { x: lane.right - radius, y: lane.from }, { x: lane.right, y: lane.from }, { x: lane.right, y: lane.y },
    { x: lane.left, y: lane.y }, { x: lane.left, y: lane.to }, tip
  ], radius);
  const n = pathNumber;
  return {
    path: `${body} L ${n(tip.x - 6)} ${n(tip.y)}`,
    arrow: `M ${n(tip.x)} ${n(tip.y)} L ${n(tip.x - 9)} ${n(tip.y - 6)} L ${n(tip.x - 9)} ${n(tip.y + 6)} Z`
  };
}
