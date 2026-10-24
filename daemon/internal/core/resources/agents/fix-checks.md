---
name: Fix checks
key: checks
workspace: worktree
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: teal
icon: wrench
outcomes:
  - name: fixed
    when: The failing checks pass locally and the fix is pushed.
    tone: good
    then: goto
    to: watch
  - name: not ours
    when: The failures come from outside the change, such as a flaky test or broken CI, and a person should look.
    tone: you
    then: hold
  - name: blocked
    when: It can't be fixed without a decision from a person.
    tone: you
    then: hold
---

The checks on the pull request from {branch}, listed below, are failing. Make them pass.

1. See which checks fail with `gh pr checks <number>`, and read their logs with `gh run view <run-id> --log-failed`.
2. Reproduce each failure locally when you can. Fix its cause in the code or the tests, keeping to what the change was for (the spec at {spec}, where it exists). Don't skip, disable or loosen a check or a test to make it pass.
3. If a failure has nothing to do with this change, such as a flaky test, a broken runner or a failure {base} has too, leave it and say so.
4. Make sure the repository's tests and linters pass locally. Commit the fix on {branch} and push it to origin. Never force-push.

Write {checks}:
- Each failing check, why it failed, and what you changed.
- The commands you ran and their results.
