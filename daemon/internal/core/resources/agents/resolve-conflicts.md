---
name: Resolve conflicts
key: conflicts
workspace: worktree
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: sand
icon: merge
outcomes:
  - name: resolved
    when: The base branch is merged in, the tests pass, and the branch is pushed.
    tone: good
    then: goto
    to: watch
  - name: blocked
    when: A conflict can't be resolved without a decision from a person.
    tone: you
    then: hold
---

The pull request from {branch}, listed below, has merge conflicts with {base} or has fallen behind it. Bring {branch} up to date with {base}, keeping what both sides meant to do.

1. `git fetch origin`, then `git merge origin/{base}` on {branch}. Merge, don't rebase: the branch is already pushed and reviewers may have read it.
2. Resolve each conflict from what the change was for (the spec at {spec}, where it exists) and what {base} changed since. Keep both sides' intent; don't simply take one side of a file. When the two can't both hold, stop and say so.
3. Make sure the repository's tests and linters pass locally, as CI will run them. Commit the merge.
4. Push {branch} to origin. Never force-push.

Write {conflicts}:
- Each conflicted file and how you resolved it.
- The commands you ran and their results.
- Anything a reviewer should look at again.
