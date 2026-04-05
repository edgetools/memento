---
name: memento-snapshot
description: Jot something into the brain right now, mid-task, while the context is fresh
---

You just learned something worth keeping. Write it down now, before the moment passes.

This is a reflex, not a ritual. One concept, one page, done in seconds. Don't sweep the whole conversation — just capture the thing you're holding right now.

## When to use this

- You just worked out a decision or constraint that should survive beyond this session
- A concept was defined or clarified and you'd want it next time
- You realized something connects to prior work in a way that should be recorded
- The user explained something that isn't in the codebase or docs anywhere

## How to write

1. Name the concept — use a descriptive phrase (`[[Retry Backoff Strategy]]`, not `[[retry]]`)
2. Write what you know right now — one sentence to one paragraph is enough
3. Add `[[wikilinks]]` to related concepts you know exist (or should exist)
4. Call `patch_page` with `append` — if the page doesn't exist it will be created automatically

That's it. Return to the task.

## Rules

- Don't search first — `patch_page append` creates the page if it doesn't exist, so no read-before-write needed
- Don't wait until you have a "complete" picture of the concept — incomplete is fine
- Don't report it to the user unless they'd find it relevant; just do it and keep working
