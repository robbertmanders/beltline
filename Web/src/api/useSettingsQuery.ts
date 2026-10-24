import { useQuery } from "@tanstack/react-query";
import { useRuntime } from "../app/runtime";
import { apiRequest } from "./client";
import { resourcesForPath } from "./resources";

import { settingsQueryKey } from "./settingsCommands";
import type { SettingsResponse } from "./wire.gen";

/** The daemon's saved settings, read by the shell (for the Insights switch and the demo chip) and by Settings. */
export function useSettingsQuery() {
  const runtime = useRuntime();
  return useQuery({
    queryKey: settingsQueryKey,
    queryFn: ({ signal }) => apiRequest<SettingsResponse>("/api/v1/settings", { signal, onUnauthorized: runtime.requireSignIn }),
    enabled: runtime.canRead,
    retry: false,
    meta: { resourceDeps: resourcesForPath("/api/v1/settings") }
  });
}
