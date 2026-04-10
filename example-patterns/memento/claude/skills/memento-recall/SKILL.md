---
name: memento-recall
description: Consult the memento brain whenever you encounter a term or concept that may have prior context
user-invocable: false
---

The memento brain is your long-term memory across sessions. Treat it as the first place to look whenever you encounter a term, concept, or decision that might have prior context — not just once at the start of a task, but continuously as new concepts surface.

## When to recall your memories from the brain

Recall your memories any time you:
- Encounter domain terminology you haven't seen defined in this conversation
- Are about to make a design or architectural decision — there may be a recorded rationale
- Notice a concept is referenced but not explained in something you just read
- See a `[[wikilink]]` in a retrieved page pointing to a concept you don't have context on
- Are about to create or name something new — recall your memories first to avoid duplicating existing work

## How to recall memories

1. Pick the most specific term or phrase for the concept — domain terms over generic words
2. Call `search` with that term
3. If results look relevant, call `get_page` to read the full page
4. Follow `[[wikilinks]]` in the retrieved page when they point to concepts that are load-bearing for your current task — but stop after 1–2 hops; don't rabbit-hole
5. If you cannot find anything, consider using `list_pages` which can deliver a paginated list of every topic in the brain -- use this as a last resort.

## How to use what you find

- Adopt the brain's terminology rather than inventing your own names for the same concepts
- Use it to enhance context surrounding the topic being discussed (the user might be asking about something in your second brain, but not the project itself)
- When multiple results cover the same concept with conflicting content, use `last_updated` (included in every search result) to identify the more recent capture — prefer it when there's tension between older and newer understanding

## Rules

- No results is a normal outcome — proceed if it has nothing relevant
- Don't report every search you run; only surface what's useful to the user
- Speed matters: a quick targeted search is better than skipping it; an exhaustive read of marginally related pages is worse than skipping it
