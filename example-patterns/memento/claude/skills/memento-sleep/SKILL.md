---
name: memento-sleep
description: End-of-session sweep — capture everything worth keeping from this conversation into the brain
disable-model-invocation: true
---

This runs at the end of a session. Your job is to capture durable knowledge from the conversation into the memento brain.

## Durability test

Before writing a page, ask: *if the codebase were deleted, would this still be useful to know?* If no, skip it.

## What to skip

- Ephemeral outcomes: test results, error messages seen and fixed, commands that were run
- Information already derivable from the code or git history
- Anything the user explicitly said not to remember

## What to capture

Go through the conversation and write down anything that might be useful in a future session:
- Concepts, terms, or domain vocabulary that were introduced or clarified
- Decisions made and why
- Constraints, requirements, or rules that were established
- Relationships between concepts that were worked out
- Things that were tried and didn't work, and why

Err heavily on the side of writing. Noise and duplicates are okay — memento-dream exists to clean that up later. The cost of a redundant page is low. The cost of losing a decision is high.

## How to write

- Call `patch_page` with `append` for every page — it creates the page automatically if it doesn't exist, and appends safely if it does
- Never use `write_page` blindly — it fully replaces any existing page with the same name, silently destroying accumulated content
- Page names should be descriptive phrases, not terse slugs — `[[Aggro From Healing]]` not `[[aggro]]`
- Add `[[wikilinks]]` to connect concepts to each other, even if the linked page doesn't exist yet
- A one-sentence page is fine — capture the concept now, elaborate later

## Rules

- Speed over perfection
- Don't block on ambiguity — make a call and move on
- Don't report every page written; just do it
