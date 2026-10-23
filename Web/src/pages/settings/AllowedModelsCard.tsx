import { useQuery, type QueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { apiRequest } from "../../api/client";
import { apiQueryKey, resourcesForPath } from "../../api/resources";
import { backendValues } from "../../api/wire.gen";
import type { Backend, ModelRunnerResource } from "../../api/wire.gen";
import { useCommand } from "../../api/useCommand";
import { ErrorProblems } from "../../components/ProblemList";
import { useRuntime } from "../../app/runtime";

/** Allowed models starts collapsed on every visit; the expanded body is the tall part. */
export function AllowedModelsCard() {
  const runtime = useRuntime();
  const [expanded, setExpanded] = useState(false);
  const [runner, setRunner] = useState<Backend>("pi");
  const [search, setSearch] = useState("");
  const command = useCommand();
  // The daemon matches the search (`q`) and labels every catalog model; the card only draws them.
  const modelsPath = `/api/v1/models/${runner}${search ? `?q=${encodeURIComponent(search)}` : ""}`;
  const models = useQuery({
    queryKey: apiQueryKey(`/api/v1/models/${runner}`, search),
    queryFn: ({ signal }) => apiRequest<ModelRunnerResource>(modelsPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/models/:runner") }
  });
  const data = models.data;
  const allowed = new Set(data?.allowedModels ?? []);
  const listed = data?.listing ?? [];
  const unavailable = data?.unavailableModels ?? [];
  const actionFor = (id: string) => data?.actions.candidates.find(candidate => candidate.id === id);
  // The listing depends on the search, so every search's copy of this runner is refetched rather than replaced.
  const publishModel = (queryClient: QueryClient) => {
    void queryClient.invalidateQueries({ queryKey: ["api", `/api/v1/models/${runner}`] });
  };
  const setAllowed = (id: string, allow: boolean) => command.submit({
    path: `/api/v1/models/${runner}/allow`, body: { model: id, allowed: allow }, affects: ["models", "settings"], publish: publishModel
  });
  const refresh = () => command.submit({
    path: `/api/v1/models/${runner}/refresh`, body: {}, affects: ["models", "settings"], publish: publishModel
  });
  const refreshAction = data?.actions.refresh;
  return <section className="settings-card" aria-label="Allowed models" data-column="allowed-models" data-ready={expanded ? "true" : "false"}>
    <header className="card-head">
      <button type="button" className="disclosure" aria-expanded={expanded} aria-label="Allowed models" onClick={() => setExpanded(value => !value)}>
        <span aria-hidden="true" className={`disclosure-mark ${expanded ? "open" : ""}`}>▶</span>
        <span className="paper-tag paper-tag--strong">Allowed models</span>
      </button>
      {expanded && <button type="button" className="paper-button paper-button--small" disabled={!refreshAction?.available || command.pending} onClick={refresh}>Refresh</button>}
    </header>
    {expanded && <>
      <p className="muted-line">Changes apply to new runs and assistant sessions. Existing runs and sessions keep their allowed models.</p>
      <label className="field-row">Harness
        <select className="paper-field" aria-label="Harness" value={runner} onChange={event => setRunner(event.currentTarget.value as Backend)}>
          {backendValues.map(option => <option key={option} value={option}>{option}</option>)}
        </select>
      </label>
      <input className="paper-field" type="search" aria-label="Search models" placeholder="Search models" value={search} onChange={event => setSearch(event.currentTarget.value)} />
      {data?.loading && <p className="muted-line" role="status">Loading models…</p>}
      {models.isPending && !data && <p className="loading-state" role="status">Loading models…</p>}
      {models.error && <><ErrorProblems error={models.error} />{data && <p className="muted-line">Using last successful catalog</p>}</>}
      {command.error ? <ErrorProblems error={command.error} /> : null}
      {data && allowed.size === 0 && <p className="muted-line">Allow models in Settings before starting new runs.</p>}
      {data && listed.length === 0 && unavailable.length === 0 && <p className="empty-state" role="status">No matching models</p>}
      {data && (listed.length > 0 || unavailable.length > 0) && <ul className="model-list">
        {listed.map(choice => {
          const action = actionFor(choice.id);
          const disabled = !action?.available || command.pending;
          return <li key={choice.id} className="model-row" title={choice.id}>
            <label className="model-check" title={action?.refusalReason ?? choice.id}>
              <input type="checkbox" checked={choice.allowed} disabled={disabled} onChange={event => setAllowed(choice.id, event.currentTarget.checked)} />
              <span className="model-row-name">{choice.label}</span>
            </label>
            {action?.refusalReason && <span className="muted-line model-row-reason">{action.refusalReason}</span>}
          </li>;
        })}
        {unavailable.map(id => {
          const action = actionFor(id);
          const disabled = !action?.available || command.pending;
          return <li key={`unavailable-${id}`} className="model-row" title={id}>
            <label className="model-check" title={action?.refusalReason ?? id}>
              <input type="checkbox" checked disabled={disabled} onChange={event => setAllowed(id, event.currentTarget.checked)} />
              <span className="model-row-name">{`${id} — unavailable in catalog`}</span>
            </label>
            {action?.refusalReason && <span className="muted-line model-row-reason">{action.refusalReason}</span>}
          </li>;
        })}
      </ul>}
      {data?.catalog && <p className="muted-line">Refreshed {new Date(data.catalog.refreshedAt).toLocaleString()}</p>}
    </>}
  </section>;
}
