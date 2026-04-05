# memento: Your Second Brain

You have access to **memento** — a persistent knowledge store that spans sessions. It is your long-term memory for cross-cutting knowledge: decisions, terminology, concepts, and reasoning that should survive beyond any single conversation.

This brain is for **personal, cross-session memory** — not project documentation. If you have a separate knowledge brain active (e.g. `kb`), use that for project-specific content and this one for things that cut across projects and sessions.

When you need to interact with your second brain, you have four skills:

## memento-recall — Look things up

Use this at any point during a task when you encounter a concept, term, or decision that might have prior context. Don't wait until the start of a session — search continuously as new concepts surface. If you're about to make a design decision, about to name something new, or see a concept you haven't defined in this conversation, search the brain first. Results may point to recorded rationale, terminology, or related work that saves re-derivation.

Uses the `memento` MCP server tools (`memento.search`, `memento.get_page`).

## memento-snapshot — Jot something down, right now

Use this mid-task when you've just worked out a decision or concept that you want to survive to the next session. One page, one concept, done in seconds. Don't wait for end-of-session cleanup — capture it while the context is fresh. The page can be a single sentence; incomplete is fine.

Uses `memento.patch_page` with `append`.

## memento-sleep — Sweep the conversation at the end

Use this when a session is wrapping up to capture durable knowledge from the whole conversation. Go through what was discussed and write down decisions, constraints, terminology, relationships between concepts, and things that were tried and why. Noise and duplicates are fine; memento-dream cleans those up later.

Uses `memento.patch_page` with `append`.

## memento-dream — Maintain the brain

Use this for dedicated cleanup sessions. Find orphaned pages and link them, consolidate duplicates, split oversized pages, strengthen cross-links, and rename pages that no longer fit their scope. This is deliberate, slow maintenance work — don't mix it with active task work.

Uses all `memento` MCP server tools.

---

**The core idea:** Your second brain accumulates cross-session knowledge. Use recall to leverage past context, snapshot to capture fresh insights, sleep to preserve a session's knowledge, and dream to keep it organized and navigable.
