import { Sheet } from "../../components/Sheet";

export interface AgentRef {
  id: string;
  version: number;
}

/**
 * The entry to a stage's agent. The agent's own editor belongs to the Agents page; this sheet decides how the belt's
 * unsaved changes are handled first: open without saving, or save and then open. A failed save keeps the editor open
 * and does not open the agent.
 */
export function AgentLeave({ agent, dirty, canSave, onClose, onOpen, onSaveAndOpen }: {
  agent: AgentRef;
  dirty: boolean;
  canSave: boolean;
  onClose: () => void;
  onOpen: () => void;
  onSaveAndOpen: () => void;
}) {
  const footer = <>
    <button type="button" className="btn" onClick={onClose}>Cancel</button>
    <button type="button" className="btn" onClick={onOpen}>{dirty ? "Open without saving" : "Open agent"}</button>
    {dirty && <button type="button" className="btn fill" disabled={!canSave} onClick={onSaveAndOpen}>Save and open</button>}
  </>;
  return <Sheet title={`Open agent ${agent.id}`} open onClose={onClose} footer={footer}>
    <p>{dirty ? "This belt has unsaved changes. The agent opens in its own editor; choose whether to save the belt first." : "The agent opens in its own editor."}</p>
  </Sheet>;
}
