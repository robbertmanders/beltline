import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { BoxMap, RunBoxes as RunBoxesData } from "../../api/wire.gen";
import { RunBoxes } from "./RunBoxes";

/** A three-box belt: the trigger, an agent stage at work and an approval waiting for you. */
const map: BoxMap = {
  columns: 3,
  nodes: [
    { id: "trigger", kind: "trigger", number: 0, column: 0, row: 0, icon: "bolt" },
    { id: "triage", kind: "agent", number: 1, column: 1, row: 0, icon: "funnel", tint: "blue" },
    { id: "approve", kind: "approval", number: 2, column: 2, row: 0, icon: "approve", tint: "gold" }
  ],
  edges: [
    { from: "trigger", to: "triage", via: [] },
    { from: "triage", to: "approve", via: [] }
  ],
  routes: []
};

const boxes: RunBoxesData = {
  map,
  caption: "Now at Triage",
  takenRoutes: [],
  nodes: [
    { id: "trigger", state: "idle", caption: "Trigger", title: "#139", details: ["github issue"] },
    { id: "triage", state: "running", caption: "01 · Agent", title: "Triage", details: ["pi"], chip: { text: "Running", look: "running" } },
    { id: "approve", state: "needs", caption: "02 · Approval", title: "Approve", details: ["you"], chip: { text: "Waiting for you", look: "waiting" }, dot: "needs" }
  ],
  edges: [
    { from: "trigger", to: "triage", state: "done" },
    { from: "triage", to: "approve", state: "todo" }
  ]
};

describe("RunBoxes", () => {
  it("draws the trigger dark with its subtitle and each stage with its caption, detail and chip", () => {
    render(<RunBoxes boxes={boxes} metadata="Belt v1 · 2 stages" selectedStageId="triage" onSelectStage={vi.fn()} />);
    expect(screen.getByText("Belt v1 · 2 stages")).toBeTruthy();
    expect(screen.getByText("Now at Triage")).toBeTruthy();
    expect(screen.getByText("github issue")).toBeTruthy();
    expect(screen.getByText("01 · Agent")).toBeTruthy();
    expect(screen.getByText("pi")).toBeTruthy();
    expect(screen.getByText("02 · Approval")).toBeTruthy();
    expect(screen.getByText("you")).toBeTruthy();
    // The waiting stage's chip is the solid one, and its corner dot is the !.
    expect(screen.getByText("Waiting for you").className).toContain("paper-chip--need");
    expect(screen.getByText("!")).toBeTruthy();
    expect(screen.getByText("Running").className).toContain("paper-status--running");
    expect(document.querySelector(".stage-box--trigger")).toBeTruthy();
    expect(document.querySelector(".stage-box--selected")?.textContent).toContain("Triage");
  });

  it("selects a stage when its box is clicked, and ignores the trigger", () => {
    const onSelectStage = vi.fn();
    render(<RunBoxes boxes={boxes} metadata="Belt v1" selectedStageId="triage" onSelectStage={onSelectStage} />);
    fireEvent.click(screen.getByRole("button", { name: "Approve — Waiting for you" }));
    expect(onSelectStage).toHaveBeenLastCalledWith("approve");
    fireEvent.click(screen.getByRole("button", { name: "#139" }));
    expect(onSelectStage).toHaveBeenCalledTimes(1);
  });

  it("places each box in its column and row and states each track", () => {
    const { container } = render(<RunBoxes boxes={boxes} metadata="Belt v1" selectedStageId={null} onSelectStage={vi.fn()} />);
    const triage = screen.getByRole("button", { name: "Triage — Running" });
    // The second column: the padding, then a box and a gap.
    expect(triage.style.left).toBe("210px");
    expect(triage.style.top).toBe("34px");
    expect(container.querySelectorAll(".box-track--done")).toHaveLength(1);
    expect(container.querySelectorAll(".box-track--todo")).toHaveLength(1);
  });

  it("states each route's track on its own, even where two routes join the same boxes", () => {
    const twoRoutes: RunBoxesData = {
      ...boxes,
      map: {
        ...map,
        edges: [
          { from: "triage", to: "approve", via: [], route: "work" },
          { from: "triage", to: "approve", via: [], route: "skip" }
        ]
      },
      edges: [
        { from: "triage", to: "approve", state: "done", route: "work" },
        { from: "triage", to: "approve", state: "skip", route: "skip" }
      ]
    };
    const { container } = render(<RunBoxes boxes={twoRoutes} metadata="Belt v1" selectedStageId={null} onSelectStage={vi.fn()} />);
    expect(container.querySelectorAll(".box-track--done")).toHaveLength(1);
    expect(container.querySelectorAll(".box-track--skip")).toHaveLength(1);
    expect(container.querySelectorAll(".box-track--todo")).toHaveLength(0);
  });

  it("draws a route's name off when the run did not take it", () => {
    const withRoute: RunBoxesData = {
      ...boxes,
      map: { ...map, routes: [{ fork: "size", route: "simple", name: "Simple", column: 1, row: 1, stageIds: ["triage"], feeds: false }] },
      takenRoutes: []
    };
    render(<RunBoxes boxes={withRoute} metadata="Belt v1" selectedStageId={null} onSelectStage={vi.fn()} />);
    expect(screen.getByText("Simple").className).toContain("box-map__route--off");
    render(<RunBoxes boxes={{ ...withRoute, takenRoutes: ["size:simple"] }} metadata="Belt v1" selectedStageId={null} onSelectStage={vi.fn()} />);
    expect(screen.getAllByText("Simple")[1]?.className).not.toContain("box-map__route--off");
  });
});
