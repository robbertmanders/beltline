---
title: Configuration
nav_order: 11
---

# Configuration
{: .no_toc }

1. TOC
{:toc}

## The data folder

Everything Beltline keeps lives in its data folder: `~/Library/Application Support/Beltline/` on macOS, `$XDG_DATA_HOME/Beltline/` on Linux (`~/.local/share/Beltline/` when `XDG_DATA_HOME` is unset), or the folder `BELTLINE_DATA_DIR` names. That includes belts, agents, settings, runs, specs, clones of library repositories, and every session's brief, output and log. One daemon owns a data folder at a time. It locks `daemon.lock` in the folder (`flock`, recording its pid) before it reads or writes anything else, and keeps the lock until it stops. A second `beltlined` on the same folder exits and names the owner's pid.

While it runs, `daemon.json` (mode 0600) holds the port and a fresh token, and `beltlined.log` keeps lifecycle and notification messages. The log moves aside to `beltlined.log.1` past 5 MB. Display choices such as folds, column widths and open sidebars are kept in the browser.

The data folder records its format in `format.json`. When a new version of Beltline changes the format, its first start rewrites the folder once. Before rewriting, it copies each file it changes to `upgrade-backups/<time>/`. A version of Beltline that is older than the folder's format refuses to start rather than risk your data. Before 1.0 the format and the API may still change, always with an automatic upgrade.

## Environment variables

| Variable | What it does |
| --- | --- |
| `BELTLINE_DATA_DIR` | Use another data folder. |
| `BELTLINE_TMUX_SOCKET` | The private tmux server's name (default `beltline`). |
| `BELTLINE_PI`, `BELTLINE_COPILOT`, `BELTLINE_CLAUDE`, `BELTLINE_CODEX`, `BELTLINE_CURSOR` | Other agent executables. |
| `BELTLINE_WEB_DIR` | Serve a built web directory (such as `Web/dist`) instead of the app compiled into `beltlined`; for development. |

`beltlined install` saves `BELTLINE_DATA_DIR` and `BELTLINE_TMUX_SOCKET` for the service when they are set on the command.

## Running at login

`beltlined install` registers the executable it runs from with the system's service manager. The service starts Beltline at login and again if it exits. `make install` registers `build/release/beltlined` in your clone, so a later `make install` replaces it in place. After an upgrade, `beltlined restart` stops the running daemon and starts the new one. Stopping the daemon leaves agent tmux sessions running, and the next start picks them up.

- **macOS:** a per-user LaunchAgent, `com.beltline.daemon`, in `~/Library/LaunchAgents/`.
- **Linux:** a systemd user service, `beltline.service`, in `~/.config/systemd/user/` (`$XDG_CONFIG_HOME/systemd/user/`). `systemctl --user status beltline` shows it, and `journalctl --user -u beltline` and the data folder's `beltlined.stdout.log` and `beltlined.stderr.log` hold its output. A user service stops when you log out; `loginctl enable-linger "$USER"` keeps it running. Without a systemd user manager (some containers, WSL2 without systemd), `install` says so: run `beltlined` in a terminal or under your own supervisor instead.
- **Windows:** run the Linux build in WSL2. Turn on systemd in WSL (`[boot] systemd=true` in `/etc/wsl.conf`) for `beltlined install`, and install `wslu` so `beltlined open` can hand the sign-in link to your Windows browser: WSL forwards `127.0.0.1` to Windows.

On Linux, desktop actions use `xdg-open` (reveal a folder, open a file or link), `code` for VS Code and `ghostty` for Ghostty, and notifications go through `notify-send` when it is installed.

## Uninstalling

See [Install](install.md#uninstall).

## Configure with an agent

**Agent** on the Belts and Agents pages opens a sidebar with a Pi, Copilot, Claude Code, Codex or Cursor conversation that changes your belts and agents for you. Pick the runner, an explicit allowed model and thinking there; they are saved locally. Blank models require repair in Settings → Allowed models before a new conversation can start. Describe what you want and **Start**; the conversation continues in the sidebar's terminal and survives a restart of the daemon.

There is no draft: the agent writes straight into `belts/` and `agents/`, and the daemon reloads them as it does. New runs use the changes right away; runs already going keep the versions they started with. Like the editor, it adds a new version for every change and never rewrites a saved one, and it asks before it archives, pauses or removes anything. Files it changed that can't be read, or that Save would refuse, are listed in the sidebar; a belt or agent whose newest version can't be read is missing from the app until the agent fixes it.

Save your edits before you start: with unsaved changes on the page, **Agent** and **Start** warn you first, since the agent can overwrite or remove what you are editing.

## Library repositories

**Settings → Library repositories** lists the git repositories that back up your library or share a team's. See [Library](library.md).

## The local server

`beltlined` serves the web app and its API on `127.0.0.1` only, at port 7331 or the next free port above it. [security.md](security.md) describes its checks. The API is versioned under `/api/v1` (`GET /api/v1/version`). Errors are `{"message":…,"problems":[…]}`, with 401, 403, 404, and 422 when the daemon refuses a command. `GET /api/v1/events` is a WebSocket that tells the browser which resources changed.
