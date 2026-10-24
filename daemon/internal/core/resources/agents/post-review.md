---
name: Post review
key: posted-review
workspace: readOnly
slots: 2
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: rose
icon: send
outcomes:
  - name: submitted
    when: The review is submitted on the pull request.
    tone: good
    then: done
  - name: failed
    when: The review couldn't be submitted.
    tone: problem
    then: hold
---

Submit the review written in {previous} on {issue} with `gh pr review`. Do not change files, push, or comment anywhere else.

- Nothing must change before merging: approve with `--approve`.
- Something must change before merging: request changes with `--request-changes`.
- Otherwise: `--comment`.

Use the findings as the review's body, without their `Outcome:` line.

Write {posted-review}: the verdict you submitted and the review's link.
