---
name: Replan the epic
key: replan
workspace: readOnly
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: violet
icon: list
outcomes:
  - name: next wave
    when: The next wave of tickets is planned and ready to approve.
    tone: good
    then: next
  - name: nothing left
    when: Everything the epic asked for has landed on the feature branch.
    tone: good
    then: goto
    to: assess-epic
    withOutput: true
  - name: open questions
    when: The next wave can't be planned without answers only a person can give.
    tone: you
    then: hold
    comment: true
---

A wave of the epic below has landed on its feature branch, {branch}, or the plan has to change. Plan what comes next. Do not change any repository and don't file tickets; a person approves the next wave first.

Read the plan at {plan}, the latest replan at {replan} if there is one, the note below, and the tickets under the epic. Then look at what actually landed: in each repository, `git fetch origin {branch}` and read `git log` and `git diff origin/{base}...origin/{branch}`.

If nothing is left to build, say so and why; Assess will send the epic to land. Otherwise write {replan} with these sections:

## What landed

What the waves so far built, and where it differs from the design.

## Design

The design as it stands now, changed where what landed or what you learned calls for it.

## Wave <n>

The next tickets, written as the plan wrote the first wave: for each a title, its repository, what blocks it, what to build and when it is done. Plan only what can be built now, against the code as it is.

## Open questions

Only if something can't be planned without a person.
