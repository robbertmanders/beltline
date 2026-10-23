import { useQuery } from "@tanstack/react-query";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import type { BeltResourceResponse, InsightsView } from "../../api/wire.gen";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { ErrorProblems } from "../../components/ProblemList";
import { Caps } from "../../components/Paper";
import { Sheet } from "../../components/Sheet";
import { publishBeltResource, shadowDecisionRequest } from "./beltCommands";

export type ShadowDecision = "accept" | "discard";

export interface ShadowDecisionSheetProps {
  beltId: string;
  beltName: string;
  /** The shadow's chip, such as "Shadow v2". */
  shadowLabel: string;
  test: string;
  decision: ShadowDecision;
  onClose: () => void;
  onDone?: (response: BeltResourceResponse) => void;
}

/**
 * Confirms accepting or discarding a belt's shadow. Accept repeats the test's four figures from Insights, so the
 * decision has its numbers beside it; the daemon decides whether the command is allowed when it arrives.
 */
export function ShadowDecisionSheet({ beltId, beltName, shadowLabel, test, decision, onClose, onDone }: ShadowDecisionSheetProps) {
  const runtime = useRuntime();
  const command = useCommand();
  const path = `/api/v1/views/insights?belt=${encodeURIComponent(beltId)}&shadow=${encodeURIComponent(test)}`;
  const insights = useQuery({
    queryKey: apiQueryKey(path),
    queryFn: ({ signal }) => apiRequest<InsightsView>(path, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && decision === "accept",
    retry: false,
    meta: { resourceDeps: resourcesForPath(path) }
  });
  const shadow = insights.data?.shadow;
  const accept = decision === "accept";
  const offline = runtime.connection !== "connected";
  const submit = () => command.submit(shadowDecisionRequest(beltId, decision, (queryClient, response) => {
    publishBeltResource(queryClient, response);
    onDone?.(response);
    onClose();
  }));

  const footer = <>
    <button type="button" className="action-button" onClick={onClose}>Cancel</button>
    <button type="button" className={`action-button ${accept ? "shadow-primary" : "danger"}`} disabled={offline || command.pending} onClick={submit}>
      {accept ? "Accept shadow" : "Discard shadow"}
    </button>
  </>;

  return <Sheet title={accept ? `Accept ${shadowLabel.toLowerCase()}?` : `Discard ${shadowLabel.toLowerCase()}?`} tag={beltName} open width={560} onClose={onClose}
    subtitle={accept ? "The shadow becomes the live version and the test ends." : "New runs all start on the live version again and the test ends."} footer={footer}>
    {accept && shadow && <div className="shadow-sheet-figures" aria-label="The test's figures">
      {shadow.figures.map(figure => <div key={figure.id}>
        <Caps>{figure.label}</Caps>
        <strong>{figure.shadow.value}</strong>
        <span className={`insights-delta is-${figure.delta.tone}`}>{figure.delta.label}</span>
        <Caps>{`${figure.live.value} live · ${figure.shadow.caption}`}</Caps>
      </div>)}
    </div>}
    <ul className="shadow-sheet-list">
      {accept ? <>
        <li>New runs start on the shadow's version.</li>
        <li>Runs already going finish on the version they started with.</li>
        <li>Insights keeps this test under Version.</li>
      </> : <>
        <li>Runs on the shadow keep going on it until they finish.</li>
        <li>The shadow's version stays in the belt's history, so those runs and Insights can still read it.</li>
        <li>You can start another shadow afterwards.</li>
      </>}
    </ul>
    {command.error !== null && command.error !== undefined && <ErrorProblems error={command.error} />}
  </Sheet>;
}
