---
title: Runners and models
nav_order: 6
---

# Runners and models
{: .no_toc }

1. TOC
{:toc}

## Runners

An agent stage runs one of these command-line agents (the stage's *harness*) in a tmux session on a private server, `tmux -L beltline`. Install at least one and sign it in; **Settings → Tools** checks them.

| Runner | Executable | Override |
| --- | --- | --- |
| Claude Code | `claude` | `BELTLINE_CLAUDE` |
| Codex CLI | `codex` | `BELTLINE_CODEX` |
| Copilot CLI | `copilot` | `BELTLINE_COPILOT` |
| Cursor CLI | `cursor-agent` (from [cursor.com/install](https://cursor.com/install), signed in with `cursor-agent login`) | `BELTLINE_CURSOR` |
| Pi | `pi` | `BELTLINE_PI` |

{: .warning }
Every runner starts with its approval prompts turned off, so a stage never stalls on a prompt. Read the [security model](security.md) before you point a belt at text other people can write.

## Allowed models

**Settings → Allowed models** reads each runner's model catalog with its configured executable and existing sign-in, without sending a prompt or starting a session. Search it and check the exact models agents may use; fresh settings allow none. Pi's model names include their provider.

- The model pickers for agents, belt stages and the configuration assistant offer only allowed models that are in the catalog, and saving or starting with a blank or disallowed model is refused.
- **Refresh** reads the catalogs again and keeps the last good one when a runner fails. Changing a runner's executable clears its choices.
- A new run copies the allowed models when it starts. Later changes neither take away nor add to what it may use. Assistant conversations keep theirs until they end.
- Catalogs and allowances stay on this machine; they're never part of a library repository.

## Thinking

Thinking is set next to Model: Low, Medium, High, Max, or **The runner's default**. New stages take the agent's level, and a belt can change it for one stage. In an agent file it's an optional `thinking: high` line.

| Runner | Passed as | Levels |
| --- | --- | --- |
| Claude Code | `--effort` | all |
| Codex CLI | `-c model_reasoning_effort=…` | Low to High |
| Copilot CLI | `--reasoning-effort` | all |
| Cursor CLI | not passed: it uses the model's own reasoning settings | none |
| Pi | `--thinking` | all |

The runner's default passes no flag, and continuing a session by hand leaves the level alone. A level the runner doesn't take blocks saving the belt or agent. One that reaches a run anyway, as the assistant's setting or through an import, passes the highest level the runner takes below it. Cursor keeps a saved level for when you switch back to another runner.

## How each runner is run

**Claude Code**
: Runs headless (`claude -p`) with `--allow-dangerously-skip-permissions --permission-mode bypassPermissions`, so no trust or permission prompt stalls it, and its exit code says when it's done. Its transcript reports errors in Claude's own words, and when the output file lacks an outcome, its last reply supplies it. Answering a held stage resumes the same session headless; **Continue** opens it in Claude's interactive interface.

**Codex CLI**
: Runs headless (`codex exec --dangerously-bypass-approvals-and-sandbox --skip-git-repo-check`), outside Codex's sandbox so it can commit in a worktree and push. Answering a held stage resumes the checkout's newest Codex session with `codex exec resume`, or starts a fresh one when there is none. **Continue** opens `codex resume` interactively.

**Copilot CLI**
: Runs with `--allow-all`, and `--no-ask-user --autopilot` in interactive sessions. Its costs come from its usage checkpoints or shutdown report.

**Cursor CLI**
: Runs headless (`cursor-agent --print --force --trust`) in a chat Beltline creates first with `cursor-agent create-chat` and saves on the attempt, so answering a held stage, **Continue** and a daemon restart all resume that chat. Cursor keeps no transcript Beltline reads, so its outcome must be in the output file, and it reports no usage.

**Pi**
: Runs with `--approve`.

## Live checks against the real CLIs

These opt-in tests aren't part of `make check`. Run them from `daemon/`. Each is skipped, and makes no outside calls, unless its variable is set:

- **Claude:** a triage, `BELTLINE_REAL_CLAUDE=1 BELTLINE_REAL_CLAUDE_MODEL=haiku go test -tags integration ./internal/kit/coordinator -run TestRealClaude`.
- **Cursor:** `BELTLINE_REAL_CURSOR=1 BELTLINE_REAL_CURSOR_MODEL=<model> … -run TestRealCursor`.
- **Codex:** a triage run and resuming the checkout's own session, `BELTLINE_REAL_CODEX=1 … -run TestRealCodex`. `BELTLINE_REAL_CODEX_MODEL` picks a model.

[AGENTS.md](https://github.com/robbertmanders/beltline/blob/main/AGENTS.md) lists the others.
