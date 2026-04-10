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

1. Pick the most specific term or phrase for what you're looking for
2. Call `search` with that term
3. If results look relevant, call `get_page` to read the full page
4. Follow `[[wikilinks]]` in retrieved pages to related content — but stop after 1–2 hops
5. If you cannot find anything, consider using `list_pages` which can deliver a paginated list of every topic in the brain -- use this as a last resort.

## How to use what you find

- Prefer the brain's existing terminology over inventing new names for the same concepts
- Treat documented decisions as settled unless the user is explicitly revisiting them
- If a retrieved page contradicts your assumptions, surface the conflict rather than silently overriding either
- When multiple results cover the same topic with conflicting content, use `last_updated` (included in every search result) to identify the more recently written page — it's likely the more authoritative source

## Rules

- No results is a normal outcome — proceed without the brain if it has nothing relevant
- Don't report every search you run; only surface findings that are useful to the user
- A quick targeted search is better than skipping it; an exhaustive read of marginally related pages is not
