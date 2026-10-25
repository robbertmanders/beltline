import { describe, expect, it } from "vitest";
import type { RunViewResponse } from "../../api/wire.gen";
import { golden } from "../../test/golden";
import { placeBoxMap, runBoxMetrics } from "./boxGrid";

const mapOf = (name: string) => golden<RunViewResponse>(name).boxes.map;
const m = runBoxMetrics;

describe("placeBoxMap", () => {
  it("puts a straight belt on one row, a box and a gap per column, with straight tracks", () => {
    const placed = placeBoxMap(mapOf("views-run-straight.json"));
    expect(new Set(placed.nodes.map(node => node.y))).toEqual(new Set([m.padTop]));
    placed.nodes.forEach((node, index) => expect(node.x).toBe(m.padX + index * (m.nodeWidth + m.gap)));
    for (const { points } of placed.edges) {
      expect(points).toHaveLength(2);
      expect(points[0]!.y).toBe(points[1]!.y);
    }
    expect(placed.width).toBe(m.padX * 2 + placed.nodes.length * (m.nodeWidth + m.gap));
    expect(placed.height).toBe(m.padTop + m.nodeHeight + m.padBottom);
  });

  it("keeps every box and track on the map, the highest row on the top padding", () => {
    for (const name of ["views-run-split.json", "views-run-nested.json", "views-run-forked.json"]) {
      const placed = placeBoxMap(mapOf(name));
      expect(Math.min(...placed.nodes.map(node => node.y))).toBeGreaterThanOrEqual(m.padTop);
      for (const node of placed.nodes) {
        expect(node.x + node.width).toBeLessThanOrEqual(placed.width);
        expect(node.y + node.height).toBeLessThanOrEqual(placed.height - m.padBottom);
      }
      for (const { points } of placed.edges) {
        for (const point of points) {
          expect(point.y).toBeGreaterThanOrEqual(0);
          expect(point.y).toBeLessThanOrEqual(placed.height);
        }
      }
      const spots = new Set(placed.nodes.map(node => `${node.x},${node.y}`));
      expect(spots.size).toBe(placed.nodes.length);
    }
  });

  it("runs each track from its box's right edge into the next box's middle, turning on the gaps between", () => {
    const placed = placeBoxMap(mapOf("views-run-forked.json"));
    const byId = new Map(placed.nodes.map(node => [node.node.id, node]));
    for (const { edge, points } of placed.edges) {
      const from = byId.get(edge.from)!, to = byId.get(edge.to)!;
      expect(points[0]).toEqual({ x: from.x + from.width, y: from.y + from.height / 2 });
      expect(points[points.length - 1]).toEqual({ x: to.x, y: to.y + to.height / 2 });
      // Every corner sits in the middle of a gap, never over a box.
      for (const corner of points.slice(1, -1)) {
        expect((corner.x - m.padX + m.gap / 2) % (m.nodeWidth + m.gap)).toBeCloseTo(0);
      }
    }
  });

  it("splits the line evenly, each route a row apart, its name on its lead of track", () => {
    const placed = placeBoxMap(mapOf("views-run-split.json"));
    const line = placed.nodes.find(node => node.node.id === "trigger")!;
    const rows = placed.routes.map(route => route.y);
    expect((Math.min(...rows) + Math.max(...rows)) / 2).toBe(line.y + m.nodeHeight / 2);
    expect(Math.max(...rows) - Math.min(...rows)).toBe(m.rowHeight);
    for (const { route, x, y } of placed.routes) {
      const first = placed.nodes.find(node => node.node.id === route.stageIds[0])!;
      expect(y).toBe(first.y + m.nodeHeight / 2);
      expect(x).toBeGreaterThan(first.x - m.nodeWidth - m.gap);
      expect(x).toBeLessThan(first.x);
    }
  });
});
