---
name: File the wave
key: wave
workspace: readOnly
slots: 1
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: sand
icon: doc
outcomes:
  - name: filed
    when: Every ticket of the approved wave is filed under the epic.
    tone: good
    then: next
  - name: blocked
    when: Some tickets couldn't be filed and a person should look.
    tone: you
    then: hold
---

File the approved wave of tickets for the epic below. Do not change any repository.

The wave to file is the newest of these that exists: the replan at {replan}, or else the plan at {plan}, under its "Wave" section. A note below may say what the person who approved it wants changed; follow it.

The tickets are built on the epic's feature branch, {branch}. For each ticket of the wave:

1. Skip it if a ticket with the same title is already under the epic.
2. For a Jira epic, create it with `acli jira workitem create` in the epic's project, with the epic as its parent. For a GitHub issue, create it with `gh issue create` in its repository and add it as a sub-issue of the epic (`gh api repos/<owner>/<repo>/issues/<epic>/sub_issues -f sub_issue_id=<id>`).
3. Write its description from the plan: what to build and when it is done. Start it with "Part of <the epic>. The design is in the epic's description."
4. Give it the labels that start it on the belt that builds tickets (ask nothing; use the labels the epic's existing tickets have, or `agent` when there are none), plus `base:{branch}` so its work starts from and merges into the feature branch. For a Jira ticket, say in its description which repository the work belongs in.
5. Link what blocks it, once every ticket of the wave exists. In Jira, for each ticket it waits for, run `acli jira workitem link create --out <this ticket> --in <the ticket it waits for> --type Blocks --yes`. The confirmation line names the two tickets the other way round ("<this ticket> Blocks <the blocker>"); that is expected, so don't act on it. On GitHub, add a "blocked by" dependency. Then check every ticket of the wave, including those that wait for nothing, with a command that prints only its blockers: in Jira `acli jira workitem view <ticket> --fields issuelinks --json | jq -r '.fields.issuelinks[] | select(.type.name == "Blocks" and .inwardIssue) | .inwardIssue.key'`, on GitHub `gh api repos/<owner>/<repo>/issues/<n>/dependencies/blocked_by --jq '.[].number'`. The keys it prints must be exactly the tickets the plan says it waits for. Go by that output alone, not by the link JSON or the confirmation lines. When they differ, delete the wrong link (in Jira `acli jira workitem link delete --id <id> --yes`) and create it again with the command above, then check again. A ticket that should wait but doesn't, or one blocked by the tickets that need it, starts work out of order.

The first time you file a wave, also replace the epic's description with the plan's "Design" section, so every ticket can read it, and keep what the epic said before under a "## The request" heading.

Write {wave}: each ticket you filed, with its key or number and what blocks it.
