import { useQuery } from "@tanstack/react-query";
import { apiRequest } from "../api/client";
import { resourcesForPath } from "../api/resources";
import { beltsQueryKey } from "../pages/belts/beltCommands";
import type { BeltsResponse } from "../api/wire.gen";
import { useRuntime } from "../app/runtime";
import { beltList } from "../pages/belts/beltRecords";
import { TaskSheet } from "../pages/belts/TaskSheet";
import { Layer } from "./Layer";

/** New task from the top bar: the belts the daemon says can start a task now, as one sheet. */
export function NewTaskSheet({ onClose }: { onClose: () => void }) {
  const runtime = useRuntime();
  const query = useQuery({
    queryKey: beltsQueryKey,
    queryFn: ({ signal }) => apiRequest<BeltsResponse>("/api/v1/belts", { signal, onUnauthorized: runtime.requireSignIn }).then(beltList),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/belts") }
  });
  const choices = (query.data?.belts ?? [])
    .filter(item => item.actions.some(action => action.id === "task" && action.enabled))
    .map(item => item.belt);
  if (!query.data) return <Layer label="Start a task" inverse onClose={onClose}><p className="empty" role="status">{query.error ? "Could not load the belts." : "Loading the belts…"}</p></Layer>;
  if (choices.length === 0) return <Layer label="Start a task" inverse onClose={onClose}>
    <h2 className="h1">No belt takes tasks.<br /><span className="soft">Add a manual trigger first.</span></h2>
    <p className="lede">A task starts on a belt with a Manual task trigger and a repository. Add one in Belts, then start the task here.</p>
  </Layer>;
  return <TaskSheet belt={choices[0]!} choices={choices} onClose={onClose} />;
}
