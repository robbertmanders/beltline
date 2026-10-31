import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Link, MemoryRouter, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { lazy, Suspense, useState, type PropsWithChildren } from "react";
import { RuntimeProvider } from "./runtime";
import { useSettingsQuery } from "../api/useSettingsQuery";
import { Shell } from "./Shell";
import { NotFoundPage } from "../pages/NotFound";
import { OverviewPage } from "../pages/overview/OverviewPage";

import { WorkPage } from "../pages/Work";
import { RunPage } from "../pages/run/RunPage";
import { BeltDraftsProvider } from "../pages/belts/beltDrafts";
import { AgentDraftsProvider } from "../state/agentDrafts";
import { ShareSheet, type ShareSource } from "../pages/library/ShareSheet";

// The pages a visit to the Overview or a run does not need are loaded when first opened, so the first
// script is the Overview's, the run page's and the shell's.
const pageFallback = <div className="page-loading" role="status">Loading…</div>;
const AgentsPage = lazy(() => import("../pages/AgentsPage").then(module => ({ default: module.AgentsPage })));
const AfkPage = lazy(() => import("../pages/Afk").then(module => ({ default: module.AfkPage })));
const SettingsPage = lazy(() => import("../pages/Settings").then(module => ({ default: module.SettingsPage })));
const BeltsPage = lazy(() => import("../pages/belts/BeltsPage").then(module => ({ default: module.BeltsPage })));
const InsightsPage = lazy(() => import("../pages/insights/InsightsPage").then(module => ({ default: module.InsightsPage })));
const LibraryPage = lazy(() => import("../pages/library/LibraryPage").then(module => ({ default: module.LibraryPage })));

function DevelopmentGalleryRoute() {
  const GalleryPage = lazy(() => import("../pages/Gallery"));
  return <Suspense fallback={<div className="gallery-loading" role="status">Loading theme gallery…</div>}><GalleryPage /></Suspense>;
}

export function AppProviders({ children, reloadDocument }: PropsWithChildren<{ reloadDocument?: () => void }>) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: true, staleTime: 0 }, mutations: { retry: false } }
  }));
  return <QueryClientProvider client={queryClient}><RuntimeProvider reloadDocument={reloadDocument}><BeltDraftsProvider><AgentDraftsProvider>{children}</AgentDraftsProvider></BeltDraftsProvider></RuntimeProvider></QueryClientProvider>;
}

export function AppRoutes() {
  const [share, setShare] = useState<ShareSource | null>(null);
  const openShare = (id: string, version: number, kind: "belt" | "agent", name?: string) => setShare({ kind, id, version, name });
  return <>
  <Routes>
    <Route path="/" element={<Navigate to="/overview" replace />} />
    <Route element={<Shell />}>
      <Route path="/overview" element={<OverviewPage />} />
      <Route path="/work" element={<WorkPage />} />
      <Route path="/runs/:id" element={<RunPage />} />
      <Route path="/insights" element={<InsightsRoute />} />
      <Route path="/belts" element={<Suspense fallback={pageFallback}><BeltsPage onShare={(id, version, name) => openShare(id, version, "belt", name)} /></Suspense>} />
      <Route path="/agents" element={<Suspense fallback={pageFallback}><AgentsRoute onShare={(id, version, name) => openShare(id, version, "agent", name)} /></Suspense>} />
      <Route path="/library" element={<Suspense fallback={pageFallback}><LibraryPage /></Suspense>} />
      <Route path="/afk" element={<Suspense fallback={pageFallback}><AfkPage /></Suspense>} />
      <Route path="/settings" element={<Suspense fallback={pageFallback}><SettingsPage /></Suspense>} />
    </Route>
    <Route path="*" element={<NotFoundPage />} />
  </Routes>
  {share && <ShareSheet source={share} onClose={() => setShare(null)} />}
  </>;
}

/** Insights, or a note while Settings turns the page off. */
function InsightsRoute() {
  const settings = useSettingsQuery();
  if (settings.data?.showInsights === false) {
    return <article className="page-content"><header className="page-heading"><div><p className="eyebrow">INSIGHTS</p><h1>Insights</h1></div></header>
      <section className="empty-panel"><h2>Insights is turned off</h2><p>Turn it on under <Link to="/settings">Settings → Experimental</Link>.</p></section></article>;
  }
  return <Suspense fallback={pageFallback}><InsightsPage /></Suspense>;
}

function AgentsRoute({ onShare }: { onShare: (id: string, version: number, name?: string) => void }) {
  return <AgentsPage onShare={onShare} />;
}

function RoutedApp({ children }: PropsWithChildren) {
  const { pathname } = useLocation();
  if ((import.meta.env.DEV || import.meta.env.VITE_BELTLINE_SNAPSHOTS === "1") && pathname === "/_gallery") return <DevelopmentGalleryRoute />;
  return <AppProviders><>{children}<AppRoutes /></></AppProviders>;
}

export function BeltlineApp() {
  return <BrowserRouter><RoutedApp /></BrowserRouter>;
}

export function TestApp({ initialEntries = ["/overview"], children }: PropsWithChildren<{ initialEntries?: string[] }>) {
  return <MemoryRouter initialEntries={initialEntries}><RoutedApp>{children}</RoutedApp></MemoryRouter>;
}
