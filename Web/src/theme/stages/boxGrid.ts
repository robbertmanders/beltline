import type { BoxMap, BoxMapEdge, BoxMapNode, BoxMapRoutePill } from "../../api/wire.gen";

type Point = { x: number; y: number };

/** The sizes a box map's grid is drawn at, in points. */
export interface BoxGridMetrics {
  nodeWidth: number;
  nodeHeight: number;
  /** The space between two columns, where the tracks run and turn. */
  gap: number;
  /** From one route's row to the next. */
  rowHeight: number;
  padX: number;
  /** Room above the first row, for the status chips. */
  padTop: number;
  padBottom: number;
  corner: number;
}

/** The run page: short boxes with a status chip above each. */
export const runBoxMetrics: BoxGridMetrics = { nodeWidth: 148, nodeHeight: 52, gap: 60, rowHeight: 100, padX: 2, padTop: 34, padBottom: 4, corner: 10 };

export interface PlacedBoxNode { node: BoxMapNode; x: number; y: number; width: number; height: number }
export interface PlacedBoxEdge { edge: BoxMapEdge; points: Point[] }
export interface PlacedBoxRoute { route: BoxMapRoutePill; x: number; y: number }

/** A box map in points: every box, every track's corners and every route name. */
export interface PlacedBoxMap {
  width: number;
  height: number;
  nodes: PlacedBoxNode[];
  edges: PlacedBoxEdge[];
  routes: PlacedBoxRoute[];
}

const near = (a: number, b: number) => Math.abs(a - b) < 0.5;

/**
 * The daemon's grid in points. Each column is a box wide with a gap in front of the next; each row a row height
 * apart, the highest box or route sitting on the top padding. A track leaves its box's right edge, turns its corners
 * on the middle of the gaps the daemon names, and runs into the next box's left edge. A route's name sits on its
 * lead of track, halfway from its fork's spine to its first box.
 */
export function placeBoxMap(map: BoxMap, metrics: BoxGridMetrics = runBoxMetrics): PlacedBoxMap {
  const { nodeWidth, nodeHeight, gap, rowHeight, padX, padTop, padBottom } = metrics;
  const rows = [...map.nodes.map(node => node.row), ...map.routes.map(route => route.row)];
  const top = rows.length > 0 ? Math.min(...rows) : 0;
  const bottom = rows.length > 0 ? Math.max(...rows) : 0;
  const left = (column: number) => padX + column * (nodeWidth + gap);
  const spine = (column: number) => left(column) - gap / 2;
  const middle = (row: number) => padTop + nodeHeight / 2 + (row - top) * rowHeight;

  const nodes = map.nodes.map(node => ({ node, x: left(node.column), y: middle(node.row) - nodeHeight / 2, width: nodeWidth, height: nodeHeight }));
  const byId = new Map(nodes.map(placed => [placed.node.id, placed]));
  const edges: PlacedBoxEdge[] = [];
  for (const edge of map.edges) {
    const from = byId.get(edge.from), to = byId.get(edge.to);
    if (!from || !to) continue;
    const points: Point[] = [{ x: from.x + from.width, y: from.y + from.height / 2 }];
    const add = (point: Point) => {
      const last = points[points.length - 1]!;
      if (!near(last.x, point.x) || !near(last.y, point.y)) points.push(point);
    };
    for (const turn of edge.via) add({ x: spine(turn.column), y: middle(turn.row) });
    add({ x: to.x, y: to.y + to.height / 2 });
    edges.push({ edge, points });
  }
  const routes = map.routes.map(route => ({ route, x: (spine(route.column) + left(route.column + 1)) / 2, y: middle(route.row) }));
  return {
    width: padX * 2 + Math.max(map.columns, 1) * (nodeWidth + gap),
    height: (bottom - top) * rowHeight + nodeHeight + padTop + padBottom,
    nodes,
    edges,
    routes
  };
}
