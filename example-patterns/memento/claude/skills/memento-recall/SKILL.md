---
name: memento-recall
description: Consult the memento brain whenever you encounter a term or concept that may have prior context
user-invocable: false
---

The memento brain is your long-term memory across sessions. Treat it as the first place to look whenever you encounter a term, concept, or decision that might have prior context — not just once at the start of a task, but continuously as new concepts surface.

## When to search the brain

Search any time you:
- Encounter domain terminology you haven't seen defined in this conversation
- Are about to make a design or architectural decision — there may be a recorded rationale
- Notice a concept is referenced but not explained in something you just read
- See a `[[wikilink]]` in a retrieved page pointing to a concept you don't have context on
- Are about to create or name something new — search first to avoid duplicating existing work

## How to search

1. Pick the most specific term or phrase for the concept — domain terms over generic words
2. Call `search` with that term
3. If results look relevant, call `get_page` to read the full page
4. Follow `[[wikilinks]]` in the retrieved page when they point to concepts that are load-bearing for your current task — but stop after 1–2 hops; don't rabbit-hole

## How to use what you find

- Adopt the brain's terminology rather than inventing your own names for the same concepts
- Treat recorded decisions as settled unless the user is explicitly revisiting them
- If a retrieved page contradicts what you assumed, flag it rather than silently overriding either

## Rules

- No results is a normal outcome — proceed without the brain if it has nothing relevant
- Don't report every search you run; only surface what's useful to the user
- Speed matters: a quick targeted search is better than skipping it; an exhaustive read of marginally related pages is worse than skipping it
