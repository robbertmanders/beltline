---
name: Open the feature branch
key: branches
workspace: worktree
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: grey
icon: branch
outcomes:
  - name: opened
    when: The feature branch exists on the remote of every repository.
    tone: good
    then: next
  - name: blocked
    when: The branch couldn't be pushed somewhere and a person should look.
    tone: you
    then: hold
---

The epic below is built on a feature branch, {branch}, in each of its repositories. The tickets filed for it branch from {branch} and their pull requests merge into it; once the whole epic is done, {branch} merges into {base}.

In each repository folder listed below, make sure {branch} exists on the remote:

1. If `git ls-remote --heads origin {branch}` already lists it, leave it as it is.
2. Otherwise push it as it is, without any commits: `git push -u origin HEAD:{branch}`.

Don't change any code and don't open a pull request.

Write {branches}: each repository and whether you pushed the branch or found it there.
