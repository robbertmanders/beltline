import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import type { NavigateFunction } from "react-router-dom";
import { apiRequest } from "../api/client";
import { apiQueryKey, resourcesForPath } from "../api/resources";
import type { AgentsResponse, BeltsResponse } from "../api/wire.gen";
import { useRuntime } from "../app/runtime";
import { useDestinations } from "../app/Shell";
import { beltList, type BeltList } from "../pages/belts/beltRecords";

interface Item { label: string; hint: string; run: () => void }

function useList<T, R = T>(path: string, select: (response: T) => R = response => response as unknown as R) {
  const runtime = useRuntime();
  return useQuery({
    queryKey: apiQueryKey(path),
    // The same read the pages make, in the shape they keep it in the cache.
    queryFn: ({ signal }) => apiRequest<T>(path, { signal, onUnauthorized: runtime.requireSignIn }).then(select),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath(path) }
  });
}

/**
 * Search and commands (⌘K): the pages, a new task, the belts and agents by name, and a search of Work.
 * Typing filters the list by the words; Enter takes the highlighted one.
 */
export function Palette({ onClose, onTask, navigate }: { onClose: () => void; onTask: () => void; navigate: NavigateFunction }) {
  const [text, setText] = useState("");
  const [selected, setSelected] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const pages = useDestinations();
  const belts = useList<BeltsResponse, BeltList>("/api/v1/belts", beltList).data?.belts ?? [];
  const agents = useList<AgentsResponse>("/api/v1/agents").data?.agents ?? [];
  const go = (to: string) => () => navigate(to);

  const items = useMemo((): Item[] => {
    const query = text.trim().toLowerCase();
    const all: Item[] = [
      ...pages.map(page => ({ label: page.label, hint: "Page", run: go(page.to) })),
      { label: "New task", hint: "Start a run", run: onTask },
      ...belts.filter(item => !item.archived).map(item => ({ label: item.draft.name, hint: "Belt", run: go(`/belts?belt=${encodeURIComponent(item.draft.id)}`) })),
      ...agents.filter(item => !item.archived).map(item => ({ label: item.agent.name, hint: "Agent", run: go(`/agents?agent=${encodeURIComponent(item.agent.id)}`) }))
    ];
    const words = query.split(/\s+/).filter(Boolean);
    const matching = words.length ? all.filter(item => words.every(word => `${item.label} ${item.hint}`.toLowerCase().includes(word))) : all;
    return query ? [...matching, { label: `Search runs for “${text.trim()}”`, hint: "Work", run: go(`/work?search=${encodeURIComponent(text.trim())}`) }] : matching;
  }, [text, pages, belts, agents]);

  useEffect(() => { input.current?.focus(); }, []);
  useEffect(() => { setSelected(0); }, [text]);
  const take = (item: Item | undefined) => { if (!item) return; onClose(); item.run(); };

  return <div className="pal-scrim" onClick={event => { if (event.target === event.currentTarget) onClose(); }}>
    <div className="pal" role="dialog" aria-modal="true" aria-label="Search and commands">
      <input ref={input} value={text} placeholder="Search runs, belts and agents, or type a command" autoComplete="off" role="combobox"
        aria-label="Search runs, belts and agents, or type a command" aria-controls="pal-list" aria-expanded="true"
        onChange={event => setText(event.currentTarget.value)}
        onKeyDown={event => {
          if (event.key === "Escape") { event.preventDefault(); onClose(); }
          if (event.key === "ArrowDown") { event.preventDefault(); setSelected(index => Math.min(items.length - 1, index + 1)); }
          if (event.key === "ArrowUp") { event.preventDefault(); setSelected(index => Math.max(0, index - 1)); }
          if (event.key === "Enter") { event.preventDefault(); take(items[selected]); }
        }} />
      <ul id="pal-list" role="listbox" aria-label="Results">
        {items.length === 0 && <li className="none">Nothing matches.</li>}
        {items.map((item, index) => <li key={`${item.hint}-${item.label}-${index}`}>
          <button type="button" role="option" aria-selected={index === selected} onMouseMove={() => setSelected(index)} onClick={() => take(item)}>
            <span>{item.label}</span><span>{item.hint}</span>
          </button>
        </li>)}
      </ul>
    </div>
  </div>;
}
