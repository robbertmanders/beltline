import { useMemo } from "react";
import type { FloorTrack, OverviewFloorBelt, OverviewFloorBox, OverviewFloorChip, OverviewRun, StageIcon, Tint } from "../../api/wire.gen";
import { Chip, NeedChip, type Look } from "../../components/Paper";
import { useElementWidth } from "../../components/useElementWidth";
import { StageBox, type BoxState } from "../../theme/stages/StageBox";
import { focusOpacity } from "./focus";
import { FactoryTracks } from "./FactoryTracks";
import { floorBoxFrame, placeFloor, planFloor, type FloorPlaced } from "./floorLayout";

export interface BoxFloorProps {
  track: FloorTrack;
  belts: readonly OverviewFloorBelt[];
  boxes: readonly OverviewFloorBox[];
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  pinned: string | null;
  onHover: (belt: string, pointing: boolean) => void;
  onPin: (belt: string) => void;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenStage: (stageId: string) => void;
  onOpenBelt: (beltId: string) => void;
}

/** A status's box state: how the box's border and fill read. */
const boxStates: Record<Look, BoxState> = { done: "done", running: "running", waiting: "needs", held: "held", plain: "idle" };

/**
 * The Floor: each belt's card on the left, each merged stage as one box with its status above and its run tags
 * below, and the tracks between them. The daemon says which nodes sit in which layer and which tracks join them;
 * the page lays them out over its width (planFloor once per track, placeFloor on every resize), lights a focused
 * belt's tracks and dims what it does not touch.
 */
export function BoxFloor(props: BoxFloorProps) {
  const { track, belts, boxes, runs, focus, pinned } = props;
  const [scrollRef, width] = useElementWidth<HTMLDivElement>();
  const plan = useMemo(() => planFloor(track, boxes), [track, boxes]);
  const layout = useMemo(() => placeFloor(track, plan, width > 0 ? width : undefined), [track, plan, width]);
  const placed = layout.byKey;

  return <div ref={scrollRef} className="factory-scroll box-floor" data-testid="floor-scroll" data-scroll-viewport>
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
      {boxes.map(box => {
        const at = placed.get(`stage:${box.id}`);
        if (!at) return null;
        return <BoxCell key={box.id} box={box} at={at} runs={runs} focus={focus}
          onOpenRun={props.onOpenRun} onOpenStage={props.onOpenStage} />;
      })}
    </div>
    {belts.length === 0 && <p className="empty-state" role="status">No belts are on the floor.</p>}
  </div>;
}

/** One merged stage: its status chip above, its stage box, and its run tags below, in its placed cell. */
function BoxCell({ box, at, runs, focus, onOpenRun, onOpenStage }: {
  box: OverviewFloorBox;
  at: FloorPlaced;
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenStage: (stageId: string) => void;
}) {
  const frame = floorBoxFrame(at);
  return <div className="floor-box" data-stage={box.id} style={{ left: frame.x, top: frame.y, width: frame.width, height: frame.height, opacity: focusOpacity(box.belts, focus) }}>
    <StatusChip look={box.status.look as Look} text={box.status.text} />
    <StageBox icon={box.icon as StageIcon} tint={box.tint as Tint} caption={box.caption} title={box.name} state={boxStates[box.status.look as Look]}
      label={`${box.name}: ${box.status.text.toLowerCase()}`} onClick={() => onOpenStage(box.id)} />
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

/** A run tag's name: its reference and its belt's short name. */
export function chipLabel(chip: OverviewFloorChip): string {
  return [chip.ref, chip.belt].filter(Boolean).join(" · ");
}

/** The hover text for a run: its subject, repository, and where it is. */
export function runCardText(run: OverviewRun | undefined, fallback: string): string {
  if (!run) return fallback;
  return `${run.ref} ${run.title} · ${run.repository} · ${run.location.text}`;
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
