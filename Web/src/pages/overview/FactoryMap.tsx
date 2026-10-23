import { useMemo } from "react";
import type { OverviewFloorBelt, OverviewFloorChip, FloorLayout, OverviewFloorMachine, OverviewRun } from "../../api/wire.gen";
import { focusOpacity } from "./focus";
import { FactoryTracks, nodeKey } from "./FactoryTracks";
import { EntranceCard, RunChip, Station } from "./Stations";

/** Entrances are drawn 208 points wide, centred 10 points left of their node, as the native floor draws them. */
const entranceWidth = 208;
const entranceCentreOffset = 10;

export interface FactoryMapProps {
  layout: FloorLayout;
  belts: readonly OverviewFloorBelt[];
  machines: readonly OverviewFloorMachine[];
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  pinned: string | null;
  onHover: (belt: string, pointing: boolean) => void;
  onPin: (belt: string) => void;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenMachine: (machineId: string) => void;
  onOpenBelt: (beltId: string) => void;
}

/**
 * The Floor as the daemon drew it: every box sits where the layout says, every track and lane is the path it
 * sent, and only the focus, dimming and chips are the browser's. The map scrolls sideways inside its own
 * viewport rather than widening the page.
 */
export function FactoryMap(props: FactoryMapProps) {
  const { layout, belts, machines, runs, focus, pinned } = props;
  const placed = useMemo(() => new Map(layout.placed.map(item => [nodeKey(item.node), item])), [layout.placed]);
  const beltById = useMemo(() => new Map(belts.map(belt => [belt.id, belt])), [belts]);
  const machineById = useMemo(() => new Map(machines.map(machine => [machine.id, machine])), [machines]);

  return <div className="factory-scroll" data-testid="floor-scroll" data-scroll-viewport>
    <div className="factory-canvas" style={{ width: layout.width, height: layout.height }}>
      <FactoryTracks layout={layout} focus={focus} />
      {belts.map(belt => {
        const at = placed.get(`entrance:${belt.id}`);
        if (!at) return null;
        return <div key={belt.id} className="factory-node" style={{ left: at.x - entranceCentreOffset - entranceWidth / 2, top: at.top, width: entranceWidth, height: at.height }}>
          <EntranceCard belt={belt} pinned={pinned === belt.id} focused={focus === belt.id}
            dimmed={belt.paused && focus !== belt.id} onPin={() => props.onPin(belt.id)}
            onHover={pointing => props.onHover(belt.id, pointing)} onRuns={() => props.onOpenBelt(belt.id)} />
        </div>;
      })}
      {layout.placed.flatMap(item => {
        if (item.node.kind !== "machine") return [];
        const machine = machineById.get(item.node.value);
        if (!machine) return [];
        return [<div key={machine.id} className="factory-node factory-machine" style={{ left: item.x - layout.column / 2, top: item.top, width: layout.column, height: item.height }}
          data-machine={machine.id} data-layer={machine.number}>
          <div className="factory-machine-body" style={{ opacity: focusOpacity(machine.belts, focus) }}>
            <Station machine={machine} dimmed={machine.dimmed} onOpen={() => props.onOpenMachine(machine.id)} />
            {machine.mixed && <span className="paper-tag" title={machine.workers}>MIXED</span>}
            <div className="factory-chips">
              {machine.chips.map(chip => <RunChip key={chip.runId} chip={chip} run={runs.get(chip.runId)} onOpen={() => props.onOpenRun(chip)} />)}
              {machine.more > 0 && <span className="factory-more">+{machine.more}</span>}
            </div>
          </div>
        </div>];
      })}
    </div>
    {beltById.size === 0 && <p className="empty-state" role="status">No belts are on the floor.</p>}
  </div>;
}
