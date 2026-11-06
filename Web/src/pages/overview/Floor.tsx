import { useLayoutEffect, useRef } from "react";
import type { OverviewFloor, OverviewFloorChip, OverviewRun } from "../../api/wire.gen";
import { BoxFloor } from "./BoxFloor";

export interface FloorProps {
  floor: OverviewFloor;
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  pinned: string | null;
  onHover: (belt: string, pointing: boolean) => void;
  /** Pins a belt, or with null shows every belt again. */
  onPin: (belt: string | null) => void;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenStage: (stageId: string) => void;
  onOpenBelt: (beltId: string) => void;
}

/** One line under the chosen chip, which slides to the next chip chosen. */
export function useChipIndicator<T>(dependency: T) {
  const box = useRef<HTMLDivElement>(null);
  const line = useRef<HTMLSpanElement>(null);
  const placed = useRef(false);
  useLayoutEffect(() => {
    const chip = box.current?.querySelector<HTMLElement>(".chip.on");
    const mark = line.current;
    if (!mark) return;
    if (!chip) { mark.style.width = "0px"; return; }
    if (!placed.current) mark.style.transition = "none";
    mark.style.left = `${chip.offsetLeft}px`;
    mark.style.top = `${chip.offsetTop + chip.offsetHeight - 1}px`;
    mark.style.width = `${chip.offsetWidth}px`;
    if (!placed.current) { void mark.offsetWidth; mark.style.transition = ""; placed.current = true; }
  }, [dependency]);
  return { box, line };
}

/**
 * The floor: a chip per belt to pin one, and every merged stage on the tracks the daemon laid out. Pinning a belt
 * or pointing at it traces its way through the factory and lets the rest fall back.
 */
export function Floor({ floor, ...map }: FloorProps) {
  const { box, line } = useChipIndicator(map.pinned);
  const focused = floor.belts.find(belt => belt.id === map.focus);
  const { total } = floor;
  return <section className="sec overview-floor" aria-label="Factory floor">
    <div className="sec-h"><h2 className="h3">The floor</h2>
      <span className="help">{focused ? `${focused.name}: ${focused.summary} · ${focused.counts.running} running · ${focused.counts.needYou} need you`
        : `${total.running} running · ${total.needYou} need you · ${total.inProgress} in progress · ${total.shippedToday} shipped today`}</span></div>
    <div className="chips fchips" ref={box} role="group" aria-label="Show a belt on the floor">
      <span className="cind" ref={line} aria-hidden="true" />
      <button type="button" className={`chip ${map.pinned === null ? "on" : ""}`} aria-pressed={map.pinned === null} onClick={() => map.onPin(null)}>All belts</button>
      {floor.belts.map(belt => <button key={belt.id} type="button" className={`chip ${map.pinned === belt.id ? "on" : ""}`} aria-pressed={map.pinned === belt.id}
        onPointerEnter={() => map.onHover(belt.id, true)} onPointerLeave={() => map.onHover(belt.id, false)} onClick={() => map.onPin(belt.id)}>
        {belt.name} <span className="k">{belt.counts.inProgress}</span>
      </button>)}
    </div>
    <BoxFloor track={floor.track} belts={floor.belts} boxes={floor.boxes} runs={map.runs} focus={map.focus} pinned={map.pinned}
      onHover={map.onHover} onPin={belt => map.onPin(belt)} onOpenRun={map.onOpenRun} onOpenStage={map.onOpenStage} onOpenBelt={map.onOpenBelt} />
  </section>;
}
