---
title: Library
nav_order: 8
---

# Library
{: .no_toc }

Your belts and agents are your library. Library repositories keep it in git: one to back up your own, and shared ones to install a team's or the examples.
{: .lead }

1. TOC
{:toc}

## Library repositories

Add repositories in **Settings → Library repositories**. Each is a git repository laid out like your library, optionally under a folder:

```text
belts/<id>/v<n>.json
agents/<id>/v<n>.md
```

Beltline keeps a clone of each in the data folder (`library-repos/`) and syncs it to its default branch when you refresh it, install from it, back up or share. Git runs with your own `git` and `gh` sign-ins, and nothing else is sent anywhere.

## The examples library

Beltline ships no belts or agents of its own: a fresh install starts with an empty library. The examples are in a shared library repository, [`robbertmanders/beltline-examples`](https://github.com/robbertmanders/beltline-examples), which **Settings → Library repositories** lists from the start. Nothing is installed from it until you choose to: open **Library**, refresh it, and install the belts and agents you want. It holds a belt for each trigger (GitHub issues, review requests, Jira tickets, an epic in waves, tasks you start and a schedule) and the agents they use.

Upgrading from a version that shipped built-in agents adds the examples repository to your settings once, and saves into `agents/` every built-in agent version your belts and runs use, so they keep working. Remove the repository in Settings if you don't want it; it isn't added again.

## Backing up

A **backup** repository holds your own library.

- **Back up** copies every belt and agent version to it and pushes to its default branch. The sidebar shows *Changed since backup* when your library has changes the backup doesn't.
- **Restore…** moves your current belts and agents aside into `library-backups/<time>/` in the data folder and copies the backup into place. Your paused and archived state stays on this machine, and runs keep their own copies of what they use.

## Installing and sharing

A **shared** repository holds a team's belts and agents.

- **Install** writes an item's version, and the agent versions it uses, into your library, with the repository recorded as its origin. A shared belt names no repositories, so a belt new to your library is installed paused: open it, add your repositories (and the label, query or schedule you want), save and resume it.
- **Update** takes a newer version the same way. **Compare** shows what changed.
- **Share…**, on a belt's or agent's page, sends your item to a shared repository as a pull request, or as a direct push when that repository's setting says so.

To move an old factory or marketplace repository to this layout, run `scripts/migrate-library-repo.sh <path-to-a-clone>` in a clone of this repository and commit what it copies.

## The configuration assistant

**Agent**, on the Belts and Agents pages, opens a conversation with an agent that edits your library for you. See [Configure with an agent](configuration.md#configure-with-an-agent).
