import { useMemo } from "react";
import type { FloorTrack, OverviewFloorBelt, OverviewFloorBox, OverviewFloorChip, OverviewRun } from "../../api/wire.gen";
import type { Look } from "../../components/Paper";
import { useElementWidth } from "../../components/useElementWidth";
import { focusOpacity } from "./focus";
import { FactoryTracks } from "./FactoryTracks";
import { floorBoxFrame, floorMetrics, placeFloor, planFloor, type FloorPlaced } from "./floorLayout";

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

/**
 * The floor's drawing: each belt where its line starts, each merged stage as a dot on the line with its name above
 * and its runs below, and the tracks between them as hairlines. The daemon says which nodes sit in which layer and
 * which tracks join them; the page lays them out over its width (planFloor once per track, placeFloor on every
 * resize), traces a focused belt's tracks and lets what it does not touch fall back.
 */
export function BoxFloor(props: BoxFloorProps) {
  const { track, belts, boxes, runs, focus, pinned } = props;
  const [scrollRef, width] = useElementWidth<HTMLDivElement>();
  const plan = useMemo(() => planFloor(track, boxes), [track, boxes]);
  const layout = useMemo(() => placeFloor(track, plan, width > 0 ? width : undefined), [track, plan, width]);
  const placed = layout.byKey;

  return <div ref={scrollRef} className={`factory-scroll box-floor scroll ${focus ? "trace" : ""}`} data-testid="floor-scroll" data-scroll-viewport>
    <div className="factory-canvas" style={{ width: layout.width, height: layout.height }}>
      <FactoryTracks layout={layout} focus={focus} />
      {belts.map(belt => {
        const at = placed.get(`entrance:${belt.id}`);
        if (!at) return null;
        return <div key={belt.id} className="box-floor-entrance" style={{ top: at.top, height: at.height, opacity: focusOpacity([belt.id], focus) }}>
          <BoxBeltCard belt={belt} pinned={pinned === belt.id} focused={focus === belt.id} anchor={at.anchor - at.top}
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
    {belts.length === 0 && <p className="empty" role="status">No belts are on the floor.</p>}
  </div>;
}

/** One merged stage: its name and status above the line, its dot on it, and its runs below, in its placed cell. */
function BoxCell({ box, at, runs, focus, onOpenRun, onOpenStage }: {
  box: OverviewFloorBox;
  at: FloorPlaced;
  runs: ReadonlyMap<string, OverviewRun>;
  focus: string | null;
  onOpenRun: (chip: OverviewFloorChip) => void;
  onOpenStage: (stageId: string) => void;
}) {
  const frame = floorBoxFrame(at);
  const look = box.status.look as Look;
  const has = box.runCount > 0;
  return <div className={`floor-box ${has ? "has" : ""} is-${look}`} data-stage={box.id}
    style={{ left: frame.x, top: frame.y, width: frame.width, height: frame.height, opacity: focusOpacity(box.belts, focus), "--line": `${at.anchor - at.top}px` } as React.CSSProperties}>
    <i className="ln h" aria-hidden="true" />
    <span className="dot" aria-hidden="true" />
    <button type="button" className="fh stage-box" aria-label={`${box.name}: ${box.status.text.toLowerCase()}`} title={`${box.caption} · ${box.status.text}`} onClick={() => onOpenStage(box.id)}>
      <b>{box.name}</b>{has && <span className="link">{box.runCount}</span>}
    </button>
    <span className={`fstatus ${look === "waiting" || look === "held" ? "is-strong" : ""}`}>{box.status.text}</span>
    <div className="lane floor-box-tags">
      {box.chips.map(chip => <RunTag key={chip.runId} chip={chip} run={runs.get(chip.runId)} onOpen={() => onOpenRun(chip)} />)}
      {box.more > 0 && <button type="button" className="link soft fmore floor-tag-more" onClick={() => onOpenStage(box.id)}>{box.more} more</button>}
    </div>
  </div>;
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

/** A run under a stage, as the mockup's mini: its title, its reference and belt, and a hairline that is filled while it waits for you. */
function RunTag({ chip, run, onOpen }: { chip: OverviewFloorChip; run: OverviewRun | undefined; onOpen: () => void }) {
  const tone = chip.state === "waiting" ? " floor-tag-needs wait" : chip.state === "held" ? " floor-tag-held wait" : "";
  return <button type="button" className={`mini floor-tag${tone}`} title={runCardText(run, chip.ref)} onClick={onOpen}>
    <span className="t">{run?.title ?? chip.ref}</span>
    <span className="s">{chipLabel(chip)}{chip.state === "waiting" ? " · needs you" : chip.state === "held" ? " · held" : chip.paused ? " · paused" : ""}</span>
    <span className="bar"><i style={{ "--p": chip.state === "waiting" || chip.state === "held" ? 1 : chip.state === "running" ? 0.5 : 0.12 } as React.CSSProperties} /></span>
  </button>;
}

/** A belt where its line starts: its name, trigger and counts. Clicking it pins its way through the factory; pointing traces it. */
export function BoxBeltCard({ belt, pinned, focused, anchor, onPin, onHover, onRuns }: {
  belt: OverviewFloorBelt;
  pinned: boolean;
  focused: boolean;
  anchor?: number;
  onPin: () => void;
  onHover: (pointing: boolean) => void;
  onRuns: () => void;
}) {
  const help = pinned ? `Click to stop showing ${belt.name}'s way through the factory` : `Click to keep ${belt.name}'s way through the factory drawn`;
  return <div className={`box-belt${pinned ? " is-pinned" : ""}${focused ? " is-focused" : ""}`} data-belt={belt.id} title={help}
    style={{ "--line": `${anchor ?? floorMetrics.entranceHeight / 2}px` } as React.CSSProperties}
    onPointerEnter={() => onHover(true)} onPointerLeave={() => onHover(false)}>
    <button type="button" className="box-belt-pin" aria-pressed={pinned} aria-label={help} onClick={onPin}>
      <span className="box-belt-name">{belt.name}</span>
      {belt.paused && <span className="flag o">Paused</span>}
    </button>
    <span className="box-belt-filter">{belt.summary}</span>
    <button type="button" className="link soft box-belt-runs" onClick={onRuns}>{belt.counts.running} running{belt.counts.needYou > 0 ? ` · ${belt.counts.needYou} need you` : ""}</button>
    <span className="dot" aria-hidden="true" />
  </div>;
}
