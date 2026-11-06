import type { ReactNode } from "react";
import { Sheet } from "./Sheet";

/**
 * A sheet over the whole window that lays itself out: the menu, a new task. It is a modal Sheet without the
 * heading, so focus, Escape and the inert page behave as every sheet's do.
 */
export function Layer({ label, onClose, inverse = false, children }: { label: string; onClose: () => void; inverse?: boolean; children: ReactNode }) {
  return <Sheet title={label} open bare inverse={inverse} onClose={onClose}>{children}</Sheet>;
}
