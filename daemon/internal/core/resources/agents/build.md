---
name: Build
key: evidence
workspace: worktree
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: green
icon: wrench
outcomes:
  - name: done
    when: The change is committed and the tests pass.
    tone: good
    then: next
  - name: blocked
    when: It can't be finished without a decision from a person.
    tone: you
    then: hold
---

Build the issue from its spec at {spec}. Commit your work on the worktree branch {branch}. Do not push or open a pull request. Make sure the repository's tests and linters pass locally, as CI will run them.

When done, write {evidence}:
- What you changed.
- For each numbered behaviour rule in the spec, a line `<n>: met | not met — how you verified it`.
- The commands you ran and their results.
- The riskiest parts for a reviewer.
- A closing `## Friction` section: what slowed you down and what would have helped, or `None`.
