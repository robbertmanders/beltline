import type { ReactNode } from "react";
import type { OverviewFloor, OverviewFloorChip, OverviewRun } from "../../api/wire.gen";
import { BoxFloor } from "./BoxFloor";
import { Chip, NeedChip } from "../../components/Paper";

export interface FloorProps {
  floor: OverviewFloor;
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  pinned: string | null;
  onHover: (belt: string, pointing: boolean) => void;
  onPin: (belt: string) => void;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenStage: (stageId: string) => void;
  onOpenBelt: (beltId: string) => void;
}

/** The Floor's card: its title and the factory's totals, with a ring while anything needs you. */
export function FloorCard({ total, children }: { total: OverviewFloor["total"]; children: ReactNode }) {
  return <section className={`overview-floor${total.needYou > 0 ? " needs-you" : ""}`} aria-label="Factory floor">
    <header className="overview-floor-header">
      <h2 className="overview-section-title">Factory floor</h2>
      <div className="overview-floor-totals">
        {total.needYou > 0 && <NeedChip>{total.needYou} need you</NeedChip>}
        <Chip look={total.running > 0 ? "running" : "plain"}>{total.running} RUNNING</Chip>
        <Chip tone="grey">{total.inProgress} IN PROGRESS</Chip>
        <Chip tone="good">{total.shippedToday} SHIPPED TODAY</Chip>
      </div>
    </header>
    {children}
  </section>;
}

/** The Floor the daemon laid out: every merged stage as a box, inside its card. */
export function Floor({ floor, ...map }: FloorProps) {
  return <FloorCard total={floor.total}>
    <BoxFloor layout={floor.layout} belts={floor.belts} boxes={floor.boxes} runs={map.runs} focus={map.focus} pinned={map.pinned}
      onHover={map.onHover} onPin={map.onPin} onOpenRun={map.onOpenRun} onOpenStage={map.onOpenStage} onOpenBelt={map.onOpenBelt} />
  </FloorCard>;
}
