import { keepPreviousData, useQuery, type QueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { apiRequest } from "../api/client";
import { isActionEnabled } from "../api/actions";
import { ApiError, describeError } from "../api/errors";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import { useRefetchWhen } from "../api/useRefetchWhen";
import type { Agent, AgentAction, AgentDetailResponse, AgentDraftResponse, AgentPresentation, AgentRecordResponse, AgentsResponse, Backend, LibraryBrowser as LibraryBrowserData } from "../api/wire.gen";
import { useCommand } from "../api/useCommand";
import { useRuntime } from "../app/runtime";
import { LibraryBrowser, LibraryDetailHeader } from "../components/LibraryBrowser";
import { useElementWidth } from "../components/useElementWidth";
import { AgentEditor } from "./AgentEditor";
import { AssistantOpenButton, AssistantSidebar } from "../components/AssistantSidebar";
import { usePreferences } from "../state/usePreferences";
import { newAgentDraftKey, runnerSwitchDraft, useAgentDrafts } from "../state/agentDrafts";

export const agentsListPath = "/api/v1/agents";

/** Publish the confirmed resource before refetching its daemon-owned browser presentation, in the list and in the open agent's detail. */
function publishAgentResource(client: QueryClient, record: AgentRecordResponse) {
  client.setQueriesData<{ agents: AgentRecordResponse[]; actions: AgentAction[]; browser?: LibraryBrowserData }>({
    predicate: query => query.queryKey[0] === "api" && String(query.queryKey[1]).split("?")[0] === agentsListPath
  }, list => {
    if (!list) return list;
    const previous = list.agents.find(entry => entry.agent.id === record.agent.id);
    const updated = { ...previous, ...record, beltCount: previous?.beltCount };
    return { ...list, agents: previous ? list.agents.map(entry => entry.agent.id === record.agent.id ? updated : entry) : [...list.agents, updated] };
  });
  const detailPath = `${agentsListPath}/${record.agent.id}`;
  client.setQueriesData<{ agent: Agent; archived: boolean }>({
    predicate: query => query.queryKey[0] === "api" && query.queryKey[1] === detailPath
  }, detail => detail && { ...detail, agent: record.agent, archived: record.archived });
}

/** The content comparison the Save button uses; the version is the daemon's to change. */
export function sameAgentContent(a: Agent, b: Agent): boolean {
  return JSON.stringify({ ...a, version: 0 }) === JSON.stringify({ ...b, version: 0 });
}

export function AgentsPage({ onShare = () => undefined }: { onShare?: (id: string, version: number, name?: string) => void }) {
  const runtime = useRuntime();
  const { preferences, update: updatePreferences } = usePreferences();
  const { drafts, putDraft, discardDraft, revision, generation, basis } = useAgentDrafts();
  const canCommand = runtime.canRead && runtime.connection === "connected";
  const commandReason = canCommand ? undefined : "Commands are unavailable until the daemon connection is ready.";
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const agentKey = params.get("agent");
  const search = params.get("search") ?? "";
  const filter = params.get("filter") ?? "all";
  const draft = agentKey ? drafts[agentKey] ?? null : null;
  const [assistantRequest, setAssistantRequest] = useState("");
  const [columnsRef, availableWidth] = useElementWidth<HTMLDivElement>();
  const latestDrafts = useRef(drafts);
  latestDrafts.current = drafts;
  const latestAgentKey = useRef(agentKey);
  latestAgentKey.current = agentKey;
  const latestParams = useRef(params.toString());
  latestParams.current = params.toString();
  const runnerRequests = useRef<Record<string, number>>({});

  const listPath = `${agentsListPath}?search=${encodeURIComponent(search)}&filter=${encodeURIComponent(filter)}`;
  const listQuery = useQuery({
    queryKey: apiQueryKey(listPath),
    queryFn: ({ signal }) => apiRequest<AgentsResponse>(listPath, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    placeholderData: keepPreviousData,
    retry: false,
    meta: { resourceDeps: resourcesForPath(agentsListPath) }
  });
  // Opening an agent from the browser, or coming back to the browser, asks the daemon again, as the page's first load does.
  useRefetchWhen(() => listQuery.refetch(), agentKey);
  const records = listQuery.data?.agents ?? [];
  const activeId = agentKey && draft?.version !== 0 ? agentKey : null;
  const listedRecord = activeId ? records.find(record => record.agent.id === activeId) ?? null : null;

  const detailQuery = useQuery({
    queryKey: apiQueryKey(`/api/v1/agents/${activeId ?? ""}`),
    queryFn: ({ signal }) => apiRequest<AgentDetailResponse>(`/api/v1/agents/${encodeURIComponent(activeId!)}`, { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead && activeId !== null,
    retry: false,
    meta: { resourceDeps: resourcesForPath(`/api/v1/agents/${activeId ?? ""}`) }
  });
  // Only the daemon's answer to the detail read can say the agent is gone: a 404 is a successful read of a missing agent.
  const missingSaved = activeId !== null && detailQuery.isError && detailQuery.error instanceof ApiError && detailQuery.error.status === 404;
  const missingSelectedAgent = draft === null && missingSaved;
  // The saved version is the detail's once it has arrived, so the fields and the presentation describe the same version.
  const savedInfo = activeId !== null && detailQuery.data ? { agent: detailQuery.data.agent } : listedRecord;
  const savedAgent: Agent | null = savedInfo?.agent ?? null;
  const current = draft ?? (missingSelectedAgent ? null : savedAgent);
  const isNew = current?.version === 0;
  const dirty = draft !== null && (savedAgent === null || !sameAgentContent(draft, savedAgent));
  // A draft that holds nothing beyond the saved agent it started from gives way to a newer saved version.
  const startedFrom = agentKey ? basis(agentKey) : undefined;
  const cleanDraft = draft !== null && startedFrom !== undefined && sameAgentContent(draft, startedFrom);
  useEffect(() => {
    if (agentKey && cleanDraft) discardDraft(agentKey);
  }, [agentKey, cleanDraft, discardDraft]);

  const draftPresentationQuery = useQuery({
    queryKey: ["api", "/api/v1/agents/draft/presentation", draft],
    queryFn: ({ signal }) => apiRequest<AgentDraftResponse>("/api/v1/agents/draft/presentation", {
      method: "POST", body: { agent: draft! }, signal, onUnauthorized: runtime.requireSignIn
    }),
    enabled: runtime.canRead && draft !== null,
    placeholderData: keepPreviousData,
    retry: false,
    meta: { resourceDeps: [] }
  });
  const presentation: AgentPresentation | undefined = draft
    ? draftPresentationQuery.data?.presentation
    : detailQuery.data?.presentation;
  const savedActions: AgentAction[] = draft
    ? draftPresentationQuery.data?.actions ?? []
    : detailQuery.data?.actions ?? [];
  // A draft whose saved agent is gone cannot be saved back; Discard is the way out.
  const editorActions = savedActions.filter(action => !(missingSaved && action.id === "save"));
  // The daemon's actions for a fresh draft arrive with its first presentation; until then Save waits for them.
  const actionsLoading = draft ? draftPresentationQuery.data === undefined : detailQuery.data === undefined;
  const [saveRequested, setSaveRequested] = useState(false);
  useEffect(() => {
    if (!saveRequested || actionsLoading) return;
    setSaveRequested(false);
    submitSave();
  });
  const browser: LibraryBrowserData | undefined = listQuery.data?.browser;
  const newActions = agentKey ? (draft ? savedActions : detailQuery.data?.actions ?? listQuery.data?.actions ?? []) : browser?.actions ?? [];

  const newCommand = useCommand();
  const saveCommand = useCommand();
  const archiveCommand = useCommand();
  const unarchiveCommand = useCommand();
  const [runnerError, setRunnerError] = useState<unknown>(null);

  function patchParams(next: Record<string, string | null>) {
    const updated = new URLSearchParams(params);
    for (const [key, value] of Object.entries(next)) value === null ? updated.delete(key) : updated.set(key, value);
    setParams(updated, { replace: Object.hasOwn(next, "search") || Object.hasOwn(next, "filter") });
  }

  function selectAgent(id: string) {
    patchParams({ agent: id });
  }

  function openNew() {
    if (!canCommand || !isActionEnabled(newActions, "new") || newCommand.pending) return;
    const draftKey = newAgentDraftKey();
    const requestView = params.toString();
    const requestSelection = agentKey;
    newCommand.submit({
      path: "/api/v1/agents/new", body: {}, affects: [],
      publish: (_client, agent: Agent) => {
        putDraft(draftKey, agent);
        if (latestAgentKey.current === requestSelection && latestParams.current === requestView) patchParams({ agent: draftKey });
      }
    });
  }

  function requestSave() {
    if (actionsLoading) setSaveRequested(true);
    else submitSave();
  }

  function submitSave(afterSave?: () => void) {
    if (!current || !agentKey || !canCommand || missingSaved || !isActionEnabled(savedActions, "save") || saveCommand.pending) return;
    const requestKey = agentKey;
    const submittedAgent = current;
    saveCommand.submit({
      path: "/api/v1/agents/save", body: { agent: current }, affects: ["library", "settings", "models"],
      publish: (client, record: AgentRecordResponse) => {
        publishAgentResource(client, record);
        const latestDraft = latestDrafts.current[requestKey];
        if (latestDraft === submittedAgent) discardDraft(requestKey);
        else if (latestDraft) {
          putDraft(record.agent.id, { ...latestDraft, version: record.agent.version });
          if (requestKey !== record.agent.id) discardDraft(requestKey);
        }
        if (latestAgentKey.current !== requestKey) return;
        if (requestKey !== record.agent.id) patchParams({ agent: record.agent.id });
        afterSave?.();
      }
    });
  }

  function submitArchive(archived: boolean) {
    const requiredId = archived ? "archive" : "unarchive";
    if (!current || !canCommand || !isActionEnabled(savedActions, requiredId)) return;
    const command = archived ? archiveCommand : unarchiveCommand;
    const target = current.id;
    const requestKey = agentKey;
    const editRevision = requestKey ? revision(requestKey) : 0;
    command.submit({
      path: `/api/v1/agents/${encodeURIComponent(target)}/${archived ? "archive" : "unarchive"}`,
      body: {}, affects: ["library", "settings", "models"],
      publish: (client, record: AgentRecordResponse) => {
        publishAgentResource(client, record);
        if (latestAgentKey.current !== agentKey) return;
        if (requestKey && revision(requestKey) === editRevision) discardDraft(requestKey);
        if (record.archived) patchParams({ agent: null });
      }
    });
  }

  async function changeRunner(runner: Backend, alternateIndex?: number) {
    if (!canCommand) return;
    const key = agentKey;
    const base = draft ?? savedAgent;
    if (!key || !base) return;
    const editRevision = revision(key);
    const draftGeneration = generation(key);
    const request = (runnerRequests.current[key] ?? 0) + 1;
    runnerRequests.current[key] = request;
    try {
      const result = await apiRequest<AgentDraftResponse>("/api/v1/agents/draft/switch-runner", {
        method: "POST", body: { agent: base, runner, alternateIndex }, onUnauthorized: runtime.requireSignIn
      });
      if (runnerRequests.current[key] !== request || generation(key) !== draftGeneration) return;
      if (revision(key) === editRevision) putDraft(key, result.agent, savedAgent ?? undefined);
      else {
        const merged = runnerSwitchDraft(latestDrafts.current[key], base, result.agent, alternateIndex);
        if (!merged) return;
        putDraft(key, merged);
      }
      setRunnerError(null);
    } catch (error) {
      if (latestAgentKey.current === key && runnerRequests.current[key] === request && revision(key) === editRevision) setRunnerError(error);
    }
  }

  const commandError = saveCommand.error ?? newCommand.error ?? archiveCommand.error ?? unarchiveCommand.error ?? runnerError;
  const retry = () => { if (saveCommand.error) saveCommand.retry(); else if (newCommand.error) newCommand.retry(); else if (archiveCommand.error) archiveCommand.retry(); else if (unarchiveCommand.error) unarchiveCommand.retry(); };
  const assistantButton = <AssistantOpenButton open={preferences.assistantOpen} dirty={dirty}
    saveAvailable={dirty && canCommand && !missingSaved && isActionEnabled(savedActions, "save") && !saveCommand.pending}
    onOpenChange={assistantOpen => updatePreferences({ assistantOpen })}
    onSaveAndOpen={afterSave => submitSave(afterSave)} />;
  const back = () => patchParams({ agent: null });
  const body = <div className={`library-page agents-page${agentKey ? " agents-page-detail" : ""}`} ref={columnsRef}>
    {!agentKey ? <LibraryBrowser title="Agents" lead="Your agent library" browser={browser} search={search} filter={filter}
      onSearch={value => patchParams({ search: value || null })} onFilter={value => patchParams({ filter: value === "all" ? null : value })}
      onOpen={selectAgent} onNew={openNew} onRetry={() => void listQuery.refetch()}
      loading={listQuery.isPending && !listQuery.data} error={listQuery.isError ? listQuery.error : undefined}
      commandError={newCommand.error} onRetryCommand={newCommand.retry}
      busy={newCommand.pending || listQuery.isFetching} assistantButton={assistantButton}
      drafts={Object.entries(drafts).map(([id, agent]) => ({ id, name: agent.name.trim() || "Unnamed agent", onOpen: () => selectAgent(id) }))}
      scrollKey="agents" /> : <div className="library-detail agents-editor-column">
      <LibraryDetailHeader backLabel="Back to Agents" onBack={back} />
        <h1 className="screen-reader-only">Agents</h1>
        {listQuery.isPending && !listQuery.data && <p className="loading-state" role="status">Loading agents…</p>}
        {listQuery.isError && listQuery.data && <p className="stale-error" role="status">Showing the last loaded library; refresh failed.</p>}
        {listQuery.isError && !listQuery.data && <section className="error-panel" role="alert"><h2>Unable to load agents</h2><p>{describeError(listQuery.error).message}</p></section>}
        {missingSelectedAgent
          ? <section className="error-panel agent-missing" role="alert"><h2>Agent no longer available</h2><p>This agent is no longer in the library. Choose another agent or refresh the list.</p></section>
          : detailQuery.isError && !detailQuery.data && !draft
          ? <section className="error-panel" role="alert"><h2>Unable to load agent</h2><p>{describeError(detailQuery.error).message}</p><button type="button" className="action-button" onClick={() => void detailQuery.refetch()}>Retry</button></section>
          : current
          ? <>
            {detailQuery.isError && !missingSaved && <p className="stale-error" role="status">{detailQuery.data ? "Showing the last loaded agent; refresh failed." : "The saved agent could not be loaded; your unsent changes are kept."}</p>}
            {missingSaved && draft && <section className="error-panel agent-missing" role="alert"><h2>Agent no longer in the library</h2><p>Your unsent changes are kept below. This agent cannot be saved while it is missing, so discard the changes to finish.</p><button type="button" className="action-button" onClick={() => { if (agentKey) discardDraft(agentKey); }}>Discard changes</button></section>}
            <AgentEditor agent={current} saved={savedInfo} actions={editorActions}
              presentation={presentation} folder={draft ? undefined : detailQuery.data?.folder} isNew={isNew} dirty={dirty} canCommand={canCommand} commandReason={commandReason} commandError={commandError} onRetry={retry}
              onChange={agent => agentKey && putDraft(agentKey, agent, savedAgent ?? undefined)} onChangeRunner={changeRunner} onNew={openNew} onSave={requestSave} actionsLoading={actionsLoading} saving={saveRequested || saveCommand.pending}
              onDiscard={() => { if (agentKey) discardDraft(agentKey); if (isNew) back(); }}
              onArchive={submitArchive}
              onShare={() => onShare(current.id, current.version, current.name)}
              onOpenBelt={beltId => navigate(`/belts?belt=${encodeURIComponent(beltId)}`)}
              origin={detailQuery.data?.origin}
              assistantButton={assistantButton} />
            </>
          : !listQuery.isPending && <section className="error-panel agent-missing"><h2>Agent unavailable</h2><p>Choose an agent from the library or return to the browser.</p><button className="action-button" onClick={back}>Back to Agents</button></section>}
    </div>}
    {preferences.assistantOpen && <AssistantSidebar available={availableWidth || 1080} reserved={agentKey ? 74 + 440 : 460} stored={preferences.assistantWidth}
      onWidthChange={assistantWidth => updatePreferences({ assistantWidth })} onHide={() => updatePreferences({ assistantOpen: false })}
      request={assistantRequest} onRequestChange={setAssistantRequest} dirty={dirty} />}
  </div>;

  return body;
}
