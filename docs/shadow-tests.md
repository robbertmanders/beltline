---
title: Shadow tests
nav_order: 9
---

# Shadow tests
{: .no_toc }

A shadow test runs a new version of a belt beside the live one on real work, so you can compare them before you switch.
{: .lead }

1. TOC
{:toc}

## Starting a test

On a belt's page, choose **Create shadow…** and edit the shadow like any belt: a different model for Build, a stricter Verify, a stage more or less. The shadow takes the live belt's triggers, name, base branch and repositories, so both versions see the same work. A belt has at most one shadow, and the editor's **Live** and **Shadow** tabs switch between them.

While the test runs, each new run starts on whichever version has had fewer of the test's runs so far (a tie is drawn at random), and keeps that version to the end. A shadow with warnings sends every new run to live.

## Following it

- Runs on the shadow leave the Overview's Floor and are listed in their own **Shadow** section on Work.
- The belt's page shows how many runs each version has taken. **Compare on Insights** compares the test's finished runs: first-try rates, time, cost and stage visits for each version. See [Insights](insights.md).

## Ending it

- **Accept shadow…** makes the shadow live. It keeps its version number when it's the newest, or is saved again as the next version.
- **Discard shadow…** keeps live as it is.

Either way the test ends. Runs already on either version finish on it.
