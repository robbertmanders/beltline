---
title: Development
nav_order: 13
---

# Development
{: .no_toc }

1. TOC
{:toc}

[CONTRIBUTING.md](https://github.com/robbertmanders/beltline/blob/main/CONTRIBUTING.md) covers setting up and which checks to run. [AGENTS.md](https://github.com/robbertmanders/beltline/blob/main/AGENTS.md) is the full map of the code and its rules.

## Build and run

```sh
make quick       # vet and test the daemon (without tmux) and the web app
make build       # build beltlined with the web app compiled in, into build/bin
make web         # stage the web app for the next embedded build (does not rebuild beltlined)
make check       # build, then every test, the tmux ones included (about five minutes)
make run         # start beltlined on your data folder and open the browser; Ctrl-C stops it
make demo        # the same on fixture belts and runs, without GitHub or agents
make install     # build beltlined into build/release and start it at login
make uninstall   # stop starting it at login; data and agent sessions stay
beltlined open                   # sign a browser in to the running daemon
beltlined --check-library [folder]
make snapshots   # capture every page at three sizes (see Web/README.md)
make notices     # rewrite THIRD_PARTY_NOTICES after a dependency change
```

`make build`, `make run`, `make demo` and `make install` prepare the web app first and compile it into `beltlined`, so one executable serves the whole app with no `Web/dist` or `beltline-web/` folder beside it. A plain `go build ./cmd/beltlined`, `go vet` and `go test` need no Node.js: that Go-only build serves the API, and its web app shows a "Web app not built" page unless `BELTLINE_WEB_DIR` names a built web directory, such as `Web/dist` after `npm run build`. `BELTLINE_WEB_DIR` never takes the Vite dev server's URL; for development keep using `npm run dev`.

`make install` registers the `beltlined` it builds in `build/release/` as a LaunchAgent on macOS or a systemd user service on Linux: it starts at login and again if it exits. Keep that executable where it is while it is installed, since launchd keeps that path; the web app is inside it, so no other file needs to stay beside it. `BELTLINE_DATA_DIR=/path/to/data` and `BELTLINE_TMUX_SOCKET=name` set on the install command are saved for the daemon; without them it uses the normal data folder.

One daemon owns a data folder at a time. It locks `daemon.lock` in the folder (`flock`, recording its pid) before it reads or writes anything else, and keeps it until it stops. A second `beltlined` on the same folder exits and names the owner's pid; when the owner stops or dies the lock goes with it, so the next start takes over without deleting anything. Stopping the daemon leaves agent tmux sessions running, and the next start picks them up. Each `make demo` gets its own folder under `$TMPDIR/beltline-demo`, removed when it stops, so demos run side by side. `--check-library` only reads, and works while the folder is owned.

While it runs, `<data-folder>/daemon.json` (mode 0600) holds the port and a fresh token, and `<data-folder>/beltlined.log` keeps lifecycle and notification messages. The web app shows notifications in the browser once you allow them in Settings. Display choices such as folds, column widths and open sidebars are kept in the browser.

## Architecture

The daemon owns every fact and every decision: it reads and writes the data folder, runs agents, `gh`, `acli`, `git` and `tmux`, says which actions are available and why not, validates every save, and computes every view (statuses, cost, Work grouping, run links, which stage connects to which on the drawings, parsed Markdown). The web app lays out and draws what it is given in points, keeps your navigation, unsent input and display preferences, and sends your commands. Two browsers on the same data never disagree.

- `daemon/internal/core`, `integrations`, `kit`: the model, engine and presentation; GitHub, Jira, runners, workspaces and storage; the coordinator, launch setup and demo mode.
- `daemon/internal/server`: the HTTP and WebSocket API on Go's `net/http`, in the same process as the coordinator.
- `daemon/cmd/beltlined` and `daemon/internal/daemon`: the daemon executable, its LaunchAgent or systemd service, and the desktop actions it runs on your machine (Finder, VS Code, Ghostty).
- `Web/`: the React app, built with Vite and compiled into `beltlined` (`make web` stages it in `daemon/internal/server/webdist/`).

The daemon is written in Go, for a fast build and test suite; the API runs in its process so every command calls the coordinator directly. The Go dependencies are `coder/websocket` and `creack/pty` (the server's WebSockets and terminal), `golang.org/x/text` and `golang.org/x/sys`; the web app's are React, React Router, TanStack Query and xterm.js. See `AGENTS.md` for the full layout and the rules for adding a dependency.
