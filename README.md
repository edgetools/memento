# Memento — Second Brain MCP Server

A persistent, evolving knowledge store that spans sessions. Record decisions, terminology, and reasoning in markdown pages linked with `[[wikilinks]]`. Your brain grows and stays organized through agent interaction.

**Works with any MCP client**, but examples shown are for Claude Code or Claude Desktop

---

## Quick Start

### 1. Install the server

```bash
go get github.com/edgetools/memento
```

### 2. Create a brain directory

The brain is any directory of markdown files — typically a git repo:

```bash
mkdir -p ~/my-brain
cd ~/my-brain
git init
```

### 3. Register with Client

#### Claude Example

Add memento to your Claude settings in `~/.claude/claude.json` (or via the Claude settings UI under **MCP Servers**):

```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento",
      "args": [
        "-content-dir", "/path/to/my-brain",
        "-auto-commit"
      ]
    }
  }
}
```

Replace `/path/to/memento` and `/path/to/my-brain` with your actual paths. 

**Optional flags:**
- `-auto-commit`: Automatically `git add` and `git commit` after every write. Omit if your brain directory is not a git repo.

### 4. Restart Client

Quit and relaunch your client to load the MCP server.

---

## Using Your Brain

### Introduce memento to your session

When you start working with your brain, paste the contents of `claude-skills/INTRO.md` into a session to introduce memento and its four example skills.

Alternatively, add this to your project's prompt file (e.g. `CLAUDE.md`) so agents automatically learn about memento when they start:

```markdown
## Memento: Second Brain

You have access to memento — a persistent knowledge store that spans sessions. 

When you need to interact with your brain, you have four skills:
- `/memento-recall` — Search the brain for prior context during a task
- `/memento-snapshot` — Jot down a concept mid-task
- `/memento-sleep` — Sweep the conversation at the end to capture durable knowledge
- `/memento-dream` — Maintain and organize the brain
```

### Use the example skills

To use them, copy them to your claude skills directory (e.g. `.claude/skills/`).

Four example skills are included in `claude-skills/`:

- `memento-recall/SKILL.md` — Search for prior context mid-task
- `memento-snapshot/SKILL.md` — Jot down a concept right now
- `memento-sleep/SKILL.md` — Capture a session's knowledge at the end
- `memento-dream/SKILL.md` — Maintain and organize the brain

Each is a complete prompt you can invoke from Claude with `/memento-recall`, `/memento-snapshot`, etc. They're templates — feel free to adapt them to your workflow.

---

## The Brain Content Model

### Pages and names

Every concept is a markdown page identified by a **page name** — a human-readable phrase like "Crowd Control" or "Retry Backoff Strategy". Page names are case-insensitive and whitespace-normalized.

### Wikilinks

Pages reference each other with `[[wikilinks]]`:

```markdown
# Crowd Control

[[Enchanter]] is the primary CC class, though [[Bard]] has limited CC.
```

Wikilinks create bidirectional relationships in the graph. They serve as both navigation and declarations of relevance.

### Flat, freeform structure

No hierarchy, no required templates. Pages are markdown files in a flat directory. Each page organizes its content however is most useful.

### Git as audit trail

The brain directory is a git repo. Every write operation commits automatically (with `-auto-commit`), so the full history is always recoverable.

---

## Tools Available in Claude

Your MCP provides these tools:

### `search`
Query the brain by keyword. Returns relevance-ranked results with contextual snippets and related pages.

```json
{
  "query": "enchanter crowd control",
  "max_results": 10
}
```

### `get_page`
Fetch a page by name, or specific line ranges.

```json
{
  "page": "Crowd Control",
  "lines": ["10-25", "45"]
}
```

### `write_page`
Create a new page or fully replace an existing one.

```json
{
  "page": "Enchanter",
  "content": "The enchanter is a utility class specializing in [[Mez]] and [[Haste]] spells."
}
```

### `patch_page`
Targeted edits: replace text, replace lines, append, or prepend.

```json
{
  "page": "Crowd Control",
  "operations": [
    {
      "op": "replace",
      "old": "Enchanter is the only CC class",
      "new": "[[Enchanter]] is the primary CC class"
    },
    {
      "op": "append",
      "content": "\n## Related Concepts\n\nSee [[Diminishing Returns]]."
    }
  ]
}
```

### `rename_page`
Rename a page and update all `[[wikilinks]]` across the brain atomically.

```json
{
  "page": "Crowd Control",
  "new_name": "Crowd Control Mechanics"
}
```

### `delete_page`
Remove a page from the brain.

```json
{
  "page": "Obsolete Concept"
}
```

### `list_pages`
Retrieve a sorted, paginated list of page names.

```json
{
  "sort_by": "least_linked",
  "limit": 50,
  "offset": 0,
  "filter": ["combat", "enchanter"]
}
```

---

## Design Philosophy

- **Readable over terse:** Page names are descriptive phrases, not slugs. `[[Aggro From Healing]]` is better than `[[aggro]]`.
- **Links over tags:** Every concept worth tagging is worth having a page for. Wikilinks create navigable structure.
- **Flat over hierarchical:** No premature categorization. Discovery comes from search and graph traversal.
- **Freeform over templates:** Pages organize themselves. A one-sentence page is fine; it can grow later.
- **Git as source of truth:** Markdown files are the canonical storage. The in-memory index is built from files, never the other way around.

---

## Learn More

- **DESIGN.md** — Complete specification of architecture, search algorithm, and design decisions
- **claude-skills/INTRO.md** — Introduction prompt for agents learning about their second brain
- **Example skills** — Real prompts for recall, snapshot, sleep, and dream workflows

---

## Feedback & Contributions

This is early-stage. If you find bugs, have ideas, or want to contribute, reach out or open an issue.
