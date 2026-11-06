import { ApiError, describeError } from "../../api/errors";
import type { ProblemLocation } from "../../api/wire.gen";

/** The key a field carries in `data-problem-target`, and a located problem names: kind, id and field. */
export function locationKey(kind: ProblemLocation["kind"], id?: string, field?: string): string {
  return `${kind}:${id ?? ""}:${field ?? ""}`;
}

/** The DOM id of the summary's entry for one problem, which the field it is about names in `aria-describedby`. */
export function problemDomId(index: number): string {
  return `belt-problem-${index}`;
}

/** The located problems of a failure by the field they are about, each with the summary entry that describes it. */
export function locatedProblems(failure: unknown): Map<string, { domId: string; message: string }> {
  const located = new Map<string, { domId: string; message: string }>();
  if (!(failure instanceof ApiError)) return located;
  failure.locations.forEach((location, index) => {
    if (!location) return;
    const key = locationKey(location.kind, location.id, location.field);
    if (!located.has(key)) located.set(key, { domId: problemDomId(index), message: failure.problems[index] ?? "" });
  });
  return located;
}

function describeLocation(location: ProblemLocation): string {
  switch (location.kind) {
    case "repository": return `repository ${location.field ?? ""}`.trim();
    case "outcome": return "outcome";
    case "belt": return location.field === "base" ? "base branch" : "belt name";
    case "stage": return location.field === "route" ? "stage route" : "stage";
    default: return location.kind;
  }
}

/** Focuses the field a location names, once the form that holds it is open. */
export function focusLocation(location: ProblemLocation): void {
  const byKey = (key: string) => Array.from(document.querySelectorAll<HTMLElement>("[data-problem-target]")).find(element => element.dataset.problemTarget === key) ?? null;
  // A field the form does not draw on its own falls back to the part it belongs to.
  const find = () => byKey(locationKey(location.kind, location.id, location.field)) ?? (location.field ? byKey(locationKey(location.kind, location.id)) : null);
  // A stage's form opens on the next render, so the field is looked for again for a moment.
  let tries = 0;
  const attempt = () => {
    const element = find();
    if (element) { element.scrollIntoView?.({ block: "center" }); element.focus(); return; }
    if (++tries < 10) window.setTimeout(attempt, 30);
  };
  attempt();
}

/**
 * The failure's message and its problems in the daemon's order. A problem the daemon located links to its
 * field, revealing a stage's form first, and the field is marked invalid and described by the problem; a
 * problem with no field stays in the summary without one. The text is rendered as text, never as markup.
 */
export function ProblemSummary({ failure, onDismiss, onReveal }: { failure: unknown; onDismiss: () => void; onReveal: (location: ProblemLocation) => void }) {
  if (failure === null || failure === undefined) return null;
  const { message, problems } = describeError(failure);
  const locations = failure instanceof ApiError ? failure.locations : [];
  const reveal = (location: ProblemLocation) => {
    onReveal(location);
    focusLocation(location);
  };
  return <section className="belt-problem" role="alert" aria-label="Problems">
    <div>
      <p className="problem-message">{message}</p>
      {problems.length > 0 && <ol>
        {problems.map((problem, index) => {
          const location = locations[index] ?? null;
          return <li key={index} id={problemDomId(index)}>
            <span>{problem}</span>
            {location ? <button type="button" className="link" onClick={() => reveal(location)}>Show {describeLocation(location)}</button>
              : <span className="problem-unlocated"> (not about one field)</span>}
          </li>;
        })}
      </ol>}
    </div>
    <button type="button" className="link" onClick={onDismiss}>Dismiss</button>
  </section>;
}
