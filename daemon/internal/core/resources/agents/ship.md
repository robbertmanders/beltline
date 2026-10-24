---
name: Ship
key: pull-request
workspace: worktree
slots: 2
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: coral
icon: ship
outcomes:
  - name: opened
    when: Every repository the branch changed has its pull request open.
    tone: good
    then: next
  - name: nothing to ship
    when: The branch has no commits to merge in any repository.
    tone: you
    then: hold
  - name: failed
    when: A branch couldn't be pushed or a pull request couldn't be opened.
    tone: problem
    then: hold
---

Open a pull request for the work committed on the worktree branch {branch}, in each repository where it has commits to merge. Do not change the code or make new commits.

When the run has several repositories, they are listed under Repositories below, each with its folder and the branch it merges into; with one, the working folder merges into {base}. For each repository:

1. Check what {branch} has that its base doesn't: `git fetch origin` and `git log --oneline origin/<base>..{branch}`. Leave a repository with no commits alone: it needs no pull request.
2. Push {branch} to origin.
3. If a pull request from {branch} is already open, keep it and don't open another. Otherwise open one into its base with `gh pr create --base <base> --head {branch}`.
   - Title: for a Jira ticket, its key, a colon, and its title; otherwise the title of what this run works on.
   - Body: first `Closes #<number>` for a GitHub issue, or a link to the Jira ticket. Then a short summary of the change in that repository, from the spec at {spec} and the evidence at {evidence} where they exist. End with the line "Opened by Beltline."

If no repository has commits, open nothing. Say so, and say where the work went if the spec or evidence shows it was done somewhere else.

Write {pull-request}: each pull request's repository, number and link, and the repositories you left alone.
