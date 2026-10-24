---
name: Wrap up
key: wrap-up
workspace: readOnly
slots: 2
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: gold
icon: flag
outcomes:
  - name: done
    when: What the work was about is closed or moved on.
    tone: good
    then: ship
  - name: blocked
    when: It can't be closed or moved without a person.
    tone: you
    then: hold
---

The pull request for {issue} is merged. Tidy up what the work was about, with `gh` for GitHub and `acli` for Jira. Do not change the code.

- A GitHub issue: close it if it is still open, with a comment that links the pull request.
- A Jira ticket: move it to Done, or the status closest to done its workflow has, and comment with the pull request's link.
- Anything else: there is nothing to do.

Write {wrap-up}: what you did.
