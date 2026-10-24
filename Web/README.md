# Beltline web shell

Install and run the web shell from this directory:

```sh
npm ci
npm test
npm run build
npm run dev
```

Vite serves the app on `http://127.0.0.1:5173` and proxies `/api`, `/auth`, and `/artifacts` to `http://127.0.0.1:7331`. That dev server stays the development path: `npm run build` writes `dist/`, which `make web` stages for the embedded executable, and `BELTLINE_WEB_DIR=<absolute path to dist>` lets a Go-only daemon serve that build. `BELTLINE_WEB_DIR` takes a directory, never the Vite URL. Set `BELTLINE_DAEMON_PORT` when the daemon selected another port, or set `BELTLINE_DAEMON_ORIGIN` to its exact `http://127.0.0.1:<port>` origin. The proxy rewrites Host and Origin to the daemon authority for its loopback checks and supports WebSocket upgrades for events.

To sign in during development, open `http://127.0.0.1:5173/auth?token=<daemon-token>`. The daemon exchanges that token for its HttpOnly cookie and redirects to the app root; the web client does not store credentials.

Run details request the daemon's run view, whose stage boxes the daemon lays out. Each run has one cached view per selected stage; while another stage's view loads, the current run stays mounted so its focused controls and stage panel do not blink out. Navigating to a different run does not retain the previous run's view.

## Theme review

Open `http://127.0.0.1:5173/_gallery` while `npm run dev` is running. The gallery has Tokens, Fonts, Fields and Components sections and does not need a running daemon. The route and its module are excluded from ordinary production builds. The snapshot command sets `VITE_BELTLINE_SNAPSHOTS=1` for its own web build, which its isolated Go-only daemon serves through `BELTLINE_WEB_DIR`, so Playwright can capture the same existing route without adding it to shell navigation.

For a browser capture, use a 1080 × 680 viewport and use the section links to capture the changed section. For example, with Chrome DevTools AXI installed: `chrome-devtools-axi open http://127.0.0.1:5173/_gallery`, `chrome-devtools-axi resize 1080 680`, then `chrome-devtools-axi screenshot /absolute/path/theme-gallery.png`.


## Playwright snapshots

Run `make snapshots` from the repository root. After `npm ci` has installed the web dependencies, install Playwright's matching Chromium once with `cd Web && npx playwright install chromium`. Snapshot runs never install a browser automatically and fail with this command if Chromium is missing.

The default writes the shell, Work, Agents library/editor and AFK/Settings captures to `build/web-snapshots`, each at 1080×680, 1440×900 and 2560×1440. The shell routes include Overview, Work, Belts, Agents, Library, AFK and Settings with navigation open and closed, the run with Sessions opened by a real click (`run-sessions-click`), plus the independent `/_gallery` page. The Run cases below (24 cases) draw daemon-provided runs. Work also has repository, status, epic and no-grouping views at `/work?groupBy=…`. Agents captures include `agents-folded`, `agents-new`, `agents-dirty`, `agents-save-problems`, `agents-used-by`, `agents-empty`, `agents-cursor`, `agents-model-unavailable`, `agents-long-text`, `agents-archived`, `agents-revealed`, `agents-back-hidden` and `agents-reset-confirmation`. Assistant cases are `belts-assistant-stopped`, `belts-cursor-assistant`, `belts-assistant-running-demo`, `belts-assistant-stage-open`, `agents-assistant-running` and `agents-assistant-stopped`. Archived captures use an idempotent typed API fixture; empty-library, Cursor and unavailable-model captures use typed response fixtures; reset confirmation offers reset without changing saved demo data. AFK and Settings states include `afk-all`, `native-7g-afk`, `native-7i-afk-on`, `native-7k-afk-decided`, `native-7m-overrule-row`, `settings-models-expanded`, `native-7e-settings`, `native-7g-settings-paused`, `native-7j-settings-continued`, `native-7f-settings-factories`, `settings-model-error` and `settings-inline-422`. Case files live in `Web/snapshots/cases/`, one per page, and are registered in `Web/snapshots/cases.ts`; typed steps (`click`, `set-button`, `fill`, `select`, `press`, `wait`) find targets by accessible role and name or selector. Cases may also set API responses, and use a readiness marker that must match exactly one visible element before capture. PNG names use `<case-id>-<width>x<height>.png`. Set `BELTLINE_SNAPSHOTS` to choose an output directory; reruns replace matching PNGs and leave unrelated files alone.

The Run cases wait for the daemon-drawn `.box-map` and a stage panel, never the old resource scaffold. Most draw the seeded `acme/platform#139` run; a case can name a different seeded run with a `run` selector (`beltId`, issue `number`, `repo`, `key` or `state`), which the harness resolves from `/api/v1/runs` and fails by case id when nothing matches. `run-sessions-click` opens the read-only Sessions panel by a real click, `run-wide` captures the run with Sessions already open, the `run-wrapping-*` cases select the `spec-1` attempt whose output the snapshot daemon replaces with long Markdown (gated by `BELTLINE_DEMO_LONG_MARKDOWN=1`), `run-controls-open` clicks the real Run controls button and captures the floating command menu, and `run-reduced-motion` captures with the browser's reduced-motion preference.

Sessions layout checks. `capture.spec.ts` runs `run-wrapping-closed`, `run-wrapping-open`, `run-sessions-click` and `run-wide` at 1080×680, 1440×900 and 2560×1440 with the column overflow detector, and writes no screenshot (`captureImage: false`). A separate test renders one and eight synthetic session rows and asserts that the list hugs one row and scrolls inside its 232 px cap at eight.

`BELTLINE_SNAPSHOT_ONLY=work` captures all seven Work cases. Comma-separated prefixes select their union, for example `BELTLINE_SNAPSHOT_ONLY=overview-open,gallery`. Set `BELTLINE_SNAPSHOT_SIZE=1280x720` to capture one custom positive-integer size; the case and size filters compose. Invalid sizes and prefixes that match no case fail clearly. Progress prints each completed case, viewport and path. The wrapper starts a private demo daemon on the next available loopback port, waits up to 30 seconds for its ready line, reads that daemon's discovery file and signs each isolated capture context in through `/auth`. It closes the browser run, stops and waits for its daemon, and removes its private demo data on success, failure, SIGINT or SIGTERM. Completed PNGs remain available after a failed or interrupted capture. Overflow failures name the case, size, column, element and measured right-edge excess; clipped and horizontally scrolling descendants are checked too.
