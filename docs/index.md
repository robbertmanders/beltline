---
title: Home
nav_order: 1
description: Beltline turns GitHub issues, pull request reviews and Jira tickets into pull requests, with the coding agents you already have.
permalink: /
---

# Beltline
{: .fs-9 }

Turn GitHub issues, pull request reviews and Jira tickets into pull requests, with the coding agents you already have.
{: .fs-6 .fw-300 }

[Install Beltline](install.md){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View it on GitHub](https://github.com/robbertmanders/beltline){: .btn .fs-5 .mb-4 .mb-md-0 }

---

Each work item rides a **belt** of small stages. Agents triage, spec, build, verify and open the pull request. Approvals wait for your decision, and a watch waits for the merge. Every stage ends in a named outcome, and you decide what each outcome does next.

Beltline runs on your own machine, macOS or Linux. `beltlined` is a small daemon that runs your belts whether or not a window is open, and you work with it in your browser.

![The Overview's Floor: every belt as a lane, with each stage's runs and the ones that need you](images/overview.png)

{: .warning }
Beltline runs coding agents **with their approval prompts turned off**, as your user and with your `gh` and `acli` sign-ins. Agents read issue and ticket text that other people may have written. Read the [security model](security.md) before you point a belt at a repository, and start with a label only you can apply.

## What it does

- **Bring your own agents.** Stages run Claude Code, Codex CLI, Copilot CLI, Cursor CLI or Pi in tmux sessions you can watch live and continue yourself.
- **Small stages, named outcomes.** A triage stage can end in *ready to implement*, *ready to spec*, *needs info* or *wait*. Each outcome says what happens next: the next stage, another stage, a fork's route, run again, hold for you, park or drop.
- **You stay in the loop where it matters.** Approval stages wait for your decision. Held runs list their questions under **Needs you**, and you answer them from the browser.
- **It follows the pull request to the end.** A watch stage waits for the merge, and sends conflicts, a branch that fell behind and failing checks to agents that fix them.
- **Many sources of work.** GitHub issues with a label, pull requests that ask for your review, Jira tickets from a JQL query, a schedule, or tasks you start yourself.
- **Measure and improve.** Insights shows first-try rates, time and cost per stage, and a shadow test runs a new version of a belt beside the current one.
- **Local and private.** The daemon serves its API on `127.0.0.1` only and sends no telemetry.

## Where to start

| If you want to | Read |
| --- | --- |
| Install it and look around with demo data | [Install](install.md) |
| Set up a belt for your own repository | [Your first belt](first-belt.md) |
| Understand belts, stages, outcomes and runs | [Concepts](concepts.md) |
| Write or change an agent | [Agents](agents.md) |
| Know what agents can do on your machine | [Security model](security.md) |
| Build Beltline or contribute | [Development](development.md) |
