import { test, expect, type Browser, type BrowserContext, type Locator, type Page, type Request } from "@playwright/test";
import { snapshotCases, selectCases, parseSnapshotSize, type PreCaptureStep, type SnapshotCase, type StepTarget } from "./cases";
import { describeTarget } from "./caseTypes";
import { resolveRunSelector, type DemoRun } from "./runSelector";
import { assertNoColumnOverflows, findColumnOverflows } from "./overflow";
import { applyFixture } from "./captureFixtures";
import { assertStageLine, expectStageCardsInView, stageFormCases, stageLineCases } from "./stageLineChecks";

const origin = process.env.BELTLINE_SNAPSHOT_ORIGIN;
const token = process.env.BELTLINE_SNAPSHOT_TOKEN;
const output = process.env.BELTLINE_SNAPSHOTS;
if (!origin || !token || !output) throw new Error("Snapshot capture requires BELTLINE_SNAPSHOT_ORIGIN, BELTLINE_SNAPSHOT_TOKEN and BELTLINE_SNAPSHOTS");
const cases = selectCases(process.env.BELTLINE_SNAPSHOT_ONLY);
const sizes = parseSnapshotSize(process.env.BELTLINE_SNAPSHOT_SIZE);

async function signIn(context: BrowserContext): Promise<void> {
  try {
    await context.request.get(`${origin}/auth?token=${encodeURIComponent(token!)}`);
  } catch (error) {
    throw new Error(`Could not sign the capture browser in through /auth: ${String(error)}`);
  }
}

/** Every seeded demo run with the fields a RunSelector can match. */
async function demoRuns(context: BrowserContext): Promise<DemoRun[]> {
  // /api/v1/runs computes each run's actions, but a selector needs each run's belt, state and issue,
  // and that read cost is once per capture run rather than once per case.
  const response = await context.request.get(`${origin}/api/v1/runs`);
  if (!response.ok()) throw new Error(`Could not list demo runs (HTTP ${response.status()})`);
  const body: unknown = await response.json();
  const list = (body as { runs?: unknown }).runs;
  if (!Array.isArray(list)) throw new Error("The demo runs response had no runs array");
  return list.map(entry => (((entry as Record<string, unknown>).run ?? entry) as DemoRun));
}

async function ready(page: Page, runId: string | undefined, readiness: "shell" | "gallery" | "run"): Promise<void> {
  if (readiness === "gallery") {
    await expect(page.getByRole("heading", { name: "Theme gallery" })).toBeVisible();
    for (const heading of ["Tokens", "Fonts", "Fields"]) await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Native", exact: true })).toHaveAttribute("aria-pressed", "true");
    expect(await page.evaluate(() => scrollY)).toBe(0);
    await page.evaluate(() => document.fonts.ready);
    await expect.poll(() => page.evaluate(() => document.readyState)).toBe("complete");
    return;
  }
  try {
    await expect(page.getByRole("navigation", { name: "Primary navigation" })).toBeVisible();
  } catch {
    throw new Error(`Expected the authenticated shell at ${page.url()}, received: ${(await page.locator("body").innerText()).slice(0, 500)}`);
  }
  // Library browsers show their title; detail editors retain their screen-reader heading.
  await expect(page.locator("main h1").first()).toBeAttached();
  if (new URL(page.url()).pathname === "/agents") {
    // The demo Agents response builds usage presentations for every row and can
    // take longer than the generic page readiness window.
    await expect(page.locator(".agents-page .loaded-state, .agents-page .error-panel, .agents-page .agent-editor[data-ready='true']").first()).toBeVisible({ timeout: 30_000 });
  }
  await expect(page.getByRole("group", { name: "Global controls" })).toBeVisible();
  await expect.poll(() => page.locator(".global-controls button:not(:disabled)").count(), { timeout: 30_000 }).toBeGreaterThan(0);
  await expect(page.getByRole("heading", { name: /Sign in required|Not found|Unable to load this page/ })).toHaveCount(0);
  await page.evaluate(() => document.fonts.ready);
  await expect(page.locator(".app-frame")).toHaveAttribute("data-connection-status", "connected");
  if (readiness === "run") {
    await expect(page.locator(".run-line .box-map").first()).toBeVisible();
    await expect(page.locator(".stage-panel").first()).toBeVisible();
    await expect(page.getByRole("status", { name: /Loading run|Loading output|Loading artifacts/ })).toHaveCount(0);
    return;
  }
  if (new URL(page.url()).pathname === "/library") {
    // The Library page draws one card per repository, with no loaded or empty marker of its own.
    await expect(page.locator(".library-page .library-repo").first()).toBeAttached();
    await expect(page.getByRole("status", { name: /Loading library/ })).toHaveCount(0);
    return;
  }
  await expect(page.locator("main .loaded-state, main .empty-state, main .agent-editor[data-ready='true']").first()).toBeAttached();
  if (runId) await expect(page.getByRole("heading", { name: `Run ${runId}` })).toBeVisible();
  await expect(page.getByRole("status", { name: /Loading resource|Waiting for the daemon|Checking API version/ })).toHaveCount(0);
}

async function waitForRequiredResources(page: Page, pending: Set<Request>): Promise<void> {
  await page.waitForLoadState("load");
  await expect.poll(() => [...pending].map(request => request.url()), { message: "required page requests should finish before capture", timeout: 30_000 }).toEqual([]);
  await page.evaluate(() => document.fonts.ready);
  await expect.poll(() => page.evaluate(() => document.readyState)).toBe("complete");
  await expect.poll(() => page.evaluate(() => performance.getEntriesByType("resource")
    .filter(entry => entry instanceof PerformanceResourceTiming && entry.responseEnd === 0)
    .map(entry => entry.name)), { message: "all required browser resources should settle before capture" }).toEqual([]);
}


function locate(page: Page, target: StepTarget): Locator {
  if ("selector" in target) return page.locator(target.selector);
  return page.getByRole(target.role, { name: target.name, exact: target.exact ?? true });
}


// Each step must find its target exactly once; a missing or ambiguous target fails the capture by name.
async function runStep(page: Page, caseId: string, step: PreCaptureStep): Promise<void> {
  const locator = locate(page, step.target);
  const label = `${caseId}: ${step.kind} ${describeTarget(step.target)}`;
  if (step.kind === "set-button") {
    const opposite = page.getByRole(step.opposite.role, { name: step.opposite.name, exact: step.opposite.exact ?? true });
    const desiredCount = await locator.count();
    if (desiredCount === 0) {
      await expect(opposite, `${label} needs the opposite action to be available`).toHaveCount(1);
      await opposite.click();
    } else {
      await expect(locator, `${label} needs exactly one desired action`).toHaveCount(1);
    }
    await expect(locator, `${label} should reach the desired action label`).toBeVisible();
    return;
  }
  if (step.kind === "wait") {
    if (step.state === "visible") await expect(locator, `${label} should become visible`).toBeVisible();
    else await expect(locator, `${label} should be hidden`).toBeHidden();
    return;
  }
  await expect(locator, `${label} needs exactly one target`).toHaveCount(1);
  if (step.kind === "click") await locator.click();
  else if (step.kind === "fill") await locator.fill(step.value);
  else if (step.kind === "select") await locator.selectOption(step.value);
  else await locator.press(step.key);
}

async function readPreferences(page: Page): Promise<Record<string, unknown>> {
  return page.evaluate(() => JSON.parse(localStorage.getItem("beltline.web.preferences") || "{}") as Record<string, unknown>);
}


/** `captureImage: false` runs the same browser flow and assertions without writing a screenshot. */
async function capture(browser: Browser, item: SnapshotCase, size: { width: number; height: number }, runs: DemoRun[], options: { captureImage: boolean; inspect?: (page: Page) => Promise<void> } = { captureImage: true }) {
  const context = await browser.newContext({ viewport: size, deviceScaleFactor: 1, reducedMotion: item.reducedMotion });
  try {
    const isRun = typeof item.route === "function";
    const runId = isRun ? resolveRunSelector(item.id, item.run, runs) : undefined;
    const route = typeof item.route === "function" ? item.route(runId!) : item.route;
    if (item.sidebar || item.appearance) {
      // The appearance is stored the way a visitor's browser holds it; an unset one is Light, as in the app.
      await context.addInitScript(({ key, sidebar, appearance }) => {
        localStorage.setItem(key, JSON.stringify({ version: 1, floorPinnedBeltId: null, runsFolded: {}, watchingFolded: {}, navigationFolded: {}, listFolded: {}, navigationOpen: true, sessionsOpen: false, assistantOpen: false, navigationWidth: 220, appearance, ...sidebar, sessionsWidth: null, assistantWidth: null }));
      }, { key: "beltline.web.preferences", sidebar: item.sidebar ?? null, appearance: item.appearance ?? "light" });
    }
    for (const response of item.apiResponses ?? []) {
      // Matched by path, and by query string when the case names one, so a width-dependent view can be replaced.
      await context.route(url => url.origin === origin && url.pathname === response.path && (response.query === undefined || url.search === response.query),
        request => request.fulfill({ status: response.status, contentType: "application/json", body: response.body }));
    }
    if (isRun) {
      await context.addInitScript(path => {
        window.history.replaceState(window.history.state, "", path);
      }, route);
    }
    if (item.readiness === "shell") {
      await signIn(context);
      const state = await context.request.get(`${origin}/api/v1/state`);
      if (!state.ok()) throw new Error(`Authenticated daemon state request failed (HTTP ${state.status()})`);
      await state.json();
    }
    if (item.readiness === "run") {
      await signIn(context);
    }
    for (const fixture of item.fixtures ?? []) await applyFixture(context, origin!, item, fixture);
    const page = await context.newPage();
    const pending = new Set<Request>();
    page.on("request", request => {
      // The event WebSocket intentionally remains open for the lifetime of the page.
      if (!request.url().includes("/api/v1/events")) pending.add(request);
    });
    // A successful API body is consumed before its loaded-state marker renders.
    // Track until response headers arrive; the content assertions below verify parsing/rendering.
    page.on("response", response => pending.delete(response.request()));
    page.on("requestfailed", request => {
      pending.delete(request);
    });
    // The daemon's static-file resolver treats an encoded slash as a path separator.
    // Enter the opaque run route before the app starts, without requesting it from the static server.
    await page.goto(`${origin}${isRun ? "/" : route}`, { waitUntil: "domcontentloaded" });
    const landed = new URL(page.url());
    expect(`${landed.pathname}${landed.search}`).toBe(route);
    await ready(page, isRun ? runId : undefined, item.readiness);
    await waitForRequiredResources(page, pending);
    const preferences = await readPreferences(page);
    if (item.readiness === "shell" || item.readiness === "run") {
      expect(preferences.navigationOpen).toBe(item.sidebar!.navigationOpen);
      expect(preferences.navigationWidth).toBe(item.sidebar!.navigationWidth);
      expect(preferences.sessionsOpen).toBe(item.sidebar!.sessionsOpen);
      expect(preferences.assistantOpen).toBe(item.sidebar!.assistantOpen);
      const selected = await page.locator('[aria-current="page"]').first().getAttribute("href");
      const expected = route.startsWith("/runs/") ? "/work" : route.split("?")[0];
      expect(selected).toBe(expected);
      const nav = await page.locator("[data-column=navigation]").evaluate(element => element.getBoundingClientRect().width);
      expect(nav).toBe(item.sidebar!.navigationOpen ? item.sidebar!.navigationWidth : 40);
    }
    if (item.steps?.length) {
      for (const step of item.steps) await runStep(page, item.id, step);
      await waitForRequiredResources(page, pending);
    }
    if (item.readyMarker) {
      await expect(page.locator(item.readyMarker), `${item.id} readiness marker ${item.readyMarker} should exist once`).toHaveCount(1);
      await expect(page.locator(item.readyMarker), `${item.id} readiness marker ${item.readyMarker} should be visible`).toBeVisible();
    }
    if (item.expectedText) await expect(page.getByText(item.expectedText, { exact: false })).toBeVisible();
    if (item.expectedPreferences) {
      const stored = await readPreferences(page);
      for (const [key, value] of Object.entries(item.expectedPreferences)) {
        expect(stored[key], `${item.id} should store ${key}=${String(value)} after its steps`).toBe(value);
      }
    }
    for (const selector of item.visibleColumns ?? []) {
      const column = page.locator(selector);
      await expect(column, `${item.id} column ${selector} should be visible`).toBeVisible();
      const box = await column.boundingBox();
      expect(box?.width ?? 0, `${item.id} column ${selector} should have a nonzero width`).toBeGreaterThan(0);
    }
    if (stageLineCases.includes(item.id)) await assertStageLine(page, item.id, size);
    if (stageFormCases.includes(item.id)) await expectStageCardsInView(page, item.id);
    if (item.formAtTop) await page.locator(".stage-form").first().evaluate(form => form.scrollIntoView({ block: "start" }));
    if (item.scrollToTop) await page.locator(item.scrollToTop).first().evaluate(section => section.scrollIntoView({ block: "start" }));
    const findings = await page.evaluate(source => new Function(`return (${source})()`)(), findColumnOverflows.toString()) as ReturnType<typeof findColumnOverflows>;
    if (process.env.BELTLINE_SNAPSHOT_TEST_OVERFLOW === "1" && item.id === "overview-open") {
      await page.evaluate(() => {
        const column = document.querySelector<HTMLElement>('[data-column="main"]');
        if (!column) throw new Error("overflow fixture could not find the main column");
        const fixture = document.createElement("div");
        fixture.id = "overflow-fixture";
        fixture.style.cssText = "position:absolute;left:0;top:0;width:1500px;height:20px";
        column.append(fixture);
      });
      const fixtureFindings = await page.evaluate(source => new Function(`return (${source})()`)(), findColumnOverflows.toString()) as ReturnType<typeof findColumnOverflows>;
      assertNoColumnOverflows(item.id, size, fixtureFindings);
    }
    assertNoColumnOverflows(item.id, size, findings);
    if (options.inspect) await options.inspect(page);
    if (options.captureImage) {
      await page.screenshot({ path: `${output}/${item.id}-${size.width}x${size.height}.png`, animations: "disabled", fullPage: false, scale: "css" });
      console.log(`Captured ${item.id} ${size.width}x${size.height}: ${output}/${item.id}-${size.width}x${size.height}.png`);
    }
  } finally {
    await context.close();
  }
}

let seededRuns: DemoRun[] = [];
test.beforeAll(async ({ browser }) => {
  if (cases.some(item => typeof item.route === "function")) {
    const context = await browser.newContext();
    try {
      await signIn(context);
      seededRuns = await demoRuns(context);
    } finally {
      await context.close();
    }
  }
});

test("overflow detector catches clipped, text, nested and horizontal-scroll escapes", async ({ page }) => {
  await page.setContent(`
    <style>
      * { box-sizing: border-box; }
      [data-column] { position: relative; margin: 0; padding: 0; }
      #clipped { width: 100px; height: 30px; overflow: hidden; }
      #clipped-wide { display: block; width: 130px; height: 20px; }
      #text-column { width: 100px; height: 30px; }
      #long-text { white-space: nowrap; }
      #outer { width: 200px; height: 50px; }
      #nested { position: absolute; left: 50px; top: 0; width: 100px; height: 40px; }
      #nested-escape { position: absolute; left: 0; top: 0; width: 120px; height: 20px; }
      #horizontal { width: 100px; height: 30px; overflow-x: auto; }
      #horizontal-wide { display: block; width: 130px; height: 20px; }
      #wrapped { width: 100px; }
      #vertical { width: 100px; height: 40px; overflow-y: auto; }
      #vertical-child { width: 90px; height: 300px; }
      #subpixel { width: 100px; height: 20px; }
      #subpixel-child { position: absolute; left: 0; top: 0; width: 100.5px; height: 10px; }
    </style>
    <main data-column="clipped" id="clipped"><span id="clipped-wide">clipped fixture</span></main>
    <main data-column="text" id="text-column"><span id="long-text">Averylongunbrokenwordthatmustextendpastthecolumnrightedge</span></main>
    <main data-column="outer" id="outer"><section data-column="nested" id="nested"><span id="nested-escape">nested escape</span></section></main>
    <main data-column="horizontal" id="horizontal"><span id="horizontal-wide">horizontal escape</span></main>
    <main data-column="wrapped" id="wrapped"><span>This long phrase wraps naturally inside its column with ordinary vertical content.</span></main>
    <main data-column="vertical" id="vertical"><div id="vertical-child"></div></main>
    <main data-column="subpixel" id="subpixel"><span id="subpixel-child"></span></main>
  `);
  const findings = await page.evaluate(source => new Function(`return (${source})()`)(), findColumnOverflows.toString()) as ReturnType<typeof findColumnOverflows>;
  expect(findings.some(item => item.column === "clipped" && item.element === "span#clipped-wide")).toBe(true);
  expect(findings.some(item => item.column === "text" && item.element === "span#long-text")).toBe(true);
  expect(findings.some(item => item.column === "nested" && item.element === "span#nested-escape")).toBe(true);
  expect(findings.some(item => item.column === "horizontal" && item.element === "span#horizontal-wide")).toBe(true);
  expect(findings.some(item => item.column === "outer" && item.element === "section#nested")).toBe(false);
  expect(findings.some(item => ["wrapped", "vertical", "subpixel"].includes(item.column))).toBe(false);
  expect(() => assertNoColumnOverflows("overflow-fixture", { width: 1080, height: 680 }, findings))
    .toThrow("overflow-fixture at 1080x680: column clipped, span#clipped-wide exceeds right edge by 30.00px");
});

for (const item of cases) {
  for (const size of sizes) {
    test(`${item.id} ${size.width}x${size.height}`, async ({ browser }) => capture(browser, item, size, seededRuns));
  }
}

/** Run content with Sessions open and closed, at the epic's 1080×680 minimum and the 1440 and 2560 sizes. Assertions only: no screenshot is written. */
const layoutSizes = [{ width: 1080, height: 680 }, { width: 1440, height: 900 }, { width: 2560, height: 1440 }];
const layoutCaseIds = ["run-wrapping-closed", "run-wrapping-open", "run-sessions-click", "run-wide"];
for (const id of layoutCaseIds) {
  const item = snapshotCases.find(candidate => candidate.id === id);
  if (!item) throw new Error(`layout case ${id} is not in the snapshot matrix`);
  for (const size of layoutSizes) {
    test(`${id} ${size.width}x${size.height} layout without capture`, async ({ browser }) => {
      if (seededRuns.length === 0) {
        const context = await browser.newContext();
        try {
          await signIn(context);
          seededRuns = await demoRuns(context);
        } finally {
          await context.close();
        }
      }
      await capture(browser, item, size, seededRuns, { captureImage: false });
    });
  }
}

/** The Assistant layout cases require the library, editor, and assistant columns at each supported viewport. The stage case's readiness marker requires its open form too. The shared capture flow runs findColumnOverflows/assertNoColumnOverflows against the editor and assistant data-column markers and writes no PNG here. */
const assistantLayoutCaseIds = ["belts-assistant-stopped", "belts-cursor-assistant", "belts-assistant-running-demo", "belts-assistant-stage-open"];
for (const id of assistantLayoutCaseIds) {
  const item = snapshotCases.find(candidate => candidate.id === id);
  if (!item) throw new Error(`assistant layout case ${id} is not in the snapshot matrix`);
  for (const size of layoutSizes) {
    test(`${id} ${size.width}x${size.height} layout without capture`, async ({ browser }) => {
      if (seededRuns.length === 0) {
        const context = await browser.newContext();
        try {
          await signIn(context);
          seededRuns = await demoRuns(context);
        } finally {
          await context.close();
        }
      }
      await capture(browser, item, size, seededRuns, { captureImage: false });
    });
  }
}

/** A synthetic Sessions read with `count` identical finished rows, the first selected. */
function sessionRows(count: number) {
  const rows = Array.from({ length: count }, (_, index) => ({
    attemptId: `layout-attempt-${index + 1}`,
    stageId: "build",
    title: `0${(index % 9) + 1} Build · attempt ${index + 1}`,
    caption: "2m 0s · cost not tracked",
    status: "done",
    session: `bl-layout-${index + 1}`,
    selected: index === 0
  }));
  return {
    runId: "layout",
    rows,
    selection: { stageId: "build", attemptId: "layout-attempt-1" },
    ghostty: { enabled: false, reason: "No terminal is running for this session." },
    log: "saved"
  };
}

test("the Sessions list hugs one row, shows six rows without scrolling and scrolls past six", async ({ browser }) => {
  if (seededRuns.length === 0) {
    const context = await browser.newContext();
    try {
      await signIn(context);
      seededRuns = await demoRuns(context);
    } finally {
      await context.close();
    }
  }
  const run = resolveRunSelector("sessions-list-layout", { beltId: "epics", key: "PAY-40" }, seededRuns);
  const measured: Record<number, { height: number; scrolls: boolean }> = {};
  for (const count of [1, 6, 8]) {
    const context = await browser.newContext({ viewport: { width: 1080, height: 680 }, deviceScaleFactor: 1 });
    try {
      await signIn(context);
      // The sidebar is open from the first paint, and the run route is entered before the app starts, as capture does.
      await context.addInitScript(path => {
        window.history.replaceState(window.history.state, "", path);
        window.localStorage.setItem("beltline.web.preferences", JSON.stringify({
          version: 1, floorPinnedBeltId: null, runsFolded: {}, watchingFolded: {}, navigationFolded: {}, listFolded: {},
          navigationOpen: true, sessionsOpen: true, assistantOpen: false, navigationWidth: 220, sessionsWidth: null, assistantWidth: null
        }));
      }, `/runs/${encodeURIComponent(run)}?stage=build`);
      const page = await context.newPage();
      await page.route("**/api/v1/views/run/*/sessions*", route => route.fulfill({ contentType: "application/json", body: JSON.stringify(sessionRows(count)) }));
      await page.goto(`${origin}/`, { waitUntil: "domcontentloaded" });
      const list = page.locator('[data-column="sessions"] .session-list');
      await expect(list, `${count} row(s) should render the list`).toHaveCount(1);
      measured[count] = await list.evaluate(element => ({
        height: element.getBoundingClientRect().height,
        scrolls: element.scrollHeight > element.clientHeight + 1
      }));
    } finally {
      await context.close();
    }
  }
  // Six one-line rows (30 px each, 2 px gaps) make the cap, so six fit without scrolling and a seventh scrolls.
  const cap = 6 * 30 + 5 * 2;
  expect(measured[1]!.height, "one row is one line tall").toBeLessThan(cap);
  expect(measured[1]!.scrolls, "one row does not scroll").toBe(false);
  expect(measured[6]!.height, "six rows reach the six-row cap").toBeCloseTo(cap, 0);
  expect(measured[6]!.scrolls, "six rows fit without scrolling").toBe(false);
  expect(measured[8]!.height, "eight rows stay at the six-row cap").toBeCloseTo(cap, 0);
  expect(measured[8]!.scrolls, "eight rows scroll inside the cap").toBe(true);
});

/** A box in CSS pixels, read from the browser's layout. */
interface Box { top: number; bottom: number; height: number }
/** The Sessions sidebar and its selected screen, and the shell's scroll viewport (`main.content`) they sit in. */
interface SidebarLayout { viewport: Box & { scrollTop: number }; sidebar: Box; screen: Box | null }

async function sidebarLayout(page: Page): Promise<SidebarLayout> {
  return page.evaluate(() => {
    const box = (element: Element): Box => {
      const rect = element.getBoundingClientRect();
      return { top: rect.top, bottom: rect.bottom, height: rect.height };
    };
    const viewport = document.querySelector<HTMLElement>("main.content");
    const sidebar = document.querySelector<HTMLElement>('[data-column="sessions"]');
    if (!viewport || !sidebar) throw new Error("The run page has no scroll viewport or Sessions sidebar");
    const screen = sidebar.querySelector<HTMLElement>(".session-screen");
    return { viewport: { ...box(viewport), height: viewport.clientHeight, scrollTop: viewport.scrollTop }, sidebar: box(sidebar), screen: screen ? box(screen) : null };
  });
}

/** Waits until the sidebar has refitted to the viewport's height, which it reads after the resize observer fires. */
async function waitForSidebarFit(page: Page): Promise<void> {
  await expect.poll(async () => {
    const layout = await sidebarLayout(page);
    return Math.abs(layout.sidebar.height - layout.viewport.height) < 1;
  }, { message: "the Sessions sidebar should refit to the visible content height" }).toBe(true);
}

/** The sidebar and its screen sit inside the visible area, and the sidebar fills it rather than the long page. */
function expectFitsViewport(layout: SidebarLayout, label: string): void {
  const { viewport, sidebar, screen } = layout;
  expect(sidebar.top, `${label}: sidebar top should be inside the visible content`).toBeGreaterThanOrEqual(viewport.top - 1);
  expect(sidebar.bottom, `${label}: sidebar bottom should be inside the visible content`).toBeLessThanOrEqual(viewport.bottom + 1);
  expect(Math.abs(sidebar.height - viewport.height), `${label}: sidebar height should be the visible content height`).toBeLessThan(1);
  expect(screen, `${label}: a selected session should have a screen`).not.toBeNull();
  expect(screen!.top, `${label}: screen top should be inside the sidebar`).toBeGreaterThanOrEqual(sidebar.top - 1);
  expect(screen!.bottom, `${label}: screen bottom should be inside the visible content`).toBeLessThanOrEqual(viewport.bottom + 1);
  expect(screen!.height, `${label}: screen should have visible space`).toBeGreaterThan(0);
}

/** Scrolls an owning region until the target's top is at its top, then checks the target is inside the region's visible area. */
async function revealByScrolling(page: Page, owner: string, target: string, label: string): Promise<void> {
  const selectors = { ownerSelector: owner, targetSelector: target };
  await page.evaluate(({ ownerSelector, targetSelector }) => {
    const region = document.querySelector<HTMLElement>(ownerSelector);
    const element = document.querySelector<HTMLElement>(targetSelector);
    if (!region || !element) throw new Error(`Missing ${ownerSelector} or ${targetSelector}`);
    region.scrollTop += element.getBoundingClientRect().top - region.getBoundingClientRect().top;
  }, selectors);
  const visible = await page.evaluate(({ ownerSelector, targetSelector }) => {
    const region = document.querySelector(ownerSelector)?.getBoundingClientRect();
    const element = document.querySelector(targetSelector)?.getBoundingClientRect();
    if (!region || !element) return false;
    return element.top >= region.top - 1 && element.bottom <= region.bottom + 1 && element.height > 0;
  }, selectors);
  expect(visible, `${label} should be reachable by scrolling ${owner}`).toBe(true);
}

// The long Markdown output keeps the page taller than the viewport, so these checks scroll a real page.
const sessionsFitCase = snapshotCases.find(candidate => candidate.id === "run-wrapping-open");
if (!sessionsFitCase) throw new Error("layout case run-wrapping-open is not in the snapshot matrix");

test("run-wrapping-open fits the Sessions sidebar to the visible content at the top, and refits on resize", async ({ browser }) => {
  if (seededRuns.length === 0) seededRuns = await loadSeededRuns(browser);
  await capture(browser, sessionsFitCase, { width: 1080, height: 680 }, seededRuns, {
    captureImage: false,
    inspect: async page => {
      await expect.poll(() => page.evaluate(() => document.querySelector("main.content")!.scrollTop)).toBe(0);
      await waitForSidebarFit(page);
      expectFitsViewport(await sidebarLayout(page), "1080x680");
      await page.setViewportSize({ width: 1440, height: 900 });
      await waitForSidebarFit(page);
      expectFitsViewport(await sidebarLayout(page), "1440x900 after resize");
      await page.setViewportSize({ width: 1080, height: 680 });
      await waitForSidebarFit(page);
      expectFitsViewport(await sidebarLayout(page), "1080x680 after resize");
    }
  });
});

test("run-wrapping-open moves the Sessions sidebar with the page when scrolled, and restores it at the top", async ({ browser }) => {
  if (seededRuns.length === 0) seededRuns = await loadSeededRuns(browser);
  await capture(browser, sessionsFitCase, { width: 1080, height: 680 }, seededRuns, {
    captureImage: false,
    inspect: async page => {
      await waitForSidebarFit(page);
      const top = await sidebarLayout(page);
      expect(top.viewport.scrollTop, "the page starts at the top").toBe(0);
      await page.evaluate(() => { document.querySelector("main.content")!.scrollTop = 120; });
      const scrolled = await sidebarLayout(page);
      const moved = scrolled.viewport.scrollTop;
      expect(moved, "the long output scrolls the page").toBeGreaterThan(0);
      expect(Math.abs(scrolled.sidebar.top - (top.sidebar.top - moved)), "the sidebar moves up by the scroll distance").toBeLessThan(1);
      expect(Math.abs(scrolled.sidebar.height - top.sidebar.height), "the sidebar keeps its height").toBeLessThan(1);
      expect(Math.abs(scrolled.screen!.top - (top.screen!.top - moved)), "the screen moves up by the scroll distance").toBeLessThan(1);
      expect(Math.abs(scrolled.screen!.height - top.screen!.height), "the screen keeps its height").toBeLessThan(1);
      await page.evaluate(() => { document.querySelector("main.content")!.scrollTop = 0; });
      const restored = await sidebarLayout(page);
      expect(Math.abs(restored.sidebar.top - top.sidebar.top), "returning to the top restores the sidebar position").toBeLessThan(1);
      expect(Math.abs(restored.sidebar.height - top.sidebar.height), "returning to the top restores the sidebar height").toBeLessThan(1);
    }
  });
});

test("run-wrapping-open respects the status-message area above the content when it refits", async ({ browser }) => {
  if (seededRuns.length === 0) seededRuns = await loadSeededRuns(browser);
  await capture(browser, sessionsFitCase, { width: 1080, height: 680 }, seededRuns, {
    captureImage: false,
    inspect: async page => {
      await waitForSidebarFit(page);
      const before = await sidebarLayout(page);
      // The shell's own offline message, inserted above the content as the app shows it.
      await page.evaluate(() => {
        const content = document.querySelector("main.content")!;
        const notice = document.createElement("div");
        notice.id = "layout-status-fixture";
        notice.className = "status-message offline-message";
        notice.textContent = "Offline — reconnecting";
        content.parentElement!.insertBefore(notice, content);
      });
      await expect.poll(async () => (await sidebarLayout(page)).viewport.height).toBeLessThan(before.viewport.height);
      await waitForSidebarFit(page);
      const reduced = await sidebarLayout(page);
      expectFitsViewport(reduced, "with the status message");
      await page.evaluate(() => document.getElementById("layout-status-fixture")?.remove());
      await waitForSidebarFit(page);
      expect(Math.abs((await sidebarLayout(page)).viewport.height - before.viewport.height), "removing the message restores the content height").toBeLessThan(1);
    }
  });
});

/** The demo's seeded runs, read through a signed-in context, as the capture cases need them. */
async function loadSeededRuns(browser: Browser): Promise<DemoRun[]> {
  const context = await browser.newContext();
  try {
    await signIn(context);
    return await demoRuns(context);
  } finally {
    await context.close();
  }
}

/** A synthetic Sessions read with eight rows, long usage and fallback text, and either a saved log of 400 lines or a terminal target. */
function overflowingSessions(variant: "log" | "terminal") {
  const usageLine = "Tokens: input 1,204,552 · output 88,910 · cache read 4,002,118 · ".repeat(3);
  const fallback = "This session has no live terminal, so the saved log is shown instead, and the agent's last state is kept with it. ".repeat(4);
  return {
    ...sessionRows(8),
    usage: { tokens: usageLine, estimate: "Estimated from the rate table; the figure is a guide, not a bill. ".repeat(4) },
    selection: { stageId: "build", attemptId: "layout-attempt-1", fallback },
    ...(variant === "terminal"
      ? { terminal: { target: "bl-layout-1", readOnly: true, continued: false } }
      : { log: Array.from({ length: 400 }, (_, index) => `line ${index + 1}`).join("\n") })
  };
}

for (const variant of ["log", "terminal"] as const) {
  test(`overflowing Sessions metadata and a ${variant} screen stay bounded at 1080x680`, async ({ browser }) => {
    if (seededRuns.length === 0) seededRuns = await loadSeededRuns(browser);
    const run = resolveRunSelector("sessions-overflow-layout", { beltId: "epics", key: "PAY-40" }, seededRuns);
    const context = await browser.newContext({ viewport: { width: 1080, height: 680 }, deviceScaleFactor: 1 });
    try {
      await signIn(context);
      await context.addInitScript(path => {
        window.history.replaceState(window.history.state, "", path);
        window.localStorage.setItem("beltline.web.preferences", JSON.stringify({
          version: 1, floorPinnedBeltId: null, runsFolded: {}, watchingFolded: {}, navigationFolded: {}, listFolded: {},
          navigationOpen: true, sessionsOpen: true, assistantOpen: false, navigationWidth: 220, sessionsWidth: null, assistantWidth: null
        }));
      }, `/runs/${encodeURIComponent(run)}?stage=build`);
      const page = await context.newPage();
      await page.route("**/api/v1/views/run/*/sessions*", route => route.fulfill({ contentType: "application/json", body: JSON.stringify(overflowingSessions(variant)) }));
      await page.goto(`${origin}/`, { waitUntil: "domcontentloaded" });
      await expect(page.locator('[data-column="sessions"][data-ready="true"]')).toBeVisible();
      await waitForSidebarFit(page);
      const layout = await sidebarLayout(page);
      expectFitsViewport(layout, variant);
      // The action bar keeps visible space, and the screen's own region is the terminal or the saved log.
      await expect(page.locator('[data-column="sessions"] .session-bar')).toBeVisible();
      const bar = await page.locator('[data-column="sessions"] .session-bar').boundingBox();
      expect(bar?.height ?? 0, "the action bar should have visible height").toBeGreaterThan(0);
      expect(layout.screen!.height, "the screen should keep at least 150 px").toBeGreaterThanOrEqual(150);
      // Metadata is reachable by scrolling its own region, and the sidebar does not grow to hold it.
      await revealByScrolling(page, ".sessions-meta", ".session-usage", "the usage text");
      await revealByScrolling(page, ".sessions-meta", ".session-fallback", "the fallback message");
      await revealByScrolling(page, ".session-list", '[data-column="sessions"] .session-list li:last-child', "the final session row");
      expectFitsViewport(await sidebarLayout(page), `${variant} after scrolling the metadata`);
      if (variant === "log") {
        const log = await page.locator('[data-column="sessions"] .session-log').evaluate(element => ({
          scrolls: element.scrollHeight > element.clientHeight + 1,
          box: element.getBoundingClientRect().height
        }));
        expect(log.scrolls, "a long saved log scrolls inside its screen").toBe(true);
        await page.locator('[data-column="sessions"] .session-log').evaluate(element => { element.scrollTop = element.scrollHeight; });
        const lastLine = await page.locator('[data-column="sessions"] .session-log').evaluate(element => element.scrollTop + element.clientHeight >= element.scrollHeight - 1);
        expect(lastLine, "the final log line is reached by scrolling the log").toBe(true);
      } else {
        const terminal = await page.locator('[data-column="sessions"] .terminal-screen').boundingBox();
        expect(terminal?.height ?? 0, "the terminal keeps visible space").toBeGreaterThanOrEqual(100);
        expect((terminal?.y ?? 0) + (terminal?.height ?? 0), "the terminal stays above the sidebar's bottom edge").toBeLessThanOrEqual(layout.sidebar.bottom + 1);
      }
    } finally {
      await context.close();
    }
  });
}
