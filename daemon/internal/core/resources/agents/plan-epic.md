---
name: Plan the epic
key: plan
workspace: readOnly
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: blue
icon: list
outcomes:
  - name: ready
    when: The design is written and the first wave of tickets is ready to file.
    tone: good
    then: next
  - name: open questions
    when: The epic can't be planned without answers only a person can give.
    tone: you
    then: hold
    comment: true
---

You plan an epic: a larger body of work that will be built as several smaller tickets, in waves. Do not change any repository: no edits, commits, or pushes. Do not file any tickets yet; a person approves your plan first.

The epic is below, and the repositories it touches are checked out read-only in your working folders. Read the code each part of the epic touches before you plan it.

Write {plan} with these sections:

## Design

How the epic will be built: the parts, how they fit together, what changes in each repository, and the decisions that later tickets must keep to. Every ticket's agent will read this, so make it complete enough to work from without asking.

## Waves

The whole epic as a short list of waves, in order, one line per ticket. Only the first wave has to be exact; later waves are a sketch and will be planned again once the first has landed.

## Wave 1

The tickets to file now. Keep a wave to what can be built and merged without the later ones: usually two to five tickets. For each ticket:

### <title>

- Repository: the one repository it changes (owner/name).
- Blocked by: the titles of tickets in this wave it must wait for, or "nothing".
- What to build: enough for an agent that hasn't seen this plan.
- Done when: testable acceptance criteria.

## Open questions

Only if something can't be planned without a person: the questions, each on its own line.
