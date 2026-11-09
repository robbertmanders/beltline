/**
 * The stand-in daemon for page tests: answers each request from the server's goldens unless a route says otherwise,
 * and records every request. Also the router probes and history buttons a test renders beside the app.
 */
import { render, screen, within } from "@testing-library/react";
import type userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { vi } from "vitest";
import { AppProviders, AppRoutes, TestApp } from "../app/App";
import { AppSocket } from "./setup";
import { json } from "./response";
import { goldenText } from "./golden";

export interface DaemonRequest { method: string; path: string; body: any; signal?: AbortSignal | null }
/** A route's answer: a function gets the request, a string is sent as it is, anything else is serialised. */
export type Handler = (request: DaemonRequest) => Response | Promise<Response>;
export type Routes = Record<string, Handler | string | object>;

/** Golden answers for the requests every page makes, and the 404 for anything not routed. */
const defaults: Record<string, string> = {
  "/api/v1/version": "version.json",
  "/api/v1/state": "state.json",
  "/api/v1/agents": "agents.json"
};

/**
 * Stubs fetch with a daemon. A route is matched by `METHOD path`, then `path`, then the same two without the query,
 * then by the longest key ending in `*` that the path starts with.
 * `after` may change the answer of any routed or default request, such as a browser view the daemon computes.
 */
export function stubDaemon(routes: Routes = {}, after?: (request: DaemonRequest, response: Response) => Promise<Response> | Response) {
  const requests: DaemonRequest[] = [];
  vi.stubGlobal("fetch", vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input);
    const method = init?.method ?? "GET";
    const request: DaemonRequest = { method, path, body: init?.body ? JSON.parse(String(init.body)) : undefined, signal: init?.signal };
    requests.push(request);
    const basePath = path.split("?")[0] ?? path;
    // A key ending in * matches every path it starts with; the longest such key wins.
    const prefix = Object.keys(routes).filter(key => key.endsWith("*") && path.startsWith(key.slice(0, -1))).sort((a, b) => b.length - a.length)[0];
    const route = routes[`${method} ${path}`] ?? routes[path] ?? routes[`${method} ${basePath}`] ?? routes[basePath] ?? (prefix ? routes[prefix] : undefined);
    let response: Response;
    if (route !== undefined) {
      response = typeof route === "function" ? await route(request) : json(route);
    } else if (method === "GET" && defaults[basePath]) {
      response = json(goldenText(defaults[basePath]));
    } else {
      response = json(goldenText("error-not-found.json"), 404);
    }
    return after ? after(request, response) : response;
  }));
  const posts = (suffix = "") => requests.filter(request => request.method === "POST" && request.path.endsWith(suffix));
  return { requests, posts };
}

/** Tells the open page the library changed, as the daemon's event does after a configuration write. */
export function libraryChanged(revision: number) {
  const socket = AppSocket.instances.at(-1)!;
  socket.send({ type: "hello", apiVersion: 1 });
  socket.send({ type: "changed", resource: "library", revision });
}

/** Renders the app at `path`, with probes showing the location and history buttons for tests that navigate. */
export function renderRoutes(path: string) {
  return render(<AppProviders><MemoryRouter initialEntries={[path]}><AppRoutes /><Where /><History /></MemoryRouter></AppProviders>);
}

export function Where() {
  const location = useLocation();
  return <output aria-label="Location">{location.pathname}{location.search}</output>;
}

/** Browser Back and Forward, and a link-free trip to other pages, for the tests. */
export function History() {
  const navigate = useNavigate();
  return <nav aria-label="Test history">
    <button type="button" onClick={() => navigate(-1)}>Back</button>
    <button type="button" onClick={() => navigate(1)}>Forward</button>
    <button type="button" onClick={() => navigate("/settings")}>Go to settings</button>
    <button type="button" onClick={() => navigate("/agents")}>Go to agents</button>
    <button type="button" onClick={() => navigate("/belts")}>Go to belts</button>
  </nav>;
}

/** Clicks one of the History buttons. */
export const history = (user: ReturnType<typeof userEvent.setup>, name: "Back" | "Forward" | "Go to settings" | "Go to agents" | "Go to belts") =>
  user.click(within(screen.getByRole("navigation", { name: "Test history" })).getByRole("button", { name }));

/** A route that answers with a golden file, at `status`. */
export const fixtureRoute = (name: string, status = 200): Handler => () => new Response(goldenText(name), { status, headers: { "content-type": "application/json" } });

/** A route that refuses with the daemon's message and problems. */
export const refusal = (message: string, problems: string[] = [], status = 422): Handler => () => json({ message, problems }, status);

/** Renders the whole app at `path`, the way the page tests open a page. */
export function renderPage(path: string) {
  return render(<TestApp initialEntries={[path]} />);
}
