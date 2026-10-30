/** Shared by the Agents page tests: a stand-in daemon that answers from the server's goldens, and the library edits a configuration change makes. */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { vi } from "vitest";
import { AppRoutes } from "../app/App";
import { RuntimeProvider } from "../app/runtime";
import { AppSocket } from "../test/setup";
import { stubDaemon } from "../test/daemon";
import { AgentDraftsProvider } from "../state/agentDrafts";
import { json } from "../test/response";
import { golden } from "../test/golden";

export type Record_ = { agent: { id: string; version: number; name: string } & Record<string, unknown>; archived: boolean; glyph: string };
export type Captured = { method: string; path: string; body?: Record<string, unknown> };
export type List = { agents: Record_[]; choices: Array<Record<string, unknown>>; actions: Array<Record<string, unknown>>; browser?: Record<string, unknown> };
export type Detail = { actions: Array<Record<string, unknown>>; presentation: Record<string, unknown>; folder: string };

export const listGolden = () => golden<List>("agents.json");
export const detailGolden = () => golden<Detail>("agent-detail.json");
export const draftGolden = () => golden<Detail & { agent: unknown }>("agent-draft-presentation.json");

export function detailFor(record: Record_): Detail & Record<string, unknown> {
  const detail = detailGolden();
  return { ...detail, agent: record.agent, archived: record.archived };
}

export interface StubOptions {
  /** When it returns true, the library and detail reads answer 503, as a daemon that cannot read the folder does. */
  failing?: () => boolean;
  list?: List;
  listResponse?: (requestNumber: number) => Response | Promise<Response>;
  detail?: (id: string) => unknown;
  missingDetail?: boolean;
  draftActions?: Array<Record<string, unknown>>;
  models?: (runner: string) => unknown;
  modelResponse?: (runner: string) => Response | Promise<Response>;
  assistant?: Record<string, unknown>;
  assistantResponse?: (readCount: number) => Response | Promise<Response>;
  save?: (body: { agent: Record<string, unknown> }) => Response | Promise<Response>;
  switchRunner?: (body: { agent: Record<string, unknown>; runner: string; alternateIndex?: number }) => unknown;
}

export function stubAgents(options: StubOptions = {}) {
  const list = (): List => {
    const value = options.list ?? listGolden();
    if (value.browser) return { ...value, browser: { ...value.browser, actions: value.actions } };
    return { ...value, browser: {
      items: value.agents.map(record => ({ id: record.agent.id, name: record.agent.name, version: record.agent.version,
        summary: String(record.agent.brief ?? ""), detail: `v${record.agent.version} · ${record.archived ? "archived" : "available"}`,
        status: record.archived ? "Archived" : "Ready", statusTone: record.archived ? "grey" : "good",
        icon: record.agent.icon ?? record.glyph, tint: record.agent.tint, stages: [] })),
      filters: [{ id: "all", label: "All agents", count: value.agents.length }, { id: "used", label: "Used", count: 0 }, { id: "unused", label: "Unused", count: value.agents.length }, { id: "archived", label: "Archived", count: value.agents.filter(record => record.archived).length }],
      totalCount: value.agents.length, matchCount: value.agents.length, actions: value.actions
    } } as List;
  };
  const failure = () => json({ message: "The library cannot be read", problems: [] }, 503);
  const notFound = () => json({ message: "Not found", problems: [] }, 404);
  let listRequestCount = 0;
  let assistant = options.assistant ?? golden<Record<string, unknown>>("assistant.json");
  const agentRecord = (id: string) => list().agents.find(entry => entry.agent.id === id);
  const modelRoute = (runner: string) => options.modelResponse ? options.modelResponse(runner)
    : options.models ? json(options.models(runner)) : json(golden("models.json"));
  const daemon = stubDaemon({
    "GET /api/v1/agents": () => {
      listRequestCount += 1;
      if (options.failing?.()) return failure();
      return options.listResponse ? options.listResponse(listRequestCount) : json(list());
    },
    "/api/v1/agents/new": () => json(golden("agent-new.json")),
    "POST /api/v1/agents/save": request => {
      const agent = request.body.agent as Record<string, unknown>;
      if (options.save) return options.save(request.body as { agent: Record<string, unknown> });
      const version = Number(agent.version ?? 0) + 1;
      return json({ agent: { ...agent, version }, archived: false, glyph: agent.icon ?? "chip", detail: `v${version}` });
    },
    "POST /api/v1/agents/draft/presentation": request => {
      const draft = draftGolden();
      return json({ agent: request.body.agent, actions: options.draftActions ?? draft.actions, presentation: { ...draft.presentation, warnings: [], usedBy: detailGolden().presentation.usedBy } });
    },
    "POST /api/v1/agents/draft/switch-runner": async request => {
      const body = request.body as { agent: Record<string, unknown>; runner: string; alternateIndex?: number };
      if (options.switchRunner) return json(await options.switchRunner(body as Parameters<NonNullable<StubOptions["switchRunner"]>>[0]));
      const agent = body.agent;
      const alternateIndex = body.alternateIndex;
      const switched = Number.isInteger(alternateIndex)
        ? { ...agent, alternates: (agent.alternates as Array<Record<string, unknown>>).map((entry, index) => index === alternateIndex ? { ...entry, runner: body.runner, model: "", thinking: null } : entry) }
        : { ...agent, runner: body.runner, model: "" };
      const draft = draftGolden();
      return json({ agent: switched, actions: draft.actions, presentation: { ...draft.presentation, warnings: [] } });
    },
    "POST /api/v1/desktop/reveal": () => json({ ok: true }),
    "/api/v1/agents/*": request => {
      const path = request.path.split("?")[0]!;
      if (options.failing?.() && request.method === "GET") return failure();
      const command = /^\/api\/v1\/agents\/([^/]+)\/(archive|unarchive|reset)$/.exec(path);
      if (command) {
        const record = agentRecord(decodeURIComponent(command[1]!))!;
        if (command[2] === "archive") return json({ ...record, archived: true });
        if (command[2] === "unarchive") return json({ ...record, archived: false });
        return json({ agent: { ...record.agent, version: record.agent.version + 1 }, archived: record.archived });
      }
      const detail = /^\/api\/v1\/agents\/([^/]+)$/.exec(path);
      if (!detail) return notFound();
      if (options.missingDetail) return notFound();
      const id = decodeURIComponent(detail[1]!);
      if (options.detail) return json(options.detail(id));
      // A saved agent the library no longer has is a 404, which is how the daemon says it is missing.
      const record = agentRecord(id);
      return record ? json(detailFor(record)) : notFound();
    },
    "/api/v1/models/*": request => modelRoute(request.path.split("?")[0]!.split("/").at(-1)!),
    "GET /api/v1/assistant": () => assistant === undefined ? notFound() : options.assistantResponse ? options.assistantResponse(0) : json(assistant),
    "POST /api/v1/assistant/configuration": request => {
      assistant = { ...assistant, configuration: request.body };
      return json(assistant);
    },
    "POST /api/v1/assistant/start": () => {
      assistant = { ...assistant, running: true, command: null, actions: [
        { id: "configuration", enabled: false, label: "Change configuration", reason: "Stop the assistant before changing its configuration." },
        { id: "start", enabled: false, label: "Start", reason: "The assistant is already running." },
        { id: "stop", enabled: true, label: "Stop", reason: null }
      ] };
      return json(assistant);
    },
    "POST /api/v1/assistant/stop": () => {
      assistant = golden<Record<string, unknown>>("assistant.json");
      return json(assistant);
    }
  });
  return { requests: daemon.requests };
}

export function renderAgents(queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: true, staleTime: 0 }, mutations: { retry: false } } }), route = "/agents?agent=triage") {
  const result = render(<QueryClientProvider client={queryClient}><AgentDraftsProvider><RuntimeProvider><MemoryRouter initialEntries={[route]}><AppRoutes /></MemoryRouter></RuntimeProvider></AgentDraftsProvider></QueryClientProvider>);
  return { ...result, queryClient };
}

export const posts = (requests: Captured[]) => requests.filter(request => request.method === "POST");

/** Clears what a test leaves behind: local storage, stubbed globals and the socket it opened. */
export function resetAgentsEnvironment() {
  localStorage.clear();
  vi.unstubAllGlobals();
  vi.stubGlobal("WebSocket", AppSocket);
}

/** Renames a listed agent and gives it its next version, as a configuration change does in the data folder. */
export function configure(list: List, id: string, name: string) {
  const record = list.agents.find(entry => entry.agent.id === id)!;
  record.agent = { ...record.agent, name, version: record.agent.version + 1 };
  // The golden list carries its own browser cards; the browser shows those, so they change with the agent.
  const card = (list.browser?.items as Array<{ id: string; name: string; version: number }> | undefined)?.find(item => item.id === id);
  if (card) Object.assign(card, { name, version: record.agent.version });
}

/** The saved agent-save requests, which is what a refresh or navigation must never send. */
export const agentSaves = (requests: Captured[]) => requests.filter(request => request.method === "POST" && request.path === "/api/v1/agents/save");
