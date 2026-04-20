---
name: kb-explore
description: Explore the knowledge brain for relevant pages before answering or writing
user-invocable: false
---

The knowledge brain contains project documentation — pages, design docs, reference material, and anything else the project has recorded. Explore it before making assumptions about how things work or what has already been decided.

## When to explore

Explore any time you:
- Are about to answer a question that might be documented
- Are about to make a change and want to know if there's relevant context or prior decisions
- See a `[[wikilink]]` in a retrieved page pointing to something you don't have context on
- Are about to create or name something new — search first to avoid conflicting with existing content

## How to explore

1. Query with a short phrase that describes what you're looking for, in whatever words come naturally — search matches meaning, not just exact terms, so you don't need to guess the brain's vocabulary
2. Call `search` with that phrase
3. If results look relevant, call `get_page` to read the full page
4. Follow a `[[wikilink]]` only when the linked concept is load-bearing for what you're doing — a well-scoped search usually returns the right page directly, so don't reflexively hop
5. A zero-result search is a reliable "not documented" signal — `list_pages` is a last resort for when you need to browse the full topic list, not a fallback for missed searches

## How to use what you find

- Prefer the brain's existing terminology over inventing new names for the same concepts
- Treat documented decisions as settled unless the user is explicitly revisiting them
- If a retrieved page contradicts your assumptions, surface the conflict rather than silently overriding either
- When multiple results cover the same topic with conflicting content, use `last_updated` (included in every search result) to identify the more recently written page — it's likely the more authoritative source

## Rules

- No results is a normal outcome — proceed without the brain if it has nothing relevant
- Don't report every search you run; only surface findings that are useful to the user
- A quick targeted search is better than skipping it; an exhaustive read of marginally related pages is not
