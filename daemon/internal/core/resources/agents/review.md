---
name: Review
key: review
workspace: worktreeReadOnly
slots: 2
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: rose
icon: eye
outcomes:
  - name: no blocking findings
    when: Nothing must change before merging.
    tone: good
    then: next
  - name: blocking findings
    when: Something must change before merging.
    tone: problem
    then: goto
    to: build
    withOutput: true
    max: 1
---

Review the change on this branch as checked out: `git diff origin/{base}...HEAD`. Do not change files, push, or comment on anything.

Write {review}: your findings with severity and file and line references, or that you found none, and the checks you ran.
