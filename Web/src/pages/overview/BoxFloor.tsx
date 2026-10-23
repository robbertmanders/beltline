import { useMemo } from "react";
import type { OverviewFloorBelt, OverviewFloorBox, OverviewFloorChip, FloorLayout, OverviewRun, StageIcon, Tint } from "../../api/wire.gen";
import { Chip, NeedChip, type Look } from "../../components/Paper";
import { StageBox, type BoxState } from "../../theme/stages/StageBox";
import { focusOpacity } from "./focus";
import { FactoryTracks, nodeKey } from "./FactoryTracks";
import { chipLabel, runCardText } from "./Stations";

export interface BoxFloorProps {
  layout: FloorLayout;
  belts: readonly OverviewFloorBelt[];
  boxes: readonly OverviewFloorBox[];
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  pinned: string | null;
  onHover: (belt: string, pointing: boolean) => void;
  onPin: (belt: string) => void;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenMachine: (machineId: string) => void;
  onOpenBelt: (beltId: string) => void;
}

/** A status's box state: how the box's border and fill read. */
const boxStates: Record<Look, BoxState> = { done: "done", running: "running", waiting: "needs", held: "held", plain: "idle" };

/**
 * The boxes Floor as the daemon laid it out: each belt's card on the left, each merged stage as one box with its
 * status above and its run tags below, and the tracks between them. The positions are the daemon's; the
 * browser only draws the boxes, lights a focused belt's tracks and dims what it does not touch.
 */
export function BoxFloor(props: BoxFloorProps) {
  const { layout, belts, boxes, runs, focus, pinned } = props;
  const placed = useMemo(() => new Map(layout.placed.map(item => [nodeKey(item.node), item])), [layout.placed]);

  return <div className="factory-scroll box-floor" data-testid="floor-scroll" data-scroll-viewport>
    <div className="factory-canvas" style={{ width: layout.width, height: layout.height }}>
      <FactoryTracks layout={layout} focus={focus} />
      {belts.map(belt => {
        const at = placed.get(`entrance:${belt.id}`);
        if (!at) return null;
        return <div key={belt.id} className="box-floor-entrance" style={{ top: at.top, height: at.height }}>
          <BoxBeltCard belt={belt} pinned={pinned === belt.id} focused={focus === belt.id}
            onPin={() => props.onPin(belt.id)} onHover={pointing => props.onHover(belt.id, pointing)}
            onRuns={() => props.onOpenBelt(belt.id)} />
        </div>;
      })}
      {boxes.map(box => <BoxCell key={box.id} box={box} runs={runs} focus={focus}
        onOpenRun={props.onOpenRun} onOpenMachine={props.onOpenMachine} />)}
    </div>
    {belts.length === 0 && <p className="empty-state" role="status">No belts are on the floor.</p>}
  </div>;
}

/** One merged stage: its status chip above, its stage box, and its run tags below, in the cell the daemon sized. */
function BoxCell({ box, runs, focus, onOpenRun, onOpenMachine }: {
  box: OverviewFloorBox;
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenMachine: (machineId: string) => void;
}) {
  return <div className="floor-box" data-machine={box.id} style={{ left: box.x, top: box.y, width: box.width, height: box.height, opacity: focusOpacity(box.belts, focus) }}>
    <StatusChip look={box.status.look as Look} text={box.status.text} />
    <StageBox icon={box.icon as StageIcon} tint={box.tint as Tint} caption={box.caption} title={box.name} state={boxStates[box.status.look as Look]}
      label={`${box.name}: ${box.status.text.toLowerCase()}`} onClick={() => onOpenMachine(box.id)} />
    <div className="floor-box-tags">
      {box.chips.map(chip => <RunTag key={chip.runId} chip={chip} run={runs.get(chip.runId)} onOpen={() => onOpenRun(chip)} />)}
      {box.more > 0 && <span className="floor-tag floor-tag-more">+{box.more}</span>}
    </div>
  </div>;
}

function StatusChip({ look, text }: { look: Look; text: string }) {
  if (look === "waiting") return <span className="floor-box-status"><NeedChip>{text}</NeedChip></span>;
  if (look === "plain") return <span className="floor-box-status"><Chip tone="grey">{text}</Chip></span>;
  return <span className="floor-box-status"><Chip look={look}>{text}</Chip></span>;
}

/** A run tag under a box: its reference and belt, lit by what the run needs. */
function RunTag({ chip, run, onOpen }: { chip: OverviewFloorChip; run: OverviewRun | undefined; onOpen: () => void }) {
  const tone = chip.state === "waiting" ? " floor-tag-needs" : chip.state === "held" ? " floor-tag-held" : "";
  return <button type="button" className={`floor-tag${tone}`} title={runCardText(run, chip.ref)} onClick={onOpen}>{chipLabel(chip)}</button>;
}

/** A belt's card at the Floor's left: its name, filter and counts, and its Runs button. Pinning or pointing lights its tracks. */
export function BoxBeltCard({ belt, pinned, focused, onPin, onHover, onRuns }: {
  belt: OverviewFloorBelt;
  pinned: boolean;
  focused: boolean;
  onPin: () => void;
  onHover: (pointing: boolean) => void;
  onRuns: () => void;
}) {
  const help = pinned ? `Click to stop showing ${belt.name}'s way through the factory` : `Click to keep ${belt.name}'s way through the factory drawn`;
  return <div className={`box-belt${pinned ? " is-pinned" : ""}${focused ? " is-focused" : ""}`} data-belt={belt.id} title={help}
    onPointerEnter={() => onHover(true)} onPointerLeave={() => onHover(false)} onClick={onPin}>
    <button type="button" className="box-belt-pin" aria-pressed={pinned} aria-label={help}
      onClick={event => { event.stopPropagation(); onPin(); }}>
      <span className="box-belt-name">{belt.name}</span>
      {belt.paused && <Chip tone="grey">Paused</Chip>}
    </button>
    <span className="box-belt-filter">{belt.summary}</span>
    <span className="box-belt-counts">
      {belt.counts.needYou > 0 && <NeedChip>{belt.counts.needYou} need you</NeedChip>}
      <span className="box-belt-running">{belt.counts.running} running</span>
    </span>
    <button type="button" className="action-button small box-belt-runs" onClick={event => { event.stopPropagation(); onRuns(); }}>Runs →</button>
  </div>;
}
