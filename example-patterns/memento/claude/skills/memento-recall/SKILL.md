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

1. Query with a short phrase that describes the concept in whatever words come naturally — memento matches meaning, not just exact terms, so a paraphrase of what you're thinking about usually works. Domain terms still help when you know them, but you don't need to guess the brain's vocabulary.
2. Call `search` with that phrase
3. If results look relevant, call `get_page` to read the full page
4. Follow a `[[wikilink]]` only when it's load-bearing for your current task — a well-scoped search usually returns the right page directly, so don't reflexively hop
5. A zero-result search is a reliable "not in the brain" signal — `list_pages` is a last resort for when you need to browse the full topic list, not a fallback for missed searches

## How to use what you find

- Adopt the brain's terminology rather than inventing your own names for the same concepts
- Use it to enhance context surrounding the topic being discussed (the user might be asking about something in your second brain, but not the project itself)
- When multiple results cover the same concept with conflicting content, use `last_updated` (included in every search result) to identify the more recent capture — prefer it when there's tension between older and newer understanding

## Rules

- No results is a normal outcome — proceed if it has nothing relevant
- Don't report every search you run; only surface what's useful to the user
- Speed matters: a quick targeted search is better than skipping it; an exhaustive read of marginally related pages is worse than skipping it
