---
name: memento-sleep
description: End-of-session sweep — capture everything worth keeping from this conversation into the right brain
disable-model-invocation: true
---

This runs at the end of a session. Your job is to capture durable knowledge from the conversation into the appropriate brain(s).

## Step 1: Route before you write

Ask two questions for each thing worth capturing:

**Is it cross-project?** — Would this be useful in a completely different project, or does it only make sense in this one?
- Cross-project → memento brain
- Project-specific → kb (if active), otherwise skip or memento as a fallback

**Is a kb brain active?** — Check whether kb tools are available. If yes, project-specific content (design decisions, architecture, feature notes, implementation rationale) belongs there, not in memento.

### Write to memento when:
- The concept spans projects or sessions — terminology, vocabulary, mental models
- It's about *how you work* — preferences, rules, patterns Claude established with this user
- It's a decision or constraint that would matter even if this codebase were replaced
- There's no kb active and the content is worth preserving

### Write to kb when a kb is active and content is:
- Design or architecture decisions specific to this project
- Feature behavior, data models, or implementation rationale
- Anything that belongs in project documentation

### Skip entirely:
- Ephemeral outcomes: test results, error messages seen and fixed, commands that were run
- Information already derivable from the code or git history
- Anything the user explicitly said not to remember

## Step 2: Write

**For memento** (`mcp__memento__patch_page`):
- Use `patch_page` with `append` for every page — creates the page if it doesn't exist, appends safely if it does
- Never use `write_page` blindly — it fully replaces any existing page, silently destroying accumulated content
- Page names should be descriptive phrases, not terse slugs — `[[Retry Backoff Strategy]]` not `[[retry]]`
- Add `[[wikilinks]]` to connect related concepts, even if the linked page doesn't exist yet
- A one-sentence page is fine — capture now, elaborate later

**For kb** (e.g. `mcp__kb__patch_page`):
- Same write mechanics, but kb is for accurate documentation — be more deliberate
- Read the existing page first if you're adding to something that likely already exists

## Rules

- Speed over perfection
- Err on the side of writing to memento — noise and duplicates are okay; memento-dream cleans that up
- Be more selective for kb — it's documentation, not a notes dump
- Don't block on ambiguity — make a routing call and move on
- Don't report every page written; just do it
