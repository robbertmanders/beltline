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
  it("draws each stage as the Floor does, with its number, name and chip, and the run's caption beside the metadata", () => {
    const { container } = render(<RunBoxes boxes={boxes} metadata="Belt v1 · 2 stages" selectedStageId="triage" onSelectStage={vi.fn()} />);
    expect(screen.getByText("Belt v1 · 2 stages")).toBeTruthy();
    expect(screen.getByText("Now at Triage")).toBeTruthy();
    expect(screen.getByText("01")).toBeTruthy();
    expect(screen.getByText("02")).toBeTruthy();
    expect(screen.getByText("Waiting for you")).toBeTruthy();
    // The trigger is where the line starts: drawn, but not a stage to select.
    expect(screen.getByText("#139")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "#139" })).toBeNull();
    expect(container.querySelector('[data-stage="trigger"]')?.className).toContain("is-trigger");
    const triage = container.querySelector('[data-stage="triage"]')!;
    expect(triage.className).toContain("is-here");
    expect(triage.className).toContain("is-selected");
    expect(screen.getByRole("button", { name: "Triage — Running" }).getAttribute("aria-pressed")).toBe("true");
    expect(container.querySelector('[data-stage="approve"]')?.className).toContain("is-waiting");
  });

  it("draws the tracks in ink where the run has travelled", () => {
    const { container } = render(<RunBoxes boxes={boxes} metadata="Belt v1" selectedStageId={null} onSelectStage={vi.fn()} />);
    const tracks = [...container.querySelectorAll(".run-track")].map(path => path.getAttribute("class"));
    expect(tracks).toEqual(["run-track is-todo", "run-track is-done"]);
  });

  it("selects a stage when its name is clicked", () => {
    const onSelectStage = vi.fn();
    render(<RunBoxes boxes={boxes} metadata="Belt v1" selectedStageId="triage" onSelectStage={onSelectStage} />);
    fireEvent.click(screen.getByRole("button", { name: "Approve — Waiting for you" }));
    expect(onSelectStage).toHaveBeenLastCalledWith("approve");
  });

  it("names a route on its track: in ink when taken, struck through when passed by, soft while still ahead", () => {
    const route = { fork: "size", route: "simple", name: "Simple", column: 1, row: 1, stageIds: ["approve"], feeds: false };
    const withRoute: RunBoxesData = { ...boxes, map: { ...map, routes: [route] } };
    const look = (data: RunBoxesData) => {
      const { unmount } = render(<RunBoxes boxes={data} metadata="Belt v1" selectedStageId={null} onSelectStage={vi.fn()} />);
      const className = screen.getByText("Simple").className;
      unmount();
      return className;
    };
    expect(look(withRoute)).toBe("run-map-route");
    expect(look({ ...withRoute, nodes: boxes.nodes.map(node => node.id === "approve" ? { ...node, state: "skip" as const } : node) })).toContain("is-off");
    expect(look({ ...withRoute, takenRoutes: ["size:simple"] })).toContain("is-taken");
  });
});
