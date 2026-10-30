import { useDraftEditor } from "./draftEditor";
import { ThenControls, toneLabels } from "../../components/ThenControls";
import { thenActionValues, toneValues } from "../../api/wire.gen";
import type { BeltContext, Outcome, StageMenu, Then } from "../../api/wire.gen";

/** A new outcome's id. The daemon keeps whatever id it is given, so the browser names it. */
function newOutcomeId(): string {
  return crypto.randomUUID();
}

const actionLabels: Record<(typeof thenActionValues)[number], string> = {
  next: "Next stage",
  goto: "Go to a stage",
  route: "Take a route",
  retry: "Run again",
  hold: "Hold for me",
  park: "Park",
  done: "Done",
  ship: "Ship",
  drop: "Drop"
};

/** The wire sends one target as a string and several as a list; the editor reads both as a list. */
export function targetList(value: string | readonly string[] | undefined): string[] {
  if (value === undefined) return [];
  return typeof value === "string" ? [value] : [...value];
}

type StageKind = "agent" | "approve" | "prompt" | "watch";

/**
 * A stage's outcomes, edited locally and sent as one stage-update with Apply. Where each one goes comes from
 * the daemon's menu for the stage: the actions it may take, the stages a `goto` can name with the counterparts a
 * pick adds on every other route of a fork it comes back into, and the routes of the fork after the stage. The
 * browser places the ids the daemon offers and computes none of them.
 */
export function OutcomesEditor({ stageId, kind, outcomes, context, menu, disabled, onChange }: {
  stageId: string;
  kind: StageKind;
  outcomes: readonly Outcome[];
  context: BeltContext | null;
  menu: StageMenu | null;
  disabled: boolean;
  onChange: (outcomes: Outcome[]) => void;
}) {
  const labels = new Map((context?.outcomes ?? []).map(item => [item.outcome, item.label]));
  const update = (index: number, change: (outcome: Outcome) => Outcome) => {
    onChange(outcomes.map((outcome, position) => position === index ? change(outcome) : outcome));
  };
  const addOutcome = () => {
    const when = kind === "watch" ? menu?.watchEvents[0]?.id ?? "" : "";
    onChange([...outcomes, { id: newOutcomeId(), name: "", when, tone: "neutral", then: { do: "hold" } }]);
  };

  // As the Agent editor lays it out: the heading and help with Add outcome beside them, then one row per outcome.
  return <div className="outcomes-editor" aria-label="Outcomes">
    <div className="agent-outcomes-header">
      <div className="form-heading"><span className="paper-tag">Outcomes</span><h3>How it can end</h3>{menu && <p className="agent-lead">{menu.outcomesIntro}</p>}</div>
      <button type="button" className="action-button small" disabled={disabled} onClick={addOutcome}>+ Add outcome</button>
    </div>
    <div className="outcomes-scroll" data-scroll-viewport>
      <table className="outcomes-table">
        <caption className="screen-reader-only">Outcomes</caption>
        <thead><tr>
          <th scope="col">Outcome</th><th scope="col">{menu?.whenTitle ?? "When"}</th><th scope="col">Tone</th><th scope="col">Then</th><th scope="col"><span className="screen-reader-only">Remove</span></th>
        </tr></thead>
        <tbody>
          {outcomes.map((outcome, index) => <OutcomeRow key={outcome.id} stageId={stageId} kind={kind} outcome={outcome} index={index} menu={menu}
            label={labels.get(outcome.id) ?? ""} disabled={disabled}
            onChange={change => update(index, change)}
            onRemove={() => onChange(outcomes.filter((_, position) => position !== index))} />)}
        </tbody>
      </table>
    </div>
  </div>;
}

function OutcomeRow({ stageId, kind, outcome, index, menu, label, disabled, onChange, onRemove }: {
  stageId: string;
  kind: StageKind;
  outcome: Outcome;
  index: number;
  menu: StageMenu | null;
  label: string;
  disabled: boolean;
  onChange: (change: (outcome: Outcome) => Outcome) => void;
  onRemove: () => void;
}) {
  const editor = useDraftEditor();
  const then = outcome.then;
  const setThen = (change: (then: Then) => Then) => onChange(item => ({ ...item, then: change(item.then) }));
  const number = index + 1;
  // A saved action the menu no longer offers stays selectable, so it is never replaced without a choice.
  const actions = menu?.actions.map(action => ({ ...action })) ?? thenActionValues.map(action => ({ id: action, label: actionLabels[action] }));
  if (!actions.some(action => action.id === then.do)) actions.push({ id: then.do, label: actionLabels[then.do] });
  const events = menu?.watchEvents ?? [];
  return <tr id={`outcome-${outcome.id}`} data-outcome={outcome.id} data-tone={outcome.tone}>
    <td><input aria-label={`Outcome ${number} name`} {...editor.problemTarget("outcome", stageId, outcome.id)} value={outcome.name} placeholder="name" disabled={disabled} onChange={event => { const name = event.target.value; onChange(item => ({ ...item, name })); }} /></td>
    <td>{kind === "watch"
      ? <select aria-label={`Outcome ${number} when`} value={outcome.when} disabled={disabled} onChange={event => { const when = event.target.value; onChange(item => ({ ...item, when })); }}>
        {!events.some(item => item.id === outcome.when) && <option value={outcome.when}>{outcome.when}</option>}
        {events.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
      </select>
      : <textarea aria-label={`Outcome ${number} when`} rows={1} value={outcome.when} disabled={disabled}
        placeholder={kind === "approve" || kind === "prompt" ? "Button label" : "Describe when the agent should pick this"}
        onChange={event => { const when = event.target.value; onChange(item => ({ ...item, when })); }} />}
    </td>
    <td><select aria-label={`Outcome ${number} tone`} value={outcome.tone} disabled={disabled} onChange={event => { const tone = event.target.value as Outcome["tone"]; onChange(item => ({ ...item, tone })); }}>
      {toneValues.map(tone => <option key={tone} value={tone}>{toneLabels[tone]}</option>)}
    </select></td>
    <td className="outcome-then" title={label || undefined}>
      <ThenControls number={number} then={then} actions={actions} gate={kind === "approve"} passesOn={kind !== "prompt"} disabled={disabled}
        onAction={action => setThen(current => ({ do: action, ...(current.withOutput ? { withOutput: true } : {}) }))}
        onChange={setThen}
        target={then.do === "goto"
          ? <GotoTargets number={number} then={then} menu={menu} disabled={disabled}
            onPick={gotoTarget => editor.operate("goto-pick", { stageId, outcomeId: outcome.id, gotoTarget })}
            onPickRoute={(routeId, gotoTarget) => editor.operate("goto-route", { stageId, outcomeId: outcome.id, routeId, gotoTarget })} />
          : <RouteTargets number={number} then={then} menu={menu} disabled={disabled} onChange={setThen} />} />
    </td>
    <td className="outcome-remove">
      <button type="button" className="outcome-remove-button" disabled={disabled} aria-label={`Remove outcome ${number}`} title="Remove this outcome" onClick={onRemove}>×</button>
    </td>
  </tr>;
}

/**
 * Where a `goto` goes: the stage it names first, and, for each other route of a fork the jump comes back into, the
 * stage a run that took that route goes to. Picking a stage adds the counterparts the daemon offers for it; a route
 * set to none stays none.
 */
function GotoTargets({ number, then, menu, disabled, onPick, onPickRoute }: {
  number: number;
  then: Then;
  menu: StageMenu | null;
  disabled: boolean;
  /** Sends `goto-pick`: the daemon sets the stage and its counterparts, or none for an empty pick. */
  onPick: (gotoTarget: string) => void;
  /** Sends `goto-route`: the daemon sets the stage a run that took that route goes to. */
  onPickRoute: (routeId: string, gotoTarget: string) => void;
}) {
  const to = targetList(then.to);
  const primary = to[0] ?? "";
  const targets = menu?.gotoTargets ?? [];
  const chosen = targets.find(target => target.id === primary);
  const pick = (id: string) => onPick(id);
  const onRoute = (route: { stages: readonly { id: string }[] }) => to.slice(1).find(id => route.stages.some(stage => stage.id === id)) ?? "";
  const setOnRoute = (route: { id: string }, id: string) => onPickRoute(route.id, id);
  return <span className="goto-targets">
    <select aria-label={`Outcome ${number} stage`} value={primary} disabled={disabled || !menu} onChange={event => pick(event.target.value)}>
      <option value="">Pick a stage</option>
      {primary !== "" && !chosen && <option value={primary}>{`${primary} (not on this belt)`}</option>}
      {targets.map(target => <option key={target.id} value={target.id}>{target.label}</option>)}
    </select>
    {chosen?.others.map(route => <select key={route.route.id} aria-label={`Outcome ${number} on ${route.route.label}`} value={onRoute(route)} disabled={disabled}
      title={`Where a run that took the ${route.route.label} route goes`} onChange={event => setOnRoute(route.route, event.target.value)}>
      <option value="">{`${route.route.label}: none`}</option>
      {route.stages.map(stage => <option key={stage.id} value={stage.id}>{stage.label}</option>)}
    </select>)}
  </span>;
}

/**
 * Which routes a `route` outcome takes: one or several of the fork right after the stage, each named one by one.
 * There is no "all routes" pick, so adding a route to a fork never widens an outcome. A saved route that is
 * unavailable here stays listed until it is deselected.
 */
function RouteTargets({ number, then, menu, disabled, onChange }: {
  number: number;
  then: Then;
  menu: StageMenu | null;
  disabled: boolean;
  onChange: (change: (then: Then) => Then) => void;
}) {
  const selected = targetList(then.route);
  const available = menu?.routesAfter ?? [];
  const unavailable = selected.filter(id => !available.some(route => route.id === id));
  const pathOf = (id: string) => menu?.routePaths.find(route => route.id === id)?.label ?? id;
  const toggle = (id: string, on: boolean) => onChange(current => {
    const routes = targetList(current.route).filter(route => route !== id);
    if (on) routes.push(id);
    return routes.length === 0 ? { ...current, do: "next", route: undefined } : { ...current, do: "route", route: routes };
  });
  return <fieldset className="route-targets" aria-label={`Outcome ${number} routes`}>
    {available.map(route => <label key={route.id} className="belt-inline">
      <input type="checkbox" checked={selected.includes(route.id)} disabled={disabled} onChange={event => toggle(route.id, event.target.checked)} />
      <span>{route.label}</span>
    </label>)}
    {unavailable.map(id => <label key={id} className="belt-inline route-unavailable">
      <input type="checkbox" checked disabled={disabled} onChange={event => toggle(id, event.target.checked)} />
      <span>{`${pathOf(id)} (unavailable here)`}</span>
    </label>)}
    {menu && available.length === 0 && <span className="field-note">No fork after this stage</span>}
  </fieldset>;
}
