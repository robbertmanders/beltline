import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import { Link, Outlet, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { apiRequest } from "../api/client";
import { ApiError, SignInRequiredError } from "../api/errors";
import { globalCommandRequest } from "../api/globalCommands";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import type { GlobalAction, StateResponse } from "../api/wire.gen";
import { isGlobalActionId } from "../api/actions";
import { useSettingsQuery } from "../api/useSettingsQuery";
import { useCommand } from "../api/useCommand";
import { ActionControl, type ButtonKind } from "../components/ActionControl";
import { Chip, NavIcon, type NavIconKind } from "../components/Paper";
import { ErrorProblems } from "../components/ProblemList";
import { useRuntime } from "./runtime";
import { clampNavigationWidth, defaultPreferences } from "../state/preferences";
import { usePreferences } from "../state/usePreferences";

interface Destination { label: string; to: string; icon: NavIconKind; count?: (state: StateResponse) => number; hot?: (state: StateResponse) => boolean; hotHelp?: string }

const destinations: readonly Destination[] = [
  { label: "Overview", to: "/overview", icon: "overview" },
  { label: "Work", to: "/work", icon: "work", count: state => state.counts.needsYou, hot: state => state.counts.needsYou > 0 },
  { label: "Insights", to: "/insights", icon: "insights" },
  { label: "Belts", to: "/belts", icon: "belts", count: state => state.navigation.belts },
  { label: "Agents", to: "/agents", icon: "agents", count: state => state.navigation.agents },
  { label: "Library", to: "/library", icon: "library" },
  { label: "AFK", to: "/afk", icon: "afk", count: state => state.navigation.afkToCheck, hot: state => state.navigation.afkToCheck > 0, hotHelp: "decisions to check" }
];
const settingsDestination: Destination = { label: "Settings", to: "/settings", icon: "settings" };

function useStateQuery() {
  const runtime = useRuntime();
  return useQuery({
    queryKey: apiQueryKey("/api/v1/state"),
    queryFn: ({ signal }) => apiRequest<StateResponse>("/api/v1/state", { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/state") }
  });
}

/** The browser's clock, for the date and the lamp's time. */
function useMinute(): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 15_000);
    return () => window.clearInterval(timer);
  }, []);
  return now;
}

export function dateLabel(now: Date): string {
  const day = now.toLocaleDateString("en-US", { weekday: "short" }).toUpperCase();
  const month = now.toLocaleDateString("en-US", { month: "short" }).toUpperCase();
  return `${day} · ${month} ${now.getDate()}`;
}

/** The lamp and time at the foot of the navigation: green when live, gold while checking or paused. The daemon writes the label and hover text. */
function LiveClock({ state }: { state: StateResponse | undefined }) {
  const paused = state?.globallyPaused ?? false;
  const checking = state?.checking ?? false;
  return <span className="live-clock" title={state?.clockTooltip || undefined}>
    <span className={`live-lamp ${paused ? "is-paused" : checking ? "is-checking" : ""}`} aria-hidden="true" />
    <span>{state?.clockLabel ?? ""}</span>
  </span>;
}

function selectedPath(pathname: string): string {
  return pathname.startsWith("/runs/") ? "/work" : pathname;
}

function NavItem({ destination, selected, state }: { destination: Destination; selected: boolean; state: StateResponse | undefined }) {
  const count = state && destination.count ? destination.count(state) : undefined;
  const hot = Boolean(state && destination.hot?.(state));
  const help = hot ? `${count ?? 0} ${destination.hotHelp ?? "need you"}` : undefined;
  return <Link to={destination.to} aria-current={selected ? "page" : undefined} className={`nav-link ${selected ? "selected" : ""}`}
    aria-label={destination.label} aria-description={help ?? (count !== undefined ? String(count) : undefined)} title={help}>
    <NavIcon kind={destination.icon} />
    <span className="nav-label">{destination.label}</span>
    {count !== undefined && <span className={`nav-count ${hot ? "is-hot" : ""}`}>{count}</span>}
  </Link>;
}

/** Drag to resize the navigation; double-click restores its default width. */
function NavigationHandle({ width, onResize }: { width: number; onResize: (width: number) => void }) {
  const origin = useRef<{ x: number; width: number } | null>(null);
  const down = (event: ReactPointerEvent<HTMLDivElement>) => {
    origin.current = { x: event.clientX, width };
    event.currentTarget.setPointerCapture?.(event.pointerId);
  };
  const move = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (!origin.current) return;
    onResize(clampNavigationWidth(origin.current.width + event.clientX - origin.current.x));
  };
  const up = () => { origin.current = null; };
  return <div className="column-handle navigation-handle" style={{ left: width - 5 }} role="separator" aria-orientation="vertical" aria-label="Resize navigation"
    aria-valuenow={Math.round(width)} aria-valuemin={180} aria-valuemax={420} tabIndex={0} title="Drag to resize; double-click to restore the default width"
    onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={up}
    onDoubleClick={() => onResize(defaultPreferences.navigationWidth)}
    onKeyDown={event => {
      if (event.key === "ArrowLeft") onResize(clampNavigationWidth(width - 10));
      if (event.key === "ArrowRight") onResize(clampNavigationWidth(width + 10));
    }} />;
}

function Navigation() {
  const location = useLocation();
  const { preferences, update } = usePreferences();
  const settings = useSettingsQuery();
  const state = useStateQuery().data;
  const now = useMinute();
  // The daemon says whether Library is offered: with no library repository configured it is hidden.
  // Insights is hidden while Settings turns it off.
  const items = destinations.filter(item =>
    !(item.to === "/library" && state && !state.navigation.library.offered) &&
    !(item.to === "/insights" && settings.data?.showInsights === false));
  const current = selectedPath(location.pathname);
  // A dot while the belts and agents differ from the last backup; the daemon decides what counts as changed.
  const changed = state?.navigation.library.changedSinceBackup === true;
  // The global AFK switch lives at the foot of the sidebar; the daemon's action says whether it is offered and why not.
  const afk = state?.actions.find(action => action.id === "afk");
  if (!preferences.navigationOpen) {
    return <aside data-column="navigation" className="navigation navigation-collapsed">
      <button type="button" className="rail-show" onClick={() => update({ navigationOpen: true })} aria-label="Show navigation" title="Show navigation">«</button>
      <span className="rail-vertical rail-workspace" aria-hidden="true">WORKSPACE</span>
      <nav aria-label="Primary navigation" className="nav-rail">
        {items.map(item => <Link key={item.to} to={item.to} aria-label={item.label} title={item.label} aria-current={current === item.to ? "page" : undefined}
          className={`rail-link ${current === item.to ? "selected" : ""}`}><NavIcon kind={item.icon} /></Link>)}
      </nav>
      <div className="rail-bottom">
        {changed && <span className="rail-backup" title="Changed since the last backup" aria-label="Changed since the last backup">●</span>}
        {afk && state && <GlobalControl action={afk} state={state} sidebar={{ icon: <NavIcon kind="afk" />, iconOnly: true }} />}
        <Link to="/settings" aria-label="Settings" title="Settings" aria-current={current === "/settings" ? "page" : undefined}
          className={`rail-link ${current === "/settings" ? "selected" : ""}`}><NavIcon kind="settings" /></Link>
        <span className="rail-vertical rail-clock"><LiveClock state={state} /></span>
      </div>
    </aside>;
  }
  return <><aside data-column="navigation" className="navigation" style={{ width: preferences.navigationWidth }}>
    <div className="window-space" aria-hidden="true" />
    <Link className="brand" to="/overview">Beltline</Link>
    <p className="date-label">[ {dateLabel(now)} ]</p>
    <div className="nav-section-title"><span className="paper-caps">Workspace</span>
      <button type="button" className="quiet-button" onClick={() => update({ navigationOpen: false })} aria-label="Hide navigation" title="Fold navigation away">« Hide</button></div>
    <nav aria-label="Primary navigation" className="nav-links">
      {items.map(item => <NavItem key={item.to} destination={item} selected={current === item.to} state={state} />)}
    </nav>
    <div className="nav-bottom">
      {changed && <div className="nav-backup" title="Your belts and agents changed since the last backup. Back them up in Library.">
        <span className="nav-backup-dot" aria-hidden="true" />
        <span className="nav-backup-label">Changed since backup</span>
      </div>}
      {afk && state && <GlobalControl action={afk} state={state} sidebar={{ icon: <NavIcon kind="afk" />, iconOnly: false }} />}
      <NavItem destination={settingsDestination} selected={current === "/settings"} state={state} />
      <div className="nav-clock">
        <LiveClock state={state} />
        {settings.data?.demo && <Chip tone="gold">DEMO</Chip>}
      </div>
    </div>
  </aside>
  <NavigationHandle width={preferences.navigationWidth} onResize={width => update({ navigationWidth: width })} /></>;
}

/** Search issues, runs and belts: typing opens Work with the query; Escape clears it. */
function SearchField() {
  const location = useLocation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const onWork = location.pathname === "/work";
  const [text, setText] = useState(onWork ? params.get("search") ?? "" : "");
  useEffect(() => {
    if (onWork) setText(params.get("search") ?? "");
  }, [onWork, params]);
  const search = (value: string, replace: boolean) => {
    const next = new URLSearchParams(onWork ? params : undefined);
    if (value) next.set("search", value); else next.delete("search");
    const query = next.toString();
    navigate(`/work${query ? `?${query}` : ""}`, { replace, state: { workSearch: true } });
  };
  return <label className="toolbar-search">
    <input type="search" aria-label="Search issues, runs and belts" placeholder="Search issues, runs and belts" value={text}
      onChange={event => {
        const value = event.currentTarget.value;
        setText(value);
        if (value || onWork) search(value, onWork && location.state?.workSearch === true);
      }}
      onKeyDown={event => {
        if (event.key === "Enter" && text) search(text, onWork);
        if (event.key === "Escape") {
          setText("");
          if (onWork) search("", true);
          event.currentTarget.blur();
        }
      }} />
    {text && <button type="button" className="toolbar-search-clear" title="Clear the search" aria-label="Clear the search"
      onClick={() => { setText(""); if (onWork) search("", true); }}>×</button>}
  </label>;
}

function buttonKind(id: string, state: StateResponse): ButtonKind {
  if (id === "pause") return state.globallyPaused ? "primary" : "slate";
  return "primary";
}

function ActionBar() {
  const stateQuery = useStateQuery();
  const state = stateQuery.data;
  return <div className="global-controls" role="group" aria-label="Global controls">
    {state?.actions.map((action, index) => action.id === "afk" ? null : <GlobalControl key={`${action.id}-${index}`} action={action} state={state} />)}
    {stateQuery.error && !state && <ErrorProblems error={stateQuery.error} />}
  </div>;
}

/**
 * Each control owns its request, so a slow Fetch cannot block Pause or AFK. The sidebar shows AFK as a switch:
 * the same action, checked while the daemon's AFK setting is on.
 */
function GlobalControl({ action, state, sidebar }: { action: GlobalAction; state: StateResponse; sidebar?: { icon: ReactNode; iconOnly: boolean } }) {
  const runtime = useRuntime();
  const command = useCommand();
  const online = runtime.canRead && runtime.connection === "connected";
  // A sign-in failure is shown by the sign-in state itself; every other failure stays visible here.
  const commandError = command.error instanceof SignInRequiredError ? null : command.error;
  const canRetry = commandError instanceof ApiError && (commandError.kind === "network" || commandError.kind === "contract") && online;
  const activate = () => {
    if (!online || command.pending || !action.enabled || !isGlobalActionId(action.id)) return;
    command.submit(globalCommandRequest(action.id, state));
  };
  return <>
    <ActionControl id={action.id} fallbackLabel={sidebar ? "Global AFK" : action.label} action={action} canRead={runtime.canRead}
      connection={runtime.connection} pending={command.pending} kind={buttonKind(action.id, state)} onActivate={activate}
      switchOptions={sidebar && { checked: state.settings.afkEnabled, ...sidebar }} />
    {Boolean(commandError) && <div className="command-failure">
      <ErrorProblems error={commandError} />
      {canRetry && <button type="button" className="quiet-button" onClick={command.retry}>Try again</button>}
    </div>}
  </>;
}

function StatusMessages() {
  const runtime = useRuntime();
  if (runtime.incompatible) return <div className="status-message version-message" role="alert"><span>API version changed — reload required</span><button type="button" onClick={runtime.reloadDocument}>Reload</button></div>;
  if (runtime.connection === "offline") return <div className="status-message offline-message" role="status">Offline — reconnecting</div>;
  return null;
}

function MainState() {
  const runtime = useRuntime();
  if (runtime.authRequired) return <article className="page-content"><header className="page-heading"><div><p className="eyebrow">LOCAL DAEMON</p><h1>Sign in required</h1></div></header><section className="resource-card"><h2>Open the daemon sign-in link</h2><p>Open the daemon’s <code>/auth?token=…</code> link in this browser to exchange the token for its local session cookie, then return to the app.</p><p className="muted-copy">Credentials are kept in the daemon’s HttpOnly cookie and are not stored in browser preferences.</p></section></article>;
  if (runtime.incompatible) return <article className="page-content"><header className="page-heading"><h1>API compatibility</h1></header><section className="resource-card"><p>Reload this page to use a client that supports API version {runtime.apiVersion}.</p></section></article>;
  return <Outlet />;
}

export function Shell() {
  const runtime = useRuntime();
  return <div className="app-frame" data-connection-status={runtime.connection}>
    <Navigation />
    <div className="main-column" data-column="main">
      <header className="topbar"><SearchField /><ActionBar /></header>
      <StatusMessages />
      {runtime.versionLoading && !runtime.authRequired && <div className="version-loading" role="status">Checking API version…</div>}
      <main className="content"><MainState /></main>
    </div>
  </div>;
}
