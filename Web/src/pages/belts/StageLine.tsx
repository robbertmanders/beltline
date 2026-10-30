import { useEffect, useRef, useState, type ReactElement } from "react";
import { useDraftEditor, useTypedForm } from "./draftEditor";
import type { Belt, StageLinePresentation, StageLinePresentationItem, StageLinePresentationRouteRow } from "../../api/wire.gen";

/** The drag data type that carries a stage id between stage line blocks. */
const stageDragType = "application/x-beltline-stage";

export interface AgentOption {
  id: string;
  name: string;
  version: number;
}

/** The kind a stage block shows before the daemon's structure arrives: its number and type, worded as the daemon words them. */
export function placeholderStageKind(stage: Belt["stages"][number], number: number): string {
  return `${String(number).padStart(2, "0")} · ${stage.type === "approve" ? "Approval" : stage.type === "prompt" ? "Prompt" : stage.type === "watch" ? "Watch" : "Agent"}`;
}

export interface StageLineProps {
  /** The daemon's structure for the draft; null until the editor's context is read. */
  line: StageLinePresentation | null;
  agents: readonly AgentOption[];
  agentChoice: string;
  onAgentChoice: (choice: string) => void;
  onNeedAgents: () => void;
  onUpdateStage: (stageId: string) => void;
  updateStageEnabled: boolean;
}

/**
 * The belt's trigger and stages as blocks you select, add between, drag and remove. A fork is a
 * box with a row per route. The structure, numbers and insert positions are the daemon's; names come from the draft.
 */
export function StageLine({ line: sent, onUpdateStage, updateStageEnabled }: StageLineProps) {
  const editor = useDraftEditor();
  const belt: Belt = editor.belt;
  const disabled = (editor.blocked ?? editor.structuralReason) !== null;
  const [dragging, setDragging] = useState<string | null>(null);
  // Until the daemon's context for this draft arrives, the stages are drawn in order without forks; its answer replaces them.
  const line: StageLinePresentation = sent ?? {
    trigger: { kind: "Trigger", title: "", lines: [] }, addEnd: belt.stages.length, agents: [], watchAllowed: false,
    items: belt.stages.map((stage, index) => ({ addAt: index, stage: { id: stage.id, kind: placeholderStageKind(stage, index + 1), subtitle: [] } }))
  };

  const block = (item: StageLinePresentationItem) => {
    const stage = item.stage!;
    const saved = belt.stages.find(entry => entry.id === stage.id);
    const name = saved?.name ?? stage.id;
    const selected = editor.selectedStageId === stage.id;
    // A shadow's stage that differs from live carries the daemon's tag for how.
    const difference = editor.shadow ? editor.context?.shadowDifferences?.find(entry => entry.stageId === stage.id) : undefined;
    const marked = difference ? ` shadow-diff shadow-diff--${difference.change}` : "";
    return <div key={stage.id} className={`stage-block-wrap${dragging === stage.id ? " is-dragging" : ""}${marked}`} id={`stage-${stage.id}`} data-stage={stage.id}
      draggable={!disabled} aria-roledescription="draggable stage"
      onDragStart={event => { setDragging(stage.id); event.dataTransfer.setData(stageDragType, stage.id); event.dataTransfer.effectAllowed = "move"; }}
      onDragEnd={() => setDragging(null)}
      onDragOver={event => { if (!disabled && event.dataTransfer.types.includes(stageDragType)) event.preventDefault(); }}
      onDrop={event => {
        const dragged = event.dataTransfer.getData(stageDragType);
        event.preventDefault();
        setDragging(null);
        if (!disabled && dragged && dragged !== stage.id) editor.operate("stage-move", { stageId: dragged, targetId: stage.id });
      }}>
      <button type="button" className={`stage-block${selected ? " is-selected" : ""}`} aria-pressed={selected} aria-label={name}
        title={disabled ? `${name}: click to see it` : `${name}: click to edit, drag to reorder`}
        onClick={() => editor.selectStage(selected ? null : stage.id)}>
        <span className="stage-block-kind">{stage.kind}</span>
        <span className="stage-block-title">{name}</span>
        <span className="stage-block-lines">{[0, 1].map(index => <span key={index}>{stage.subtitle[index] ?? " "}</span>)}</span>
      </button>
      {difference && <span className="shadow-diff__tag">{difference.label}</span>}
      {!disabled && <button type="button" className="stage-block-remove" aria-label={`Remove ${name}`} title={`Remove ${name}`}
        onClick={() => { editor.operate("stage-remove", { stageId: stage.id }); if (selected) editor.selectStage(null); }}>×</button>}
      {!disabled && stage.update !== undefined && <button type="button" className="stage-block-update" disabled={!updateStageEnabled}
        aria-label={`Update ${name} to its agent's latest version`} title={`Update to v${stage.update}`} onClick={() => onUpdateStage(stage.id)}>↑ v{stage.update}</button>}
    </div>;
  };

  const items = (list: readonly StageLinePresentationItem[], route?: string) => list.map(item => <div key={item.stage?.id ?? `fork:${item.fork?.id}`} className="stage-line-piece">
    <AddMenu line={line} at={item.addAt} route={route} disabled={disabled} />
    {item.stage ? block(item) : <ForkBox item={item} line={line} disabled={disabled} renderItems={items} />}
  </div>);

  const triggerSelected = editor.selectedStageId === null;
  return <section className="stage-line-card" aria-label="Stages">
    <div className="stage-line-scroll" data-scroll-viewport>
      <div className="stage-line-row">
        <button type="button" className={`stage-block stage-block-trigger${triggerSelected ? " is-selected" : ""}`} aria-pressed={triggerSelected} aria-label="Trigger"
          title={line.trigger.help ?? "The trigger: where the belt's work comes from"} onClick={() => editor.selectStage(null)}>
          <span className="stage-block-kind">{line.trigger.kind}</span>
          <span className="stage-block-title">{line.trigger.title}</span>
          <span className="stage-block-lines">{[0, 1].map(index => <span key={index}>{line.trigger.lines[index] ?? " "}</span>)}</span>
        </button>
        {items(line.items)}
        <AddMenu line={line} at={line.addEnd} disabled={disabled} />
      </div>
    </div>
  </section>;
}

/** A fork: its routes one under another, each with its name, stages and any fork nested there. */
function ForkBox({ item, line, disabled, renderItems }: {
  item: StageLinePresentationItem; line: StageLinePresentation; disabled: boolean; renderItems: (list: readonly StageLinePresentationItem[], route?: string) => ReactElement[]
}) {
  const editor = useDraftEditor();
  const fork = item.fork!;
  return <section className="fork-box" id={`fork-${fork.id}`} aria-label={`Fork ${fork.id}`}>
    <div className="fork-box-head">
      <span className="fork-box-heading">{fork.heading}</span>
      {!disabled && <span className="fork-box-actions">
        <button type="button" title={fork.feeding ? "Add another branch that feeds in here." : "Add a route with no stages yet. Runs on it skip ahead to where the routes meet."}
          onClick={() => editor.operate("route-add", fork.feeding ? { forkId: fork.id, feeds: true } : { forkId: fork.id })}>{fork.feeding ? "+ Feed" : "+ Route"}</button>
        <button type="button" aria-label={`Remove fork ${fork.id}`} title="Remove the fork. Its stages stay on the belt, one after another." {...editor.problemTarget("fork", fork.id)}
          onClick={() => editor.operate("fork-remove", { forkId: fork.id })}>Remove fork</button>
      </span>}
    </div>
    {fork.routes.map(route => <RouteRow key={route.id} route={route} line={line} disabled={disabled} renderItems={renderItems} />)}
  </section>;
}

function RouteRow({ route, line, disabled, renderItems }: {
  route: StageLinePresentationRouteRow; line: StageLinePresentation; disabled: boolean; renderItems: (list: readonly StageLinePresentationItem[], route?: string) => ReactElement[]
}) {
  const editor = useDraftEditor();
  const name = editor.routeName(route.id);
  return <div className="fork-route" id={`route-${route.id}`}>
    <div className="fork-route-name">
      <span className="fork-route-caption" title={route.help || undefined}>{route.caption}</span>
      <RouteName routeId={route.id} name={name} disabled={disabled} />
      {!disabled && route.removable && <button type="button" className="fork-route-remove" aria-label={`Remove route ${name}`} title={`Remove the ${name} route and its stages`}
        onClick={() => editor.operate("route-remove", { routeId: route.id })}>Remove route</button>}
    </div>
    {renderItems(route.items, route.id)}
    <AddMenu line={line} at={route.addEnd} route={route.id} disabled={disabled} />
    {route.empty && <span className="fork-route-empty">{route.empty}</span>}
  </div>;
}

/** Where an open menu sits: fixed to the viewport beside its "+", on whichever side has more room, no taller than that room or the list. */
interface MenuPlacement { left: number; top?: number; bottom?: number; maxHeight: number }

/** The stage line's scroll box clips anything inside it, so the menu is placed in viewport coordinates from its "+". */
function placeMenu(plus: HTMLElement): MenuPlacement {
  const rect = plus.getBoundingClientRect();
  const margin = 12;
  const halfWidth = 90;
  const gap = 6;
  const below = window.innerHeight - rect.bottom - gap - margin;
  const above = rect.top - gap - margin;
  const left = Math.min(Math.max(rect.left + rect.width / 2, margin + halfWidth), window.innerWidth - margin - halfWidth);
  // Opening upward anchors the card's bottom edge to the "+", so it grows up from there and stays on the page.
  if (below >= above) return { left, top: rect.bottom + gap, maxHeight: Math.max(0, Math.min(320, below)) };
  return { left, bottom: window.innerHeight - rect.top + gap, maxHeight: Math.max(0, Math.min(320, above)) };
}

/** The "+" between blocks: adds an agent, an approval or, where the belt allows one, a watcher here. */
function AddMenu({ line, at, route, disabled }: { line: StageLinePresentation; at: number; route?: string; disabled: boolean }) {
  const editor = useDraftEditor();
  const [placement, setPlacement] = useState<MenuPlacement | null>(null);
  const open = placement !== null;
  const ref = useRef<HTMLDivElement>(null);
  const plus = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent) => { if (!ref.current?.contains(event.target as Node)) setPlacement(null); };
    const key = (event: KeyboardEvent) => { if (event.key === "Escape") setPlacement(null); };
    // A scroll of the page or the stage line moves the "+" away from the fixed card; scrolling the card itself does not.
    const scrolled = (event: Event) => { if (!ref.current?.contains(event.target as Node)) setPlacement(null); };
    const resized = () => setPlacement(null);
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", key);
    document.addEventListener("scroll", scrolled, true);
    window.addEventListener("resize", resized);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", key);
      document.removeEventListener("scroll", scrolled, true);
      window.removeEventListener("resize", resized);
    };
  }, [open]);
  const add = (request: Parameters<typeof editor.addStage>[0]) => { setPlacement(null); void editor.addStage({ ...request, index: at, route }); };
  return <div className="add-menu" ref={ref}>
    <span className="add-menu-track" aria-hidden="true" />
    {!disabled && <button ref={plus} type="button" className="add-menu-plus" aria-haspopup="menu" aria-expanded={open} aria-label="Add a stage here" title="Add a stage here"
      onClick={() => { if (open) setPlacement(null); else if (plus.current) setPlacement(placeMenu(plus.current)); }}>+</button>}
    {placement && <div className="add-menu-card" role="menu" style={{ left: placement.left, top: placement.top, bottom: placement.bottom, maxHeight: placement.maxHeight }}>
      <p className="add-menu-section">Agent</p>
      {line.agents.map(agent => <button key={`${agent.id}@${agent.version}`} type="button" role="menuitem" onClick={() => add({ type: "agent", agent: { id: agent.id, version: agent.version } })}>{agent.name}</button>)}
      <p className="add-menu-section">Waits for you</p>
      <button type="button" role="menuitem" onClick={() => add({ type: "approve" })}>Approval</button>
      <button type="button" role="menuitem" onClick={() => add({ type: "prompt" })}>Prompt</button>
      {line.watchAllowed && <><p className="add-menu-section">Watcher</p>
        <button type="button" role="menuitem" onClick={() => add({ type: "watch" })}>Watch</button></>}
    </div>}
  </div>;
}

/** A route's name, typed into the app-level store and sent on Enter or when the field loses focus. */
function RouteName({ routeId, name, disabled }: { routeId: string; name: string; disabled: boolean }) {
  const editor = useDraftEditor();
  const form = useTypedForm(`route:${routeId}`, { name });
  const typed = form.values.name;
  const rename = () => {
    if (typed.trim() !== "" && typed !== name) editor.operate("route-update", { routeId, routeName: typed }, form.applies());
  };
  return <input className="paper-field fork-route-input" aria-label={`Route name for ${name}`} placeholder="Name" {...editor.problemTarget("route", routeId)}
    value={typed} readOnly={disabled} onChange={event => form.set("name", event.target.value)} onBlur={rename}
    onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); rename(); } }} />;
}
