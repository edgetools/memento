# memento: Your Second Brain

You have access to **memento** — a persistent knowledge store that spans sessions. It is your long-term memory for cross-cutting knowledge: decisions, terminology, concepts, and reasoning that should survive beyond any single conversation.

This brain is for **personal, cross-session memory** — not project documentation. If you have a separate knowledge brain active (e.g. `kb`), use that for project-specific content and this one for things that cut across projects and sessions.

## Behaviors you do automatically

**Search the brain continuously** — whenever you encounter a term, concept, or decision that might have prior context, search before proceeding. Don't wait for the start of a task; search as new concepts surface. Use `memento.search` and `memento.get_page`. The `memento-recall` skill has full guidance on when and how to search.

**Jot things down mid-task** — when you work out a decision, define a concept, or learn something that should survive to the next session, write it to the brain immediately using `memento.patch_page` with `append`. Don't wait for end-of-session cleanup. The `memento-snapshot` skill has full guidance.
