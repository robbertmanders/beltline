// Presentation-only helpers for the Overview: which belt stands out, and where a click goes. They decide no
// geometry, membership, run status or ordering; the daemon supplies those. The pin is browser preference, and
// hover is transient page state.

/** The opacity an unrelated floor item takes while a belt is in focus. */
export const dimmedOpacity = 0.35;

/** A pinned belt wins over a hovered one; with no pin, the hovered belt, if any, is the focus. */
export function resolveFocus(pinned: string | null, hovered: string | null): string | null {
  return pinned ?? hovered;
}

/** Whether an item drawn for `belts` stays fully visible under the focus. With no focus, everything does. */
export function includesBelt(belts: readonly string[], focus: string | null): boolean {
  return focus === null || belts.includes(focus);
}

export function focusOpacity(belts: readonly string[], focus: string | null): number {
  return includesBelt(belts, focus) ? 1 : dimmedOpacity;
}

/**
 * The hover after a pointer enters or leaves a belt's entrance or line. A late exit from one belt does not clear
 * the hover another belt has since taken.
 */
export function hoveredAfter(current: string | null, belt: string, pointing: boolean): string | null {
  if (pointing) return belt;
  return current === belt ? null : current;
}

/** The pin the page uses: the saved id when the floor still has that belt, otherwise none. */
export function validPin(pinned: string | null, belts: readonly { id: string }[]): string | null {
  return pinned !== null && belts.some(belt => belt.id === pinned) ? pinned : null;
}

/** Clicking a pinned belt unpins it; clicking any other belt pins that one instead. */
export function togglePin(pinned: string | null, belt: string): string | null {
  return pinned === belt ? null : belt;
}

/** A run's page, opened at the stage that needs a person when the daemon names one. */
export function runHref(runId: string, stage?: string | null): string {
  const path = `/runs/${encodeURIComponent(runId)}`;
  return stage ? `${path}?stage=${encodeURIComponent(stage)}` : path;
}

/** Active Work filtered to one merged stage across every belt. Search and belt are left at their defaults. */
export function stageWorkHref(stageId: string): string {
  return `/work?${new URLSearchParams({ stage: stageId }).toString()}`;
}

/** Active Work for one belt, with the stage and search cleared. */
export function beltWorkHref(beltId: string): string {
  return `/work?${new URLSearchParams({ belt: beltId }).toString()}`;
}
