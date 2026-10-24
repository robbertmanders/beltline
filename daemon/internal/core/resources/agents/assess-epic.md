---
name: Assess the epic
key: assess
workspace: readOnly
slots: 2
runner: pi
model: openrouter/stealth/space-bunny-alpha
tint: teal
icon: gauge
outcomes:
  - name: wait
    when: Nothing needs doing yet; the wave is still being built.
    tone: neutral
    then: goto
    to: watch
  - name: wave done
    when: Every ticket of the current wave is done, or the plan has to change now.
    tone: good
    then: goto
    to: replan-epic
    withOutput: true
  - name: epic done
    when: Every ticket is done and the epic asked for nothing more.
    tone: good
    then: goto
    to: land-epic
  - name: stuck
    when: A comment asks something only a person can decide.
    tone: you
    then: hold
    comment: true
---

Something changed on the epic below: a ticket under it was added or changed status, or someone commented on it. What changed is in the note. Decide what the epic needs now. Do not change any repository, and don't file or edit tickets yourself.

Read the plan at {plan}, the newest replan at {replan} if there is one, and the tickets under the epic (with `acli` or `gh`). Then:

- If every ticket of the current wave is done, the wave is done. So is it when what landed, or a comment, means the plan has to change now.
- If every ticket is done and the plan has no wave left to build, the epic is done.
- If a comment asks something only a person can decide, say what they must decide.
- Otherwise the wave is still being built: wait.

When someone commented, answer what they asked in your output.

Write {assess}: what changed, what you made of it, and what should happen next.
