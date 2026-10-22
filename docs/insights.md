---
title: Insights
nav_order: 10
---

# Insights
{: .no_toc }

Insights shows how well your belts work: how often a stage gets it right the first time, how long it takes and what it costs, and how that changes as you change the belt. Turn it on with **Show the Insights page** in **Settings → General**.
{: .lead }

1. TOC
{:toc}

## Every belt at a glance

The Insights page lists every belt with its first-try rate as a 7-day rolling line, and the change against the 7 days before. A first try *passes* when its outcome's tone is good.

## One belt over time

A belt's Insights opens on its **Evolution**: its finished runs in columns, one per version (small versions are merged until a column has five runs) or per week they finished. The recent columns are compared with the ones before them, by default the last third of the runs against the rest.

Six figures each show the recent value and its change, a statistic and the middle half of each column, and one dot per run:

- **Lead time**, from the run's start to its end
- **Time to first pull request**
- **Cost**
- **Stage visits**
- **Approvals and holds**
- **Ship rate**

Below them: where the time goes (agents, waiting on you, watching the pull request, the rest), approvals, holds and AFK decisions, sends back to an earlier stage, tries per run, cost by stage, and how runs end.

## One belt's stages

The stage view lists each stage of the belt with its visits, visits per run, tries per visit, first-try rate, median time and cost per visit. When a stage spreads its sessions over [other models](stages.md#agent-stage-settings), its models are ranked. You can narrow it to one version of the belt.

A *visit* is one entry of a run into a stage, so a run sent back to a stage visits it again. Stopped attempts count nowhere, and paused time is left out of durations. A run on a [shadow version](shadow-tests.md) counts towards the live version it was tested against, and the shadow comparison shows the two side by side.

Insights reads only the runs in your data folder, so erasing a run removes it here too.
