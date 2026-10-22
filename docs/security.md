---
title: Security model
nav_order: 12
---

# Security model
{: .no_toc }

1. TOC
{:toc}

Beltline runs coding agents on your machine, as you, with your GitHub and Jira sign-ins. This page explains what it protects, what it can't protect, and how to run it safely. To report a vulnerability, see [SECURITY.md](https://github.com/robbertmanders/beltline/blob/main/SECURITY.md): use GitHub's private vulnerability reporting, not a public issue.

## Agents run with full permissions

This is the most important part. Every agent stage starts an agent CLI in tmux **with its approval prompts turned off**, so nothing stalls waiting for a person:

| Runner | Flags Beltline passes |
| --- | --- |
| Claude Code | `--allow-dangerously-skip-permissions --permission-mode bypassPermissions` |
| Codex CLI | `--dangerously-bypass-approvals-and-sandbox` |
| Cursor CLI | `--force --trust` |
| Copilot CLI | `--allow-all` (and `--no-ask-user --autopilot` in interactive sessions) |
| Pi | `--approve` |

An agent can therefore run any command your user can run. It can read and write any file you can, use the network, and push, comment, open pull requests, label issues and move Jira tickets with your `gh` and `acli` sign-ins. Beltline doesn't sandbox agents.

**Issue, pull request and ticket text is untrusted input.** A belt hands the title, body and comments of the issue, pull request or ticket it works on to an agent as part of its brief. Anyone who can write that text can try to steer the agent (prompt injection). That includes anyone who can open an issue on a public repository you watch, or comment on a pull request that asks for your review.

Recommendations:

- **Choose triggers carefully.** Only trigger belts from repositories, labels and JQL queries where you trust everyone who can write the text. On a public repository, use a label that only maintainers can apply.
- **Limit what the sign-ins can do.** Sign `gh` in with a fine-grained token scoped to the repositories the belts work on, and use a Jira account that can only reach the relevant projects.
- **Separate it from your personal data.** Run Beltline under a dedicated user account, or on a dedicated machine or VM, without your personal credentials, SSH keys or cloud sessions in reach.
- **Keep approvals in the belt.** Put an approval stage before any stage whose agent pushes, merges or posts. Review what agents ship: the built-in Ship and Post review agents open pull requests and post reviews, but nothing merges without the policies you configure.
- **Treat AFK mode as delegation** (below).

## AFK mode decides on your behalf

When a run is in AFK mode, a run that stops for you (an approval, a held agent's questions, a failed stage) gets a short agent session that decides instead of you. That agent runs with the same full permissions. The allowances in Settings → AFK limit *what it may choose*: answer, run again, approve, carry on after holds, or drop. They don't limit what the deciding agent can do on your Mac. Every decision is saved, marked on the attempt it ended and listed on the AFK page until you check it. Only turn AFK on for belts whose triggers you trust.

## The local server

`beltlined` serves the web app and its API on your machine only:

- **Loopback only.** It listens on IPv4 `127.0.0.1`, at port 7331 or the next free port above it, never on another interface.
- **Exact Host and Origin.** Every request must name `127.0.0.1:<port>` as its `Host`, and any `Origin` must be exactly `http://127.0.0.1:<port>`. Anything else gets 403. This stops DNS-rebinding attacks and other websites from calling the API from your browser.
- **A token for the API.** `/api` needs the daemon's token, as `Authorization: Bearer <token>` or the `beltline_auth` cookie. Each daemon start makes a fresh random token and publishes it, with the port, in `<data folder>/daemon.json`, mode `0600`, so only your user can read it. `GET /auth?token=` checks the token in constant time and sets the cookie (`HttpOnly`, `SameSite=Strict`, `Path=/api`) before redirecting to `/` with `Referrer-Policy: no-referrer`. `beltlined open` makes that request for you. The URL with the token can stay in the browser's history until the daemon restarts.
- **Terminals.** `GET /api/v1/terminals?session=` attaches a tmux session over a WebSocket, with the same Host, Origin and token checks. It is read-only unless the session is one you continued or the configuration assistant's. On a read-only session the daemon drops your input instead of trusting the browser to withhold it.
- **HTML artifacts.** Agents can write `.html` artifacts. They are served from `/artifacts/…`, outside `/api` and without the cookie, through a link signed with HMAC-SHA256 that expires after 60 seconds. They come with `Content-Security-Policy: sandbox allow-scripts` (an opaque origin with no access to the app or its cookie), `X-Content-Type-Options: nosniff` and `no-store`. Files are opened below the attempt folder with `openat` and `O_NOFOLLOW`, so a symbolic link can't lead out of it. An artifact's scripts can still run and reach the network when you open it.
- **Other processes on your machine.** Any process running as your user can read `daemon.json` and so call the API. Beltline doesn't defend against software already running as you.

## What is written to disk

Everything lives in the data folder, `~/Library/Application Support/Beltline/` on macOS and `~/.local/share/Beltline/` on Linux by default:

- belts, agents and settings
- runs, including each session's brief, output and transcript
- clones of the library repositories
- `daemon.json` (port and token, `0600`)
- `beltlined.log`, which holds lifecycle and notification lines but no tokens; it moves aside to `beltlined.log.1` past 5 MB

Agents work in git worktrees whose files hold whatever the agent wrote. Beltline stores no GitHub or Jira credentials: it calls `gh` and `acli`, which keep their own.

Beltline sends no telemetry. The only network calls it makes are the ones `gh`, `acli`, `git` and the agent CLIs make for it.

## Reviewing the code

These files carry the checks above, with tests next to them:

- `daemon/internal/server/server_host_origin.go`: the Host and Origin checks
- `server_authentication.go` and `server_auth_route.go`: the token and the cookie
- `terminal_route.go` and `terminal_pty_bridge.go`: terminals, with `terminal_security_test.go`
- `routes/routes_artifact_routes.go` and `routes/routes_artifact_link_signer.go`: artifacts
- `daemon/internal/integrations/store`: the confined artifact reads
- `daemon/internal/integrations/runner`: the agent flags
