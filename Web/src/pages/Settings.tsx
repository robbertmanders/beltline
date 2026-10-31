import { useQuery } from "@tanstack/react-query";
import { Segmented, Tag } from "../components/Paper";
import { useEffect } from "react";
import { apiRequest } from "../api/client";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import type { SettingsResponse, ToolsResponse } from "../api/wire.gen";
import { settingsPatchRequest } from "../api/settingsCommands";
import { useSettingsQuery } from "../api/useSettingsQuery";

import { useCommand } from "../api/useCommand";
import { ErrorProblems } from "../components/ProblemList";
import { useRevealLabel, useRuntime } from "../app/runtime";
import { usePreferences } from "../state/usePreferences";
import { AllowedModelsCard } from "./settings/AllowedModelsCard";
import { LibraryRepositoriesCard } from "./settings/LibraryRepositoriesCard";

const appearanceOptions = [
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" }
] as const;

/** Browser-local: it saves through the browser preferences and sends no daemon command. */
function AppearanceControl() {
  const { preferences, update } = usePreferences();
  return <div className="appearance-control">
    <Segmented label="Appearance" options={appearanceOptions} value={preferences.appearance} onChange={appearance => update({ appearance })} />
    <p className="muted-line">Applies to this browser.</p>
  </div>;
}

/** Notify me is the daemon's confirmed setting; the checkbox shows it and saves each change through the settings patch. */
function GeneralCard({ settings }: { settings: SettingsResponse }) {
  const command = useCommand();
  return <section className="settings-card" aria-label="General">
    <p className="paper-tag paper-tag--strong">General</p>
    <AppearanceControl />
    <label className="afk-toggle"><input type="checkbox" checked={settings.notifications} onChange={event => command.submit(settingsPatchRequest({ notifications: event.currentTarget.checked }))} />Notify me when a run needs me</label>
    <p className="muted-line">Send macOS notifications when a run needs you.</p>
    {command.error ? <ErrorProblems error={command.error} /> : null}
  </section>;
}

/**
 * The experimental features, each its own switch: Show the Insights page and Enable shadow tests. Both are the
 * daemon's confirmed settings, saved one at a time through the settings patch. The daemon says whether shadow
 * tests may be turned off now; while a test runs the switch stays on and disabled, with its reason.
 */
function ExperimentalCard({ settings }: { settings: SettingsResponse }) {
  const command = useCommand();
  const shadowLocked = settings.enableShadowTests && !settings.shadowTestsToggle.enabled;
  return <section className="settings-card" aria-label="Experimental">
    <p className="paper-tag paper-tag--strong">Experimental</p>
    <p className="muted-line">Both features are experimental: their figures, layout and controls may still change.</p>
    <label className="afk-toggle"><input type="checkbox" checked={settings.showInsights} onChange={event => command.submit(settingsPatchRequest({ showInsights: event.currentTarget.checked }))} />Show the Insights page</label>
    <p className="muted-line">List Insights in the navigation, with each belt's first-try rate, time and cost.</p>
    <label className="afk-toggle"><input type="checkbox" checked={settings.enableShadowTests} disabled={command.pending || shadowLocked}
      onChange={event => command.submit(settingsPatchRequest({ enableShadowTests: event.currentTarget.checked }))} />Enable shadow tests</label>
    <p className="muted-line">Try a changed belt beside its live version. Accept or discard every running shadow test before turning this off.</p>
    {shadowLocked && settings.shadowTestsToggle.reason ? <p className="muted-line" role="note">{settings.shadowTestsToggle.reason}</p> : null}
    {command.error ? <ErrorProblems error={command.error} /> : null}
  </section>;
}

function DataCard({ settings }: { settings: SettingsResponse }) {
  const revealLabel = useRevealLabel();
  const command = useCommand();
  const reveal = () => command.submit({
    path: "/api/v1/desktop/reveal-data", body: {}, affects: [], publish: () => {}
  });
  return <section className="settings-card" aria-label="Data">
    <p className="paper-tag paper-tag--strong">Data</p>
    <p className="muted-line">{settings.demo
      ? "Demo mode keeps its data in a temporary folder that starts fresh every launch."
      : "Belts, runs, specs and every session’s brief, output and log live here."}</p>
    <div className="data-row"><code>{settings.dataLocation || "Unknown"}</code>
      <button type="button" className="paper-button paper-button--small" disabled={command.pending || !settings.dataLocation} onClick={reveal}>{revealLabel}</button>
    </div>
    {command.error ? <ErrorProblems error={command.error} /> : null}
  </section>;
}

/** Stops the daemon cleanly. Agent sessions keep running, and the LaunchAgent does not restart a clean quit. */
function QuitCard() {
  const command = useCommand();
  const quit = () => command.submit({
    path: "/api/v1/quit", body: {}, affects: [], publish: () => {}
  });
  return <section className="settings-card" aria-label="Quit card">
    <p className="paper-tag paper-tag--strong">Quit</p>
    <p className="muted-line">Stops Beltline in the background. Agent sessions keep running and are picked up when it starts again.</p>
    <button type="button" className="paper-button paper-button--small" disabled={command.pending} onClick={quit}>Quit Beltline</button>
    {command.error ? <ErrorProblems error={command.error} /> : null}
  </section>;
}

function ToolsCard() {
  const runtime = useRuntime();
  const tools = useQuery({
    queryKey: apiQueryKey("/api/v1/tools"),
    queryFn: ({ signal }) => apiRequest<ToolsResponse>("/api/v1/tools", { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/tools") }
  });
  const rows = tools.data?.tools ?? [];
  return <section className="settings-card" aria-label="Tools">
    <header className="card-head">
      <span className="paper-tag paper-tag--strong">Tools</span>
      <button type="button" className="paper-button paper-button--small" disabled={tools.isFetching} onClick={() => void tools.refetch()}>{tools.isFetching ? "Checking…" : "Check again"}</button>
    </header>
    <p className="muted-line">Beltline drives these command-line tools. Agents run in tmux on a private server (tmux -L {tools.data?.tmuxSocket ?? "…"}).</p>
    {tools.isPending && !tools.data && <p className="loading-state" role="status">Checking tools…</p>}
    {tools.error && <ErrorProblems error={tools.error} />}
    {rows.length === 0 && tools.data && <p className="empty-state" role="status">No tools reported.</p>}
    <ul className="tools-list">
      {rows.map(tool => <li key={tool.name} className="tool-row" data-state={tool.ok ? "available" : tool.optional ? "missing-optional" : "missing-required"}>
        <span aria-hidden="true" className="tool-mark">{tool.ok ? "✓" : tool.optional ? "−" : "×"}</span>
        <span className="tool-name">{tool.name}</span>
        <span className="tool-purpose">{tool.purpose}</span>
        <span className="tool-detail" title={tool.detail}>{tool.detail}</span>
      </li>)}
    </ul>
  </section>;
}

export function SettingsPage() {
  const settings = useSettingsQuery();
  useEffect(() => { document.title = "Beltline — Settings"; }, []);
  return <article className="page-content">
    <header className="page-heading"><div className="work-title-block"><Tag>Beltline</Tag><h1>Settings</h1></div></header>
    {settings.isPending && !settings.data && <section className="settings-card"><p className="loading-state" role="status">Loading settings…</p></section>}
    {settings.error && !settings.data && <section className="error-panel"><h2>Unable to load this page</h2><ErrorProblems error={settings.error} /></section>}
    {settings.data && <>
      <AllowedModelsCard />
      <GeneralCard settings={settings.data} />
      <ExperimentalCard settings={settings.data} />
      <DataCard settings={settings.data} />
      <QuitCard />
      <LibraryRepositoriesCard settings={settings.data} />
      <ToolsCard />
      <p className="loaded-state screen-reader-only" role="status">Settings loaded</p>
    </>}
    {settings.isError && settings.data && <p className="stale-error" role="status">Showing the last loaded settings; refresh failed.</p>}
  </article>;
}
