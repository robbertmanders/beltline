import { useRef } from "react";
import type { BoxMap, BoxMapNode, RunBoxes as RunBoxesData } from "../../api/wire.gen";
import { Caps, NeedChip, StatusPill, type Look } from "../../components/Paper";
import { BoxMapView, type BoxContent, type BoxState, type TrackState } from "../../theme/stages/StageBox";
import { useCentreOnStage } from "./centerOnStage";

export interface RunBoxesProps {
  boxes: RunBoxesData;
  metadata: string;
  selectedStageId: string | null;
  onSelectStage: (stageId: string) => void;
  /** The run and its current stage, which the line opens centred on. */
  opening?: { run: string; stage: string };
}

const looks: readonly Look[] = ["done", "running", "waiting", "held", "plain"];
const boxStates: readonly BoxState[] = ["idle", "done", "todo", "skip", "needs", "held", "running", "watching", "paused"];

/** The daemon names every look and state; anything unknown reads as a plain box, never a guess of its own. */
function asLook(value: string | undefined): Look {
  return (looks as readonly string[]).includes(value ?? "") ? value as Look : "plain";
}
function asBoxState(value: string | undefined): BoxState {
  return (boxStates as readonly string[]).includes(value ?? "") ? value as BoxState : "idle";
}

/** The chip above a box: the solid slate one for a stage waiting for you, the status pill otherwise. */
function chipNode(text: string, look: string) {
  if (look === "waiting") return <NeedChip>{text}</NeedChip>;
  return <StatusPill text={text} look={asLook(look)} />;
}

/** A track's identity: the boxes it joins and the route it runs on, since two routes can join the same boxes. */
function trackKey(edge: { from: string; to: string; route?: string }): string {
  return `${edge.from}>${edge.to}>${edge.route ?? ""}`;
}

/**
 * The run's belt as stage boxes on hooked tracks (BoxMapView), in a card with the run's metadata above it. The
 * daemon puts every box in its column and row, says which corners each track turns, and gives each one's words,
 * state and chip; the page sizes the grid, draws them and reports a click on a stage. The trigger's box is not a
 * stage, so selecting it does nothing.
 */
export function RunBoxes({ boxes, metadata, selectedStageId, onSelectStage, opening }: RunBoxesProps) {
  const section = useRef<HTMLElement>(null);
  useCentreOnStage(section, opening);
  const nodes = new Map(boxes.nodes.map(node => [node.id, node]));
  const tracks = new Map(boxes.edges.map(edge => [trackKey(edge), edge.state]));
  const taken = new Set(boxes.takenRoutes);
  const map: BoxMap = boxes.map;
  const content = (node: BoxMapNode): BoxContent => {
    const words = nodes.get(node.id);
    const chip = words?.chip ?? null;
    return {
      caption: words?.caption ?? "",
      title: words?.title ?? node.id,
      details: words?.details ?? [],
      state: asBoxState(words?.state),
      dot: words?.dot ?? null,
      above: chip ? chipNode(chip.text, chip.look) : undefined,
      label: chip ? `${words?.title ?? node.id} — ${chip.text}` : words?.title ?? node.id
    };
  };
  const track = (edge: BoxMap["edges"][number]): TrackState => tracks.get(trackKey(edge)) ?? "todo";
  return <section ref={section} className="run-line paper-card paper-card--raised" aria-label="Stage line">
    <header className="run-line-header"><Caps>{metadata}</Caps><Caps>{boxes.caption}</Caps></header>
    <BoxMapView map={map} label="Stage boxes" content={content} track={track}
      routeTaken={route => taken.has(`${route.fork}:${route.route}`)}
      selected={selectedStageId}
      onSelect={nodeId => { if (nodeId !== "trigger") onSelectStage(nodeId); }} />
  </section>;
}
