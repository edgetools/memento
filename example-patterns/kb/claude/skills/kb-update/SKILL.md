---
name: kb-update
description: Update a knowledge brain page when the user asks to change the docs
user-invocable: false
---

The user wants to update the knowledge brain. This is deliberate documentation work — write accurately and completely, targeting the specific page the user has in mind.

## Before writing

1. If the user named a specific page, call `get_page` to read the current content before changing it — note `last_updated` in the response. A recently-updated page may reflect deliberate decisions worth understanding before overwriting; a page last updated long ago is more likely safe to revise freely.
2. If the target page isn't clear, call `search` — it resolves a natural-language description of the page ("the page about how we handle flaky tests") to the actual page, so you don't need the user's exact title. Reserve `list_pages` for when you truly need to browse every topic.
3. Understand what currently exists before deciding how to change it

## How to update

**To revise existing content:**
- Use `patch_page` with `replace` for targeted changes to specific passages
- Use `patch_page` with `replace_lines` when rewriting a section you've already read by line range
- Use `write_page` only when fully rewriting a page — be aware it replaces all existing content

**To add new content to an existing page:**
- Use `patch_page` with `append` to add a section at the end
- Use `patch_page` with `prepend` to add content at the top

**To create a new page:**
- Use `write_page` with the new page name and full content
- Or use `patch_page` with `append` if you only have partial content now

**To rename a page:**
- Use `rename_page` — it atomically renames the file and updates all `[[wikilinks]]` across the brain that reference the old name
- Never use delete + create for renames; that breaks link graph integrity

## What to write

- Be accurate and complete — this is documentation, not a notes dump
- Use `[[wikilinks]]` to reference related pages by name
- Page names should be descriptive phrases, not terse slugs — `[[Combat Turn Order]]` not `[[turns]]`
- Don't pad content — a focused, accurate page is better than a long one

## Rules

- Always read the current page before replacing it
- Confirm with the user before deleting a page
- Report what changed so the user can verify the update was correct
