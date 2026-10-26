import { pathNumber } from "../../theme/stages/tracks";

/** The Shipped sparkline drawn in its 300 × 150 space: the line, its area and the latest week's dot. */
export interface ShippedCurve {
  line: string;
  area: string;
  dotX: number;
  dotY: number;
}

/** The weeks the sparkline spans, as the daemon counts them and labels its axis. */
export const shippedWeeks = 12;

/**
 * Shipped runs per week, oldest first, as a smooth line across the chart: each of the twelve weeks a column from x 10
 * to 290, so the curve always matches the daemon's twelve-week axis labels, a count
 * of zero on the base at y 120, and the busiest week an eighth below the top of the 100-point rise so its dot clears
 * the latest label. Each stretch bends halfway between its two weeks.
 */
export function shippedCurve(weeks: readonly number[]): ShippedCurve {
  const n = pathNumber;
  const counts = weeks.length > 0 ? weeks : [0];
  const most = Math.max(...counts, 1);
  const scale = 100 / (most * 8 / 7);
  const step = 280 / (shippedWeeks - 1);
  const points = counts.map((value, index) => ({ x: 10 + index * step, y: 120 - value * scale }));
  let line = `M${n(points[0]!.x)} ${n(points[0]!.y)}`;
  for (let index = 1; index < points.length; index += 1) {
    const previous = points[index - 1]!, current = points[index]!;
    const middle = (previous.x + current.x) / 2;
    line += ` C${n(middle)} ${n(previous.y)} ${n(middle)} ${n(current.y)} ${n(current.x)} ${n(current.y)}`;
  }
  const last = points[points.length - 1]!;
  return { line, area: `${line} L${n(last.x)} 120 L10 120 Z`, dotX: last.x, dotY: last.y };
}
