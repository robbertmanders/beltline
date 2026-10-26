export type Point = { x: number; y: number };

/** A coordinate as path data writes it: to two places, whole numbers bare. */
export function pathNumber(value: number): string {
  const rounded = Math.round(value * 100) / 100;
  return String(Object.is(rounded, -0) ? 0 : rounded);
}

const at = (point: Point) => `${pathNumber(point.x)} ${pathNumber(point.y)}`;

/** A straight line through the points. */
export function lineTrack(points: readonly Point[]): string {
  if (points.length === 0) return "";
  return `M ${at(points[0]!)}` + points.slice(1).map(point => ` L ${at(point)}`).join("");
}

/**
 * A line through the points whose inner corners turn on arcs of radius, as the Floor's and the run page's tracks do. Each arc meets
 * both stretches, and is shortened so it never runs past a neighbouring corner: a stretch shared with the next corner
 * gives each half, one ending the line gives it all. A point the line runs straight on through has no corner.
 */
export function arcTrack(points: readonly Point[], radius: number): string {
  if (points.length <= 1) return "";
  let d = `M ${at(points[0]!)}`;
  for (let index = 1; index < points.length; index += 1) {
    const corner = points[index]!;
    if (index >= points.length - 1) { d += ` L ${at(corner)}`; break; }
    const previous = points[index - 1]!, next = points[index + 1]!;
    const cross = (corner.x - previous.x) * (next.y - corner.y) - (corner.y - previous.y) * (next.x - corner.x);
    const dot = (corner.x - previous.x) * (next.x - corner.x) + (corner.y - previous.y) * (next.y - corner.y);
    // The angle the line turns through: an arc of radius r meets both stretches r·tan(turn/2) from the corner.
    const turn = Math.atan2(Math.abs(cross), dot);
    if (!(turn > 0.001 && turn < Math.PI - 0.001)) { d += ` L ${at(corner)}`; continue; }
    const tangent = Math.tan(turn / 2);
    const firstShare = index === 1 ? 1 : 2;
    const secondShare = index === points.length - 2 ? 1 : 2;
    const reach = Math.min(radius * tangent, distance(previous, corner) / firstShare, distance(corner, next) / secondShare);
    if (!(reach > 0)) { d += ` L ${at(corner)}`; continue; }
    const entry = towards(corner, previous, reach), exit = towards(corner, next, reach);
    const r = reach / tangent;
    d += ` L ${at(entry)} A ${pathNumber(r)} ${pathNumber(r)} 0 0 ${cross > 0 ? 1 : 0} ${at(exit)}`;
  }
  return d;
}

function distance(a: Point, b: Point): number {
  return Math.hypot(a.x - b.x, a.y - b.y);
}

/** The point the given distance from corner on the way back to origin. */
function towards(corner: Point, origin: Point, length: number): Point {
  const whole = distance(origin, corner);
  if (!(whole > 0)) return corner;
  return { x: corner.x + ((origin.x - corner.x) * length) / whole, y: corner.y + ((origin.y - corner.y) * length) / whole };
}
