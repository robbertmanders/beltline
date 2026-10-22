---
title: Watches
parent: Concepts
nav_order: 3
---

# Watches
{: .no_toc }

A watch stage waits for something to happen and ends in an outcome when it does. Put it after the agent that opens the pull request.
{: .lead }

1. TOC
{:toc}

## Watching the pull request

A watch waits on the run's pull request: the one under review on a review belt, or the newest one from the run's branch in each repository where that branch has commits to merge. It reports one of these events, each an outcome you configure:

| Event | When |
| --- | --- |
| **Merged** | The pull request was merged. |
| **Closed** | It was closed without merging. |
| **Failing checks** | A required check failed. |
| **Conflicted** | It has merge conflicts with its base. |
| **Behind** | Its branch fell behind the base branch. It's reported only after two checks, so a base that is only catching up is left alone. |
| **Approved**, **Commented**, **Changes requested** | A review was submitted on the current head commit. Plain pull request comments don't count. |
| **New changes** | New commits on a pull request under review. |
| **None** | Optional: taken on a check where none of the other outcomes applies, including the first quiet check after the run arrives. Without it, the watch keeps waiting. |

A new watch moves on when merged. It holds on a closed pull request, conflicts and reviews, since the stages after a watch expect a merge: run the watch again to keep waiting, and the same review isn't reported twice. For failing checks or a branch behind, it runs the nearest earlier agent again, at most twice.

## The fork after the watch

A new issue or Jira belt ends in a fork after its watch instead:

| Route | Taken on | Stage |
| --- | --- | --- |
| Merged | merged | **Wrap up** tidies up after the merge, such as closing the issue or moving the ticket. |
| Conflicts | conflicted, behind | **Resolve conflicts** merges the base branch in, keeps both sides' intent, runs the tests and pushes. It never force-pushes. |
| Failing checks | failing checks | **Fix checks** reads the failing logs, fixes them and pushes. |

Resolve conflicts and Fix checks go back to the watch with a **Go to a stage**, at most twice for each event, and hold when they need you. Their routes *feed in*: nothing on the line splits into them, so only the watch's outcomes reach them.

## Watching the issue or ticket

A watch can instead wait on the run's own issue or ticket (**Waits on: the issue or ticket**). It reports one of three events:

- **Children changed**: a ticket under it, such as a sub-issue or an epic's child, was added or changed status.
- **New comment**: someone commented on it.
- **Closed**: it was closed.

It gathers changes for a quiet period and reports them together, and the agent after it decides what they mean. What it saw stays on the run across visits, so it reports only what's new. The [epic belt](belts.md#epics) uses this to follow its waves of tickets.
