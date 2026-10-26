import { cleanup, fireEvent, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { OverviewFloorChip, OverviewResponse, OverviewRun } from "../../api/wire.gen";
import { BoxFloor, type BoxFloorProps } from "./BoxFloor";
import { hoveredAfter, resolveFocus } from "./focus";
import { golden } from "../../test/golden";

const overview = (): OverviewResponse => golden<OverviewResponse>("views-overview.json");

function runsOf(view: OverviewResponse): Map<string, OverviewRun> {
  return new Map(view.runs.map(run => [run.id, run]));
}

/** The page's own hover and pin rules, so a belt's focus runs the way the Overview runs it. */
function Harness({ view, pinned: initialPin = null, onOpenRun = vi.fn(), onOpenStage = vi.fn(), onOpenBelt = vi.fn() }: {
  view: OverviewResponse;
  pinned?: string | null;
  onOpenRun?: (chip: OverviewFloorChip) => void;
  onOpenStage?: (stageId: string) => void;
  onOpenBelt?: (beltId: string) => void;
}) {
  const [pinned, setPinned] = useState<string | null>(initialPin);
  const [hovered, setHovered] = useState<string | null>(null);
  const props: BoxFloorProps = {
    track: view.floor.track,
    belts: view.floor.belts,
    boxes: view.floor.boxes,
    runs: runsOf(view),
    focus: resolveFocus(pinned, hovered),
    pinned,
    onHover: (belt, pointing) => setHovered(current => hoveredAfter(current, belt, pointing)),
    onPin: belt => setPinned(current => (current === belt ? null : belt)),
    onOpenRun,
    onOpenStage,
    onOpenBelt
  };
  return <BoxFloor {...props} />;
}

const dimmed = (element: Element | null) => element?.getAttribute("style")?.includes("opacity: 0.35") ?? false;
const cell = (id: string) => document.querySelector<HTMLElement>(`.floor-box[data-stage='${id}']`)!;

afterEach(() => cleanup());

describe("Floor drawing", () => {
  it("shows each box's status above it and at most three run tags, then the count of the rest", () => {
    const view = overview();
    const busy = view.floor.boxes.find(box => box.runCount > 3);
    expect(busy).toBeDefined();
    render(<Harness view={view} />);
    const element = cell(busy!.id);
    expect(element.querySelector(".floor-box-status")?.textContent).toBe(busy!.status.text);
    expect(element.querySelectorAll(".floor-tag:not(.floor-tag-more)").length).toBe(Math.min(busy!.chips.length, 3));
    expect(element.querySelector(".floor-tag-more")?.textContent).toBe(`+${busy!.more}`);
  });

  it("names each belt on its card, with its counts and Runs button", () => {
    const view = overview();
    render(<Harness view={view} />);
    for (const belt of view.floor.belts) {
      const card = document.querySelector(`.box-belt[data-belt='${belt.id}']`)!;
      expect(card.textContent).toContain(belt.name);
      expect(card.querySelector(".box-belt-runs")?.textContent).toBe("Runs →");
      expect(card.querySelector(".box-belt-running")?.textContent).toBe(`${belt.counts.running} running`);
    }
  });
});

describe("Boxes Floor focus and actions", () => {
  it("lights a hovered belt's tracks and dims the boxes it does not run", async () => {
    const view = overview();
    render(<Harness view={view} />);
    const card = document.querySelector(`.box-belt[data-belt='issues']`)!;
    fireEvent.pointerEnter(card);
    expect(document.querySelectorAll(".factory-track-focus").length)
      .toBe(view.floor.track.tracks.filter(stretch => stretch.belts.includes("issues")).length);
    for (const box of view.floor.boxes) {
      expect(dimmed(cell(box.id))).toBe(!box.belts.includes("issues"));
    }
    fireEvent.pointerLeave(card);
    expect(document.querySelectorAll(".factory-track-focus").length).toBe(0);
  });

  it("keeps a pinned belt lit, and a click on the card toggles the pin", async () => {
    const user = userEvent.setup();
    const view = overview();
    render(<Harness view={view} pinned="issues" />);
    expect(document.querySelector(".box-belt[data-belt='issues']")!.classList.contains("is-pinned")).toBe(true);
    expect(document.querySelectorAll(".factory-track-focus").length).toBeGreaterThan(0);
    const card = document.querySelector(".box-belt[data-belt='issues']")!;
    await user.click(document.querySelector<HTMLButtonElement>(".box-belt[data-belt='issues'] .box-belt-pin")!);
    expect(card.classList.contains("is-pinned")).toBe(false);
    // The pointer is still on the card, so its hover keeps the belt lit until the pointer leaves.
    expect(document.querySelectorAll(".factory-track-focus").length).toBeGreaterThan(0);
    fireEvent.pointerLeave(card);
    expect(document.querySelectorAll(".factory-track-focus").length).toBe(0);
  });

  it("opens the belt's runs from its Runs button without pinning it", async () => {
    const user = userEvent.setup();
    const view = overview();
    const onOpenBelt = vi.fn();
    render(<Harness view={view} onOpenBelt={onOpenBelt} />);
    await user.click(document.querySelector<HTMLButtonElement>(".box-belt[data-belt='issues'] .box-belt-runs")!);
    expect(onOpenBelt).toHaveBeenCalledWith("issues");
    expect(document.querySelector(".box-belt[data-belt='issues']")!.classList.contains("is-pinned")).toBe(false);
  });

  it("opens a box's stage when its stage box is clicked, and a run tag's run at its stage", async () => {
    const user = userEvent.setup();
    const view = overview();
    const onOpenStage = vi.fn();
    const onOpenRun = vi.fn();
    render(<Harness view={view} onOpenStage={onOpenStage} onOpenRun={onOpenRun} />);
    const box = view.floor.boxes.find(item => item.chips.length > 0)!;
    await user.click(cell(box.id).querySelector<HTMLButtonElement>(".stage-box")!);
    expect(onOpenStage).toHaveBeenCalledWith(box.id);
    await user.click(cell(box.id).querySelector<HTMLButtonElement>(".floor-tag")!);
    expect(onOpenRun).toHaveBeenCalledWith(box.chips[0]);
  });
});
