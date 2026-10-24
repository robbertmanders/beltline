---
name: Custom agent
key: custom
workspace: worktree
slots: 2
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: teal
icon: chip
outcomes:
  - name: done
    when: It finished what the brief asks.
    tone: good
    then: next
  - name: failed
    when: It couldn't finish.
    tone: problem
    then: hold
---

Describe what this agent should do, what it may change, and what it writes to {output} when done.
