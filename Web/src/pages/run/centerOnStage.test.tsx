import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BoxMap, RunBoxes as RunBoxesData } from "../../api/wire.gen";
import { RunBoxes } from "./RunBoxes";
import { centredScrollLeft } from "./centerOnStage";

/** Six boxes in a row, 160 apart, so the line is 940 wide and a 400-wide viewport can scroll 540. */
const lefts: Record<string, number> = { trigger: 2, s1: 162, s2: 322, s3: 482, s4: 642, s5: 802 };
const map: BoxMap = {
  columns: 6,
  nodes: Object.keys(lefts).map((id, index) => ({ id, kind: index === 0 ? "trigger" : "agent", number: index, column: index, row: 0, icon: "funnel" })),
  edges: [], routes: []
};
const boxes: RunBoxesData = {
  map,
  caption: "Now at s3",
  takenRoutes: [],
  nodes: Object.keys(lefts).map(id => ({ id, state: "idle", caption: "01 · Agent", title: id, details: [] })),
  edges: []
};

const viewportWidth = 400;
const contentWidth = 940;

describe("centredScrollLeft", () => {
  it("puts the point in the middle of the viewport", () => {
    expect(centredScrollLeft(546, viewportWidth, contentWidth)).toBe(346);
  });

  it("holds the position at the start and at the end", () => {
    expect(centredScrollLeft(68, viewportWidth, contentWidth)).toBe(0);
    expect(centredScrollLeft(866, viewportWidth, contentWidth)).toBe(540);
  });

  it("stays at the start when the content fits the viewport", () => {
    expect(centredScrollLeft(300, 400, 380)).toBe(0);
  });
});

describe("RunBoxes opening on the current stage", () => {
  const scrollPositions = new WeakMap<Element, number>();
  const original = Object.getOwnPropertyDescriptor(Element.prototype, "scrollLeft");

  beforeEach(() => {
    // jsdom has no layout, so the viewport and the boxes report the geometry the test needs.
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      if (this.hasAttribute("data-scroll-viewport")) return rect(0, viewportWidth);
      const stage = this.dataset.stage;
      const left = stage ? lefts[stage] : undefined;
      return left === undefined ? rect(0, 0) : rect(left, 132);
    });
    Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return this.hasAttribute("data-scroll-viewport") ? viewportWidth : 0; } });
    Object.defineProperty(HTMLElement.prototype, "scrollWidth", { configurable: true, get() { return this.hasAttribute("data-scroll-viewport") ? contentWidth : 0; } });
    Object.defineProperty(Element.prototype, "scrollLeft", {
      configurable: true,
      get() { return scrollPositions.get(this) ?? 0; },
      set(value: number) { scrollPositions.set(this, value); }
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
    Reflect.deleteProperty(HTMLElement.prototype, "scrollWidth");
    if (original) Object.defineProperty(Element.prototype, "scrollLeft", original);
  });

  function viewportOf(container: HTMLElement) {
    return container.querySelector<HTMLElement>("[data-scroll-viewport]")!;
  }

  it("centres the current stage between the start and the end", () => {
    const { container } = render(<RunBoxes boxes={boxes} metadata="Belt" selectedStageId="s1" onSelectStage={vi.fn()} opening={{ run: "run-1", stage: "s3" }} />);
    expect(viewportOf(container).scrollLeft).toBe(348);
  });

  it("opens at the start when the current stage is near the beginning", () => {
    const { container } = render(<RunBoxes boxes={boxes} metadata="Belt" selectedStageId="s1" onSelectStage={vi.fn()} opening={{ run: "run-1", stage: "trigger" }} />);
    expect(viewportOf(container).scrollLeft).toBe(0);
  });

  it("opens at the end when the current stage is near the end", () => {
    const { container } = render(<RunBoxes boxes={boxes} metadata="Belt" selectedStageId="s1" onSelectStage={vi.fn()} opening={{ run: "run-1", stage: "s5" }} />);
    expect(viewportOf(container).scrollLeft).toBe(540);
  });

  it("does not move the scroll again when the selected stage changes", () => {
    const { container, rerender } = render(<RunBoxes boxes={boxes} metadata="Belt" selectedStageId="s3" onSelectStage={vi.fn()} opening={{ run: "run-1", stage: "s3" }} />);
    viewportOf(container).scrollLeft = 100;
    rerender(<RunBoxes boxes={boxes} metadata="Belt" selectedStageId="s1" onSelectStage={vi.fn()} opening={{ run: "run-1", stage: "s3" }} />);
    expect(viewportOf(container).scrollLeft).toBe(100);
  });
});

function rect(left: number, width: number): DOMRect {
  return { left, right: left + width, top: 0, bottom: 52, width, height: 52, x: left, y: 0, toJSON: () => ({}) } as DOMRect;
}
