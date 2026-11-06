import { useQuery } from "@tanstack/react-query";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { apiRequest } from "../api/client";
import { ApiError, SignInRequiredError } from "../api/errors";
import { globalCommandRequest } from "../api/globalCommands";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import type { GlobalAction, StateResponse } from "../api/wire.gen";
import { isGlobalActionId } from "../api/actions";
import { useSettingsQuery } from "../api/useSettingsQuery";
import { useCommand } from "../api/useCommand";
import { ActionControl, type ButtonKind } from "../components/ActionControl";
import { Icon } from "../components/Icon";
import { ErrorProblems } from "../components/ProblemList";
import { NewTaskSheet } from "../components/NewTaskSheet";
import { Palette } from "../components/Palette";
import { Layer } from "../components/Layer";
import { rememberClick } from "../components/reveal";
import { useRuntime } from "./runtime";

export interface Destination { label: string; to: string; count?: (state: StateResponse) => number; help?: string }

/** The pages, in the order the navigation lists them. A count shows only while something there waits for you. */
export const destinations: readonly Destination[] = [
  { label: "Overview", to: "/overview" },
  { label: "Work", to: "/work", count: state => state.counts.needsYou, help: "need you" },
  { label: "Insights", to: "/insights" },
  { label: "Belts", to: "/belts" },
  { label: "Agents", to: "/agents" },
  { label: "Library", to: "/library" },
  { label: "AFK", to: "/afk", count: state => state.navigation.afkToCheck, help: "decisions to check" },
  { label: "Settings", to: "/settings" }
];

export function useStateQuery() {
  const runtime = useRuntime();
  return useQuery({
    queryKey: apiQueryKey("/api/v1/state"),
    queryFn: ({ signal }) => apiRequest<StateResponse>("/api/v1/state", { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/state") }
  });
}

/** The date as a sentence, such as "Thursday, November 5". */
export function dateLabel(now: Date): string {
  const day = now.toLocaleDateString("en-US", { weekday: "long" });
  const month = now.toLocaleDateString("en-US", { month: "long" });
  return `${day}, ${month} ${now.getDate()}`;
}

function selectedPath(pathname: string): string {
  if (pathname.startsWith("/runs/")) return "/work";
  return "/" + (pathname.split("/")[1] ?? "");
}

/** The pages the navigation offers: the daemon says whether Library is offered, and Settings can turn Insights off. */
export function useDestinations(): readonly Destination[] {
  const settings = useSettingsQuery();
  const state = useStateQuery().data;
  return destinations.filter(item =>
    !(item.to === "/library" && state && !state.navigation.library.offered) &&
    !(item.to === "/insights" && settings.data?.showInsights === false));
}

/** The navigation's pill: the current page is filled with the ink, and the fill slides to the next page. */
function NavPill({ items, current, state }: { items: readonly Destination[]; current: string; state: StateResponse | undefined }) {
  const nav = useRef<HTMLElement>(null);
  const blob = useRef<HTMLSpanElement>(null);
  const placed = useRef(false);
  useLayoutEffect(() => {
    const target = nav.current?.querySelector<HTMLElement>('[aria-current="page"]');
    const fill = blob.current;
    if (!fill) return;
    if (!target) { fill.style.width = "0px"; return; }
    if (!placed.current) fill.style.transition = "none";
    fill.style.left = `${target.offsetLeft}px`;
    fill.style.width = `${target.offsetWidth}px`;
    if (!placed.current) { void fill.offsetWidth; fill.style.transition = ""; placed.current = true; }
  });
  return <nav className="nav" aria-label="Primary navigation" ref={nav}>
    <span className="blob" ref={blob} aria-hidden="true" />
    {items.map(item => {
      const count = state && item.count ? item.count(state) : 0;
      return <Link key={item.to} to={item.to} aria-current={current === item.to ? "page" : undefined}
        aria-label={item.label} aria-description={count ? `${count} ${item.help}` : undefined}>
        {item.label}{count > 0 && <span className="bd" title={`${count} ${item.help}`}>{count}</span>}
        {item.to === "/library" && state?.navigation.library.changedSinceBackup && <span className="nav-mark" title="Your belts and agents changed since the last backup. Back them up in Library."><span className="vh">Changed since backup</span></span>}
      </Link>;
    })}
  </nav>;
}

function buttonKind(id: string, state: StateResponse): ButtonKind {
  if (id === "pause") return state.globallyPaused ? "primary" : "plain";
  return "plain";
}

/**
 * Each global control owns its request, so a slow Fetch cannot block Pause. AFK is switched on its own page;
 * the top bar says when it is on.
 */
export function GlobalControl({ action, state, small = true, label, iconOnly = false, asSwitch = false }: { action: GlobalAction; state: StateResponse; small?: boolean; label?: string; iconOnly?: boolean; asSwitch?: boolean }) {
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
    <ActionControl id={action.id} fallbackLabel={asSwitch ? "Global AFK" : label ?? action.label} action={label ? { ...action, label } : action} canRead={runtime.canRead} small={small}
      connection={runtime.connection} pending={command.pending} kind={buttonKind(action.id, state)} onActivate={activate}
      switchOptions={asSwitch ? { checked: state.settings.afkEnabled, icon: null, iconOnly: false, text: "AFK" } : undefined}
      icon={iconOnly ? <Icon name={action.id === "pause" ? (state.globallyPaused ? "play" : "pause") : action.id === "check-now" ? "sync" : "afk"} /> : undefined} />
    {Boolean(commandError) && <div className="command-failure">
      <ErrorProblems error={commandError} />
      {canRetry && <button type="button" className="link" onClick={command.retry}>Try again</button>}
    </div>}
  </>;
}

function TopBar({ onMenu, onPalette }: { onMenu: () => void; onPalette: () => void }) {
  const location = useLocation();
  const stateQuery = useStateQuery();
  const state = stateQuery.data;
  const settings = useSettingsQuery();
  const items = useDestinations();
  const current = selectedPath(location.pathname);
  return <header className="top">
    <Link className="mk" to="/overview"><i aria-hidden="true" />Beltline</Link>
    <NavPill items={items} current={current} state={state} />
    <div className="tr">
      {settings.data?.demo && <span className="flag f hide-s">Demo</span>}
      <span className={`clk hide-s ${state?.globallyPaused ? "is-paused" : state?.checking ? "is-checking" : ""}`} title={state?.clockTooltip || undefined}>{state?.clockLabel ?? ""}</span>
      <div className="global-controls" role="group" aria-label="Global controls">
        {state?.actions.map((action, index) => action.id === "afk" ? null : <GlobalControl key={`${action.id}-${index}`} action={action} state={state} iconOnly />)}
        {state?.actions.filter(action => action.id === "afk").map(action => <GlobalControl key="afk" action={action} state={state} asSwitch />)}
        {stateQuery.error && !state && <ErrorProblems error={stateQuery.error} />}
      </div>
      <button type="button" className="btn sm ghost icon" onClick={onPalette} aria-label="Search and commands (⌘K)" title="Search and commands (⌘K)"><Icon name="search" /></button>
      <button type="button" className="btn sm ghost menu-b" onClick={onMenu}><Icon name="menu" />Menu</button>
    </div>
  </header>;
}

/** Below 1180px the pages move into a sheet that covers the window. */
function MenuSheet({ onClose, onPalette }: { onClose: () => void; onPalette: () => void }) {
  const location = useLocation();
  const state = useStateQuery().data;
  const items = useDestinations();
  const current = selectedPath(location.pathname);
  return <Layer label="Pages" onClose={onClose}>
    <nav className="mnav" aria-label="Pages">
      {items.map(item => {
        const count = state && item.count ? item.count(state) : 0;
        return <Link key={item.to} to={item.to} onClick={onClose} aria-current={current === item.to ? "page" : undefined}>{item.label}{count > 0 && <span className="bd soft">{count}</span>}</Link>;
      })}
    </nav>
    <div className="acts" style={{ marginTop: 40 }}>
      <button type="button" className="btn" onClick={() => { onClose(); onPalette(); }}><Icon name="search" />Search</button>
    </div>
  </Layer>;
}

function StatusMessages() {
  const runtime = useRuntime();
  if (runtime.incompatible) return <div className="status-message" role="alert"><span>API version changed — reload required</span><button type="button" className="btn sm" onClick={runtime.reloadDocument}>Reload</button></div>;
  if (runtime.connection === "offline") return <div className="status-message" role="status">Offline — reconnecting</div>;
  return null;
}

function MainState() {
  const runtime = useRuntime();
  if (runtime.authRequired) return <div className="pin"><header className="ph"><h1 className="h1" aria-label="Sign in required">Sign in required.<br /><span className="soft">Open the daemon’s link.</span></h1>
    <p className="lede">Open the daemon’s <code>/auth?token=…</code> link in this browser to exchange the token for its local session cookie, then return to the app. Credentials are kept in the daemon’s HttpOnly cookie and are not stored in browser preferences.</p></header></div>;
  if (runtime.incompatible) return <div className="pin"><header className="ph"><h1 className="h1" aria-label="API compatibility">API compatibility</h1>
    <p className="lede">Reload this page to use a client that supports API version {runtime.apiVersion}.</p></header></div>;
  return <Outlet />;
}

export function Shell() {
  const runtime = useRuntime();
  const location = useLocation();
  const navigate = useNavigate();
  const [menu, setMenu] = useState(false);
  const [palette, setPalette] = useState(false);
  const [task, setTask] = useState(false);
  const onRun = location.pathname.startsWith("/runs/");

  useEffect(() => rememberClick.listen(), []);
  useEffect(() => { setMenu(false); }, [location.pathname]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") { event.preventDefault(); setPalette(open => !open); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return <div className="app app-frame" data-connection-status={runtime.connection}>
    <TopBar onMenu={() => setMenu(true)} onPalette={() => setPalette(true)} />
    {onRun ? <><StatusMessages /><MainState /></> : <main className="page scroll" tabIndex={-1} data-column="main">
      <StatusMessages />
      {runtime.versionLoading && !runtime.authRequired && <div className="pin"><p className="help" role="status">Checking API version…</p></div>}
      <MainState />
    </main>}
    {menu && <MenuSheet onClose={() => setMenu(false)} onPalette={() => setPalette(true)} />}
    {task && <NewTaskSheet onClose={() => setTask(false)} />}
    {palette && <Palette onClose={() => setPalette(false)} onTask={() => { setPalette(false); setTask(true); }} navigate={navigate} />}
  </div>;
}
