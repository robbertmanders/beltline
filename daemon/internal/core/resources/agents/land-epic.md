---
name: Land the epic
key: land
workspace: readOnly
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: coral
icon: ship
outcomes:
  - name: opened
    when: Each repository's feature branch has a pull request into its base.
    tone: good
    then: next
  - name: blocked
    when: A pull request couldn't be opened and a person should look.
    tone: you
    then: hold
---

Every wave of the epic below has landed on its feature branch, {branch}. Open the pull requests that merge it.

In each repository listed below:

1. Run `git fetch origin {branch} {base}`. If `origin/{branch}` has no commits beyond `origin/{base}`, nothing landed there; skip it and say so.
2. If a pull request from {branch} is already open, keep it. Otherwise open one into {base} with `gh pr create --base {base} --head {branch}`. Push nothing: the branch on the remote is the work. Give it a title from the epic and a body that sums up the design and lists the tickets that built it.

Write {land}: each repository and its pull request.
