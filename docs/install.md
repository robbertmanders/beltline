---
title: Install
nav_order: 2
---

# Install
{: .no_toc }

1. TOC
{:toc}

## Requirements

- macOS 15 or later, or Linux with systemd (Ubuntu 22.04, Debian 12, Fedora 39 or similar), on arm64 or x86-64. On Windows, run the Linux build in WSL2.
- [`gh`](https://cli.github.com), signed in, together with `git` and `tmux`.
- To build it: Go (the version in `daemon/go.mod`), Node.js LTS and `make`.
- At least one agent CLI, signed in: Claude Code (`claude`), Codex CLI (`codex`), Copilot CLI (`copilot`), Cursor CLI (`cursor-agent`) or Pi (`pi`). See [Runners and models](runners.md).
- For Jira belts, Atlassian's `acli`, signed in.

On macOS, Homebrew installs the tools: `brew install gh git tmux go node`.

## Install from source

Beltline installs from the repository's `main` branch. There are no packaged releases.

```sh
git clone https://github.com/robbertmanders/beltline.git
cd beltline
make install
build/release/beltlined open
```

`make install` builds `build/release/beltlined`, with the web app compiled in, and registers it to start now and at every login: a LaunchAgent on macOS, a systemd user service on Linux. `beltlined open` signs your browser in to the running daemon and opens it.

Keep the clone where it is, since the service starts `build/release/beltlined` from it. Put that folder on your `PATH` to run `beltlined` by name.

On Linux, a user service stops when you log out. Run `loginctl enable-linger "$USER"` once to keep it running. [Configuration](configuration.md#running-at-login) covers both systems and WSL2.

## Try it without touching anything

```sh
make demo
```

This starts a separate daemon on demo belts, runs, issues and pull requests in a temporary folder. It doesn't call GitHub, Jira or any agent, and it doesn't touch your data folder. Ctrl-C stops it and removes the folder.

The demo is the quickest way to see every page: the Overview's Floor, a run with its stages and sessions, the belt editor, the Library and Insights.

## Upgrade

Pull `main`, install again and restart the daemon:

```sh
git pull
make install
build/release/beltlined restart
```

Agent sessions keep running through a restart, and the new daemon picks them up. When a new version changes the data folder's format, its first start upgrades the folder once and keeps a copy of every file it changes in `upgrade-backups/`. See [Configuration](configuration.md#the-data-folder).

## Check your setup

Open **Settings → Tools** in the browser. It shows whether `gh`, `git`, `tmux`, `acli` and each agent CLI were found, and how to install the ones that are missing. Then open **Settings → Allowed models** and allow the models your agents may use: fresh settings allow none.

## Uninstall

1. `beltlined uninstall` removes the LaunchAgent or systemd service and stops the daemon. Your data and agent sessions stay.
2. `tmux -L beltline kill-server` ends any agent sessions still running.
3. To delete every belt, agent and run, remove the data folder: `rm -rf ~/Library/Application\ Support/Beltline` on macOS, `rm -rf ~/.local/share/Beltline` on Linux. Back your library up to a [library repository](library.md) first if you might want it again.

## Next

[Set up your first belt](first-belt.md).
