import { describe, expect, it } from "vitest";
import type { FactoryTrackNode, FloorTrack, OverviewFloorBox, OverviewResponse } from "../../api/wire.gen";
import { golden } from "../../test/golden";
import { floorCellHeight, floorMetrics, floorMinimumWidth, floorTagRows, layOutFloor, nodeKey, placeFloor, planFloor, spread } from "./floorLayout";

const floor = () => golden<OverviewResponse>("views-overview.json").floor;
const m = floorMetrics;
const entrance = (value: string): FactoryTrackNode => ({ kind: "entrance", value });
const stage = (value: string): FactoryTrackNode => ({ kind: "stage", value });
const box = (id: string, runs = 0): OverviewFloorBox => ({
  id, name: id, caption: "Agent", icon: "chip", tint: "grey", belts: [], status: { text: "Idle", look: "plain" }, runCount: runs,
  chips: Array.from({ length: Math.min(runs, 3) }, (_, index) => ({ runId: `${id}-${index}`, ref: "#1", state: "running", paused: false })),
  more: Math.max(runs - 3, 0)
});

/**
 * Three belts that reach Build, Test and Verify in other orders: a goes Build, Test, Verify; b goes Verify then back
 * to Build; c goes Verify then back to Test. The two returns overlap under the stages they pass.
 */
const returning: FloorTrack = {
  layers: [[entrance("a"), entrance("b"), entrance("c")], [stage("build")], [stage("test")], [stage("verify")]],
  tracks: [
    { from: entrance("a"), to: stage("build"), belts: ["a"] },
    { from: stage("build"), to: stage("test"), belts: ["a"] },
    { from: stage("test"), to: stage("verify"), belts: ["a"] }
  ],
  returns: [
    { from: stage("verify"), to: stage("build"), belts: ["b"], label: "B" },
    { from: stage("verify"), to: stage("test"), belts: ["c"], label: "C" }
  ]
};
const returningBoxes = [box("build"), box("test"), box("verify")];

describe("floor cells", () => {
  it("hold their status and box, then up to three tags and a count of the rest", () => {
    expect(floorCellHeight(box("none"))).toBe(m.statusHeight + m.boxHeight);
    expect(floorTagRows(box("three", 3))).toBe(3);
    expect(floorTagRows(box("four", 4))).toBe(4);
    expect(floorTagRows(box("nine", 9))).toBe(4);
    expect(floorCellHeight(box("three", 3))).toBe(m.statusHeight + m.boxHeight + m.tagsGap + 3 * m.tagHeight);
    expect(floorCellHeight(box("nine", 9))).toBe(floorCellHeight(box("four", 4)));
  });
});

describe("spread", () => {
  it("keeps the order and centres neighbours that overlap", () => {
    expect(spread([0, 100], [20, 20], 10)).toEqual([0, 100]);
    expect(spread([50, 50], [20, 20], 10)).toEqual([35, 65]);
  });
});

describe("layOutFloor", () => {
  it("places every node once, inside the layout, at the narrowest column", () => {
    const { track, boxes } = floor();
    const layout = layOutFloor(track, boxes);
    expect(layout.column).toBe(m.minColumn);
    expect(layout.width).toBe(floorMinimumWidth(track));
    expect(layout.placed).toHaveLength(track.layers.flat().length);
    expect(new Set(layout.placed.map(placed => nodeKey(placed.node))).size).toBe(layout.placed.length);
    for (const placed of layout.placed) {
      expect(placed.top).toBeGreaterThanOrEqual(0);
      expect(placed.top + placed.height).toBeLessThanOrEqual(layout.height);
    }
    expect(layout.tracks).toHaveLength(track.tracks.length);
    for (const { path } of layout.tracks) expect(path).toMatch(/^M /);
  });

  it("sizes each stage's cell by its tags and runs its track through its box's middle", () => {
    const { track, boxes } = floor();
    const layout = layOutFloor(track, boxes);
    for (const item of boxes) {
      const placed = layout.placed.find(at => nodeKey(at.node) === `stage:${item.id}`)!;
      expect(placed.height).toBe(floorCellHeight(item));
      expect(placed.anchor).toBe(placed.top + m.statusHeight + m.boxHeight / 2);
    }
  });

  it("never overlaps two cells in one layer", () => {
    const { track, boxes } = floor();
    const layout = layOutFloor(track, boxes);
    const columns = new Map<number, typeof layout.placed>();
    for (const placed of layout.placed) columns.set(placed.x, [...(columns.get(placed.x) ?? []), placed]);
    for (const cells of columns.values()) {
      const sorted = [...cells].sort((a, b) => a.top - b.top);
      for (let index = 1; index < sorted.length; index += 1) {
        expect(sorted[index - 1]!.top + sorted[index - 1]!.height).toBeLessThanOrEqual(sorted[index]!.top + 1e-9);
      }
    }
  });

  it("spreads its columns over a wider page and keeps the narrowest column on a narrower one", () => {
    const { track, boxes } = floor();
    const minimum = floorMinimumWidth(track);
    const wide = layOutFloor(track, boxes, minimum + 400);
    expect(wide.column).toBeGreaterThan(m.minColumn);
    expect(wide.width).toBeCloseTo(minimum + 400);
    expect(layOutFloor(track, boxes, minimum - 400)).toEqual(layOutFloor(track, boxes));
  });

  it("keeps every node's height and top across widths, so a resize only places the plan again", () => {
    const plan = planFloor(returning, returningBoxes);
    const narrow = placeFloor(returning, plan, 600), wide = placeFloor(returning, plan, 1600);
    expect(wide).toEqual(layOutFloor(returning, returningBoxes, 1600));
    for (const cell of narrow.placed) {
      const key = nodeKey(cell.node);
      expect(narrow.byKey.get(key)).toBe(cell);
      expect(wide.byKey.get(key)).toMatchObject({ top: cell.top, height: cell.height, anchor: cell.anchor });
    }
  });

  it("is empty but finite with no belts", () => {
    const layout = layOutFloor({ layers: [], tracks: [], returns: [] }, []);
    expect(layout.placed).toHaveLength(0);
    expect(layout.tracks).toHaveLength(0);
    expect(layout.lanes).toHaveLength(0);
    expect(Number.isFinite(layout.height) && layout.height >= 0).toBe(true);
  });

  it("runs each return lane under the boxes it passes and into the earlier one, below the lanes it overlaps", () => {
    const layout = layOutFloor(returning, returningBoxes);
    expect(layout.lanes).toHaveLength(2);
    const at = (node: FactoryTrackNode) => layout.placed.find(placed => nodeKey(placed.node) === nodeKey(node))!;
    // A lane label is a 16 point pill centred on its lane, so it needs 8 points of air above and below.
    const labelHalf = 8;
    for (const lane of layout.lanes) {
      const from = at(lane.lane.from), to = at(lane.lane.to);
      expect(lane.right).toBe(from.x + layout.column / 2);
      expect(lane.left).toBe(to.x - layout.column / 2);
      expect(lane.path).toMatch(/^M /);
      expect(lane.arrow).toMatch(/^M .* Z$/);
      for (const cell of layout.placed) {
        if (cell.node.kind !== "stage" || cell.x < to.x || cell.x > from.x) continue;
        expect(lane.labelY - labelHalf).toBeGreaterThanOrEqual(cell.top + cell.height + labelHalf);
      }
      expect(lane.labelY).toBeLessThan(layout.height);
    }
    expect(Math.abs(layout.lanes[0]!.labelY - layout.lanes[1]!.labelY)).toBeGreaterThanOrEqual(m.lane - 1e-6);
  });
});
