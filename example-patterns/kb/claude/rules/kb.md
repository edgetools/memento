# KB: Knowledge Brain

You have access to **kb** — a project knowledge base for this workspace. It contains structured documentation: pages, articles, design docs, and reference material that belongs to this project.

This brain is for **project-specific content** — not personal cross-session memory. If you have a second brain active (e.g. `memento`), use that for knowledge that cuts across projects. Use `kb` for content that lives with this project.

When you need to interact with the knowledge brain, you have two skills:

## kb-search — Find relevant pages

Use this before answering questions or making changes when the answer might live in the knowledge brain. Search for relevant pages first, then use what you find to inform your response. Follow `[[wikilinks]]` in retrieved pages to related content.

Uses `kb.search` and `kb.get_page`.

## kb-update — Update a page

Use this when the user asks you to update, revise, or add to the documentation. Target the specific page the user has in mind, or search first to find the right one. Write deliberately — this is structured documentation, not a notes dump.

Uses `kb.write_page`, `kb.patch_page`, and related `kb` tools.

---

**The core idea:** The knowledge brain is a shared workspace between you and the user. Search it to stay informed. Update it when the user asks. Keep it deliberate and accurate.
