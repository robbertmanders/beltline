---
title: Belts and triggers
parent: Concepts
nav_order: 1
---

# Belts and triggers
{: .no_toc }

1. TOC
{:toc}

## A belt

A belt is one or more triggers, the repositories they watch, a base branch, its policies, and its stages and forks. Edit it on **Belts**: the drawing at the top shows the trigger, the stages and the forks, and the form below it edits what you select.

Every save is a new numbered version (`belts/<id>/v<n>.json` in the data folder). A saved version never changes. New runs start on the newest version, and a run keeps the version it started on to the end, with copies of the agent versions its stages use.

From a belt's page you can:

- **Pause belt**: take no new work items and start no queued agent stages until you resume it. **Pause everything**, at the top of every page, pauses all of Beltline at once.
- **AFK**: set the belt's [AFK mode](afk.md), which each new run starts with.
- **Copy belt**: start a new unsaved belt named `Copy of …`, with the same stages, forks, triggers, policies and repositories. It takes no work until you save it.
- **Archive**: retire it. An archived belt takes no new work and is listed under **Archived** on Belts, where you can restore it.
- **Share…**: send it to a shared [library repository](library.md).
- **Agent**: open the configuration assistant on it.
- Start a [shadow test](shadow-tests.md) of a new version.

## Triggers

| Trigger | Starts a run for | Settings |
| --- | --- | --- |
| **GitHub issues** | every open issue with a label | the label (default `agent`), assignee *anyone* or *me*, how often to check (default every 60 seconds) |
| **Review requests** | every pull request that asks for your review, except one from a fork (see the [security model](security.md)); a new request on the same pull request starts a new run | reviewers *me* or *me and my teams*, how often to check |
| **Jira** | every ticket a JQL query finds | the JQL, how often to check |
| **Schedule** | a task at a time of day | every day, weekdays or one day a week, the time, and the task's title and description |
| **Manual** | a task you start with **Start a task…** | none |

A belt can have more than one trigger, each with its own repositories, for example the issues of two repositories with different labels. Every stage must be able to work with what every trigger provides: a review agent needs a pull request, so it can't follow an issue trigger. Saving checks this and says what's missing.

## Repositories and base branches

Each trigger lists the repositories it watches as `owner/repo`. Runs work in clones Beltline keeps in the data folder, and agents work in worktrees on the run's branch, `beltline/issue-<n>` for an issue.

The base branch is the repository's default unless you set one for the belt, or one per repository under **Repository base branches**. A `base:` label on an issue or ticket, such as `base:release/2.4`, sets that run's base branch.

**Jira belts** can list a project's application and infrastructure repositories. Each ticket starts one run with that repository list frozen. The agent's working folder has one checkout per repository, agents choose where to work and open their own pull requests on the shared run branch, and the watch waits for every repository's pull request: all merged moves on, and any closed without merging holds. A missing pull request holds after ten minutes, counted from the first pull request found, or from the watch's start when none appears.

## Policies

Each belt has four policies, all on by default except **Comment when held**, in the editor once a trigger watches issues, review requests or tickets. A run keeps the policies it started with.

| Policy | What it does |
| --- | --- |
| **Triage again when edited** | When the issue or ticket is edited before a stage changed code, start the run again at the first stage. For review requests: review an edited pull request again. |
| **Stop when closed** | When the issue is closed, the ticket finished or the review no longer requested, stop the run, unless it has reached a watch stage. |
| **Comment when held** | Comment on the issue, pull request or ticket when a stage holds the run. Off by default: Beltline posts the agent's questions or output without you reading it first, where anyone who can see the issue reads it. |
| **Wait for blockers** | Leave an issue or ticket alone while an open issue or ticket blocks it. Work lists it as waiting. |

## Epics

An epic belt breaks a large issue or Jira epic into waves of tickets that ordinary belts then build. Its built-in agents:

1. **Plan the epic** writes the plan, and you approve it.
2. **Open the feature branch** opens the run's branch as the epic's feature branch.
3. **File the wave** files the first wave of tickets, each with a `base:` label for the feature branch, so their pull requests merge into it.
4. A watch on the epic's own issue or ticket waits for its children to change, a comment, or the epic to close.
5. **Assess the epic** decides: *wait* goes back to the watch, *wave done* replans the next wave (**Replan the epic**, an approval, **File the wave**), and *epic done* lands it: **Land the epic** opens the pull requests that merge the feature branch, a watch waits for the merge, and Wrap up tidies up.
