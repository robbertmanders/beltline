import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { apiRequest } from "../../api/client";
import { isActionEnabled } from "../../api/actions";
import { apiQueryKey, resourcesForPath } from "../../api/resources";

import { useRefetchWhen } from "../../api/useRefetchWhen";
import type { BeltsResponse, LibraryBrowser as LibraryBrowserData } from "../../api/wire.gen";
import { beltList } from "./beltRecords";
import { useCommand } from "../../api/useCommand";
import { useRuntime } from "../../app/runtime";
import { AssistantOpenButton, AssistantSidebar } from "../../components/AssistantSidebar";
import { useElementWidth } from "../../components/useElementWidth";
import { LibraryBrowser, LibraryDetailHeader } from "../../components/LibraryBrowser";
import { ErrorProblems } from "../../components/ProblemList";
import { beltsQueryKey, newRequest } from "./beltCommands";
import { BeltEditor } from "./BeltEditor";
import { sameBelt, useBeltDrafts, type BeltDraft } from "./beltDrafts";
import { usePreferences } from "../../state/usePreferences";

const endpoint = "/api/v1/belts";

function rowLabel(name: string): string {
  return name.trim() === "" ? "Unnamed belt" : name;
}

/**
 * Belts opens on the daemon-computed library browser. The `belt` query parameter opens the existing
 * editor, so deep links and browser history keep their meaning. Unsent drafts remain in the app-level store.
 */
export function BeltsPage({ onShare = () => undefined }: { onShare?: (id: string, version: number, name?: string) => void }) {
  const runtime = useRuntime();
  const drafts = useBeltDrafts();
  const command = useCommand();
  const { preferences, update: updatePreferences } = usePreferences();
  const [searchParams, setSearchParams] = useSearchParams();
  const [columnsRef, availableWidth] = useElementWidth<HTMLDivElement>();
  const assistantSave = useRef<((afterSave?: () => void) => void) | null>(null);
  const [editorState, setEditorState] = useState({ dirty: false, saveAvailable: false });
  const [assistantRequest, setAssistantRequest] = useState("");

  const search = searchParams.get("search") ?? "";
  const filter = searchParams.get("filter") ?? "all";
  const selectedKey = searchParams.get("belt");
  // `shadow=1` shows the selected belt's shadow, whose unsent draft is kept under its own key.
  const shadowMode = selectedKey !== null && searchParams.get("shadow") === "1";
  const draftKey = selectedKey !== null && shadowMode ? `shadow:${selectedKey}` : selectedKey;
  const selectedRef = useRef(selectedKey);
  selectedRef.current = selectedKey;
  const searchParamsRef = useRef(new URLSearchParams(searchParams));
  searchParamsRef.current = new URLSearchParams(searchParams);

  const onAssistantState = useCallback((state: { dirty: boolean; saveAvailable: boolean; save: (then?: () => void) => void } | null) => {
    assistantSave.current = state?.save ?? null;
    setEditorState(state ? { dirty: state.dirty, saveAvailable: state.saveAvailable } : { dirty: false, saveAvailable: false });
  }, []);

  const editorQuery = useQuery({
    queryKey: beltsQueryKey,
    queryFn: ({ signal }) => apiRequest<BeltsResponse>(endpoint, { signal, onUnauthorized: runtime.requireSignIn }).then(beltList),
    enabled: runtime.canRead && selectedKey !== null,
    retry: false,
    meta: { resourceDeps: resourcesForPath(endpoint) }
  });
  // Selecting a belt asks the daemon for the list again, and returning to the browser does too, as a first load does.
  useRefetchWhen(() => editorQuery.refetch(), selectedKey, selectedKey !== null);
  const browsePath = `${endpoint}?search=${encodeURIComponent(search)}&filter=${encodeURIComponent(filter)}`;
  const browserQuery = useQuery({
    queryKey: apiQueryKey(endpoint, { search, filter }),
    queryFn: ({ signal }) => apiRequest<BeltsResponse>(browsePath, { signal, onUnauthorized: runtime.requireSignIn }).then(beltList),
    enabled: runtime.canRead && selectedKey === null,
    retry: false,
    meta: { resourceDeps: resourcesForPath(endpoint) }
  });
  useRefetchWhen(() => browserQuery.refetch(), selectedKey, selectedKey === null);

  const records = editorQuery.data?.belts ?? [];
  const liveIds = editorQuery.data?.liveIds ?? [];
  const unsaved = Object.values(drafts.drafts).filter(draft => draft.unsaved);
  const draftChips = Object.values(drafts.drafts);
  const defaultKey = liveIds[0] ?? unsaved[0]?.key ?? null;

  useEffect(() => { document.title = "Beltline — Belts"; }, []);
  const selectedDraft = draftKey === null ? undefined : drafts.drafts[draftKey];
  const typedHere = draftKey !== null && Object.keys(drafts.inputs[draftKey] ?? {}).length > 0;
  // Unsent work is typed form input, or a draft that is new or differs from the saved draft it started from.
  const unsent = typedHere || (selectedDraft !== undefined && (selectedDraft.unsaved || selectedDraft.basis === undefined
    || !sameBelt(selectedDraft.belt, selectedDraft.basis)));
  const selectedRecord = records.find(item => item.belt.id === selectedKey);
  // The definition the editor opens when nothing is unsent: the shadow's saved version, or for a new shadow a copy of live.
  const savedShadow = selectedRecord?.shadow?.draft;
  const openedDraft = shadowMode ? savedShadow ?? selectedRecord?.draft : selectedRecord?.draft;
  // Only a successful read of the list that lacks the belt says it is gone; a failed refresh says nothing about it.
  const missing = editorQuery.isSuccess && selectedKey !== null && selectedRecord === undefined && (shadowMode || selectedDraft?.unsaved !== true);
  const cleanDraft = draftKey !== null && selectedDraft !== undefined && !unsent;
  // A draft with nothing unsent gives way to the saved belt, so a newer saved version shows.
  useEffect(() => {
    if (draftKey !== null && cleanDraft) drafts.update(draftKey, () => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draftKey, cleanDraft]);
  // Typed input belongs to the saved version it was typed over. That version becomes the belt's draft, so the input
  // stays with the belt through a refresh, navigation and a removal from the library, and the draft is listed.
  useEffect(() => {
    if (draftKey === null || !typedHere || selectedDraft !== undefined || openedDraft === undefined) return;
    const fresh = shadowMode && savedShadow === undefined;
    drafts.update(draftKey, () => ({ key: draftKey, belt: openedDraft, unsaved: fresh, sourceKey: null, basis: fresh ? undefined : openedDraft }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draftKey, typedHere, selectedDraft, openedDraft]);

  if (runtime.authRequired || runtime.incompatible) return null;

  const patchSearchParams = (patch: { belt?: string | null; search?: string; filter?: string; shadow?: boolean }, replace = false) => {
    const next = new URLSearchParams(searchParamsRef.current);
    if ("belt" in patch) patch.belt ? next.set("belt", patch.belt) : next.delete("belt");
    if (patch.shadow !== undefined) patch.shadow ? next.set("shadow", "1") : next.delete("shadow");
    if (patch.search !== undefined) patch.search ? next.set("search", patch.search) : next.delete("search");
    if (patch.filter !== undefined) patch.filter === "all" ? next.delete("filter") : next.set("filter", patch.filter);
    setSearchParams(next, { replace });
  };
  const select = (key: string | null) => patchSearchParams({ belt: key, shadow: false });
  const setShadowMode = (on: boolean) => patchSearchParams({ shadow: on });
  /** A command's answer names the key it belongs to; selection moves only while the user stays on its source. */
  const followSelection = (from: string | null, to: string) => {
    if (selectedRef.current === from) select(to);
  };

  const discardMissing = () => {
    if (draftKey === null) return;
    drafts.update(draftKey, () => undefined);
    drafts.dropTyped(draftKey);
    drafts.selectStage(draftKey, null);
  };

  const newBelt = () => {
    const allowed = isActionEnabled(browserQuery.data?.browser?.actions ?? [], "new");
    if (!runtime.canRead || command.pending || !allowed) return;
    const from = selectedRef.current;
    const key = drafts.nextKey();
    command.submit(newRequest((_queryClient, response) => {
      drafts.setWarnings(key, response.warnings);
      drafts.update(key, () => ({ key, belt: response.draft, unsaved: true, sourceKey: null }));
      followSelection(from, key);
    }));
  };

  const assistantButton: ReactNode = <AssistantOpenButton open={preferences.assistantOpen} dirty={editorState.dirty} saveAvailable={editorState.saveAvailable}
    onOpenChange={assistantOpen => updatePreferences({ assistantOpen })}
    onSaveAndOpen={afterSave => assistantSave.current?.(afterSave)} />;

  const renderEditor = () => {
    // Unsent input is shown as it is; otherwise the saved belt, with its legacy stage keys filled in, as the daemon's draft.
    // A shadow not saved yet is new: Save starts its test.
    const shown: BeltDraft | undefined = unsent && selectedDraft ? selectedDraft
      : selectedRecord && openedDraft && { key: draftKey!, belt: openedDraft, unsaved: shadowMode && savedShadow === undefined, sourceKey: null };
    if (!shown) {
      if (editorQuery.isError && !editorQuery.data) {
        return <section className="error-panel" role="alert"><h2>Unable to load belts</h2><ErrorProblems error={editorQuery.error} /></section>;
      }
      if (!editorQuery.data) {
        return <p className="loading-state" role="status">{runtime.canRead ? "Loading the selected belt…" : "Waiting for the daemon connection…"}</p>;
      }
      // A failed refresh that leaves the belt out is not a deletion: only a successful read says so.
      if (editorQuery.isError) return <section className="error-panel" role="alert"><h2>Unable to load belts</h2><ErrorProblems error={editorQuery.error} /></section>;
      return <section className="empty-panel" role="status">
        <h2>This belt is missing</h2><p>It may have been deleted. Choose another belt.</p>
        {defaultKey !== null && <button type="button" className="btn" onClick={() => select(defaultKey)}>Show the first belt</button>}
      </section>;
    }
    // Every editor path returns the same shape, so the editor keeps its state as a draft appears or goes.
    return <>
      {missing && <section className="error-panel belt-missing-draft" role="alert">
        <h2>This belt is no longer in the library</h2>
        <p>Your unsent changes are kept below. This belt cannot be saved while it is missing, so discard the changes to finish.</p>
        <button type="button" className="btn" onClick={discardMissing}>Discard changes</button>
      </section>}
      <BeltEditor key={draftKey} base={shown} snapshot={selectedRecord ?? null} missing={missing} allIds={records.map(item => item.belt.id)}
        liveIds={liveIds} onSelect={select} followSelection={followSelection} onShare={onShare} onAssistantState={onAssistantState} assistantButton={assistantButton}
        shadow={shadowMode ? { summary: selectedRecord?.shadow ?? null } : undefined} onShadowMode={selectedRecord ? setShadowMode : undefined} />
    </>;
  };

  const renderBrowser = () => {
    const browserData: LibraryBrowserData | undefined = browserQuery.data?.browser;
    const loading = !browserQuery.data && browserQuery.isPending;
    const error = browserQuery.isError ? browserQuery.error : undefined;
    return <LibraryBrowser title="Belts" lead="A belt is a trigger and its stages. Each save makes a new version; runs already on a belt keep the version they started on."
      browser={browserData} search={search} filter={filter}
      onSearch={value => patchSearchParams({ search: value }, true)}
      onFilter={value => patchSearchParams({ filter: value }, true)}
      onOpen={id => select(id)} onNew={newBelt} onRetry={() => void browserQuery.refetch()}
      busy={command.pending || browserQuery.isFetching} loading={loading} error={error} assistantButton={assistantButton}
      commandError={command.error} onRetryCommand={command.retry}
      drafts={draftChips.map(draft => draft.key.startsWith("shadow:")
        ? { id: draft.key, name: `${rowLabel(draft.belt.name)} · shadow`, onOpen: () => patchSearchParams({ belt: draft.key.slice("shadow:".length), shadow: true }) }
        : { id: draft.key, name: rowLabel(draft.belt.name), onOpen: () => select(draft.key) })}
      scrollKey="belts" />;
  };

  const hasSelection = selectedKey !== null;
  const mainContent = <>
    {hasSelection && command.error !== null && command.error !== undefined && <div className="belt-problem">
      <ErrorProblems error={command.error} />
      <button type="button" className="link" onClick={command.clearError}>Dismiss</button>
    </div>}
    {hasSelection && editorQuery.isError && editorQuery.data !== undefined && <div className="belts-stale">
      <p className="stale-error" role="status">Showing the last loaded belts; refresh failed.</p><ErrorProblems error={editorQuery.error} />
    </div>}
    {hasSelection ? <LibraryDetailHeader backLabel="Back to Belts" onBack={() => select(null)} /> : null}
    {hasSelection ? renderEditor() : renderBrowser()}
  </>;

  return <div className={`belts-page library-page ${hasSelection ? "has-detail" : ""}`} ref={columnsRef}>
    {hasSelection ? <div className="belts-main library-detail" data-column="belts-main">
      <h1 className="screen-reader-only">Belts</h1>
      {editorQuery.data !== undefined && <p className="loaded-state screen-reader-only" role="status">Belts loaded</p>}
      {mainContent}
    </div> : <div className="library-browser-column">{mainContent}</div>}
    {preferences.assistantOpen && <AssistantSidebar available={availableWidth || 1080} reserved={74 + 320} stored={preferences.assistantWidth}
      onWidthChange={assistantWidth => updatePreferences({ assistantWidth })} onHide={() => updatePreferences({ assistantOpen: false })}
      request={assistantRequest} onRequestChange={setAssistantRequest} dirty={editorState.dirty} />}
  </div>;
}
