# memento — agent-powered knowledge stores in markdown

memento is an MCP server for building **agent-powered knowledge stores** from plain markdown files. Point it at any directory — a new folder, an existing Obsidian vault, a git repo — and agents can search, read, and write pages through a set of structured tools.

Because a memento brain is just markdown files with `[[wikilinks]]`, it is **natively Obsidian-compatible**: open the same directory in Obsidian and browse it visually, no configuration required. No Obsidian installation is needed to use memento — it operates directly on the filesystem.

**Works with any MCP client.** Examples shown use Claude Code or Claude Desktop.

---

## What is a memento brain?

A **memento brain** is a directory of markdown files that memento serves over MCP. Each page is a `.md` file identified by a human-readable name like "Crowd Control" or "Retry Backoff Strategy". Pages link to each other with `[[wikilinks]]`, forming a navigable knowledge graph.

Two common patterns:

### Second brain
A personal, cross-session memory store. Agents capture decisions, terminology, and reasoning during work and recall it in future sessions. The brain grows organically — agents jot notes mid-task, sweep the conversation at the end, and periodically reorganize. This is the classic "second brain" use case.

### Knowledge brain
A structured, project-scoped documentation workspace — like Claude Projects, but local, git-backed, and Obsidian-browsable. Agents read it for context and update specific pages when content changes. Good for game design docs, project wikis, architecture records, or any living documentation that agents collaborate on.

What makes them different is the **skills you wire up** and the **MCP server name you register** — not the server or the file format. The same memento binary serves both patterns.

---

## Quick Start

### 1. Install the server

```bash
go install github.com/edgetools/memento@latest
```

### 2. Create or point at a brain directory

**Starting a new second brain:**
```bash
mkdir -p ~/my-brain
cd ~/my-brain
git init
```

**Using an existing Obsidian vault or project directory:**
Just point memento at it — any directory of markdown files works as-is.

### 3. Register with your MCP client

The MCP server name you register becomes the tool prefix agents use. Name it to match your brain's purpose.

**Second brain** (register as `memento`):
```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento",
      "args": ["-content-dir", "/path/to/my-brain", "-auto-commit"]
    }
  }
}
```

**Knowledge brain** (register as `kb`, or a project-specific name):
```json
{
  "mcpServers": {
    "kb": {
      "command": "/path/to/memento",
      "args": ["-content-dir", "/path/to/my-project-docs"]
    }
  }
}
```

Multiple brains can be active simultaneously — each registered under a different name, each with its own skill set.

**Optional flags:**
- `-auto-commit`: Automatically `git add` and `git commit` after every write. Requires the content directory to be inside a git repo.

### 4. Restart your client

Quit and relaunch to load the MCP server.

---

## Wiring Up Skills

Skills are prompts that define what a brain is *for*. The right skill set turns a directory of markdown files into a second brain, a project knowledge base, or something else entirely.

Example patterns (skills + rules) are included in `example-patterns/`:

```
example-patterns/
├── memento/claude/
│   ├── skills/            ← second brain skills for Claude
│   │   ├── memento-recall/SKILL.md
│   │   ├── memento-snapshot/SKILL.md
│   │   ├── memento-sleep/SKILL.md
│   │   └── memento-dream/SKILL.md
│   └── rules/             ← agent instructions
│       └── memento.md
└── kb/claude/
    ├── skills/            ← knowledge brain skills for Claude
    │   ├── kb-search/SKILL.md
    │   └── kb-update/SKILL.md
    └── rules/             ← agent instructions
        └── kb.md
```

Copy the relevant skills to your Claude skills directory (e.g. `.claude/skills/`) and add the rules to your Claude rules directory (e.g. `.claude/rules/`). They're templates — adapt them to your workflow.

### Introducing the brain to an agent

The rules files teach agents about the brain automatically:

**Second brain** (`example-patterns/memento/claude/rules/memento.md`):
```markdown
## memento: Second Brain

You have access to memento — a persistent knowledge store that spans sessions.

- `/memento-recall` — Search the brain for prior context during a task
- `/memento-snapshot` — Jot down a concept mid-task
- `/memento-sleep` — Sweep the conversation at the end to capture durable knowledge
- `/memento-dream` — Maintain and organize the brain
```

**Knowledge brain** (`example-patterns/kb/claude/rules/kb.md`):
```markdown
## KB: Knowledge Brain

You have access to kb — a project knowledge base for this workspace.

- `/kb-search` — Search for relevant pages before answering or writing
- `/kb-update` — Update a page when the user asks to change the docs
```

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

Wikilinks create bidirectional relationships in the graph index. They serve as both navigation and search signal — a page linking to a concept is a deliberate declaration of relevance.

### Flat, freeform structure

No hierarchy, no required templates. Pages are markdown files in a flat directory. Each page organizes its content however is most useful. Discovery comes from search and graph traversal, not folder structure.

### Obsidian compatibility

A memento brain is a valid Obsidian vault. `[[wikilinks]]` resolve to files by name exactly as Obsidian expects. Open the content directory in Obsidian at any time to browse and edit visually — no setup required.

### Git as audit trail

Point memento at a git repo and use `-auto-commit` to record every write as a commit automatically. The full history of the brain is always recoverable.

---

## MCP Tools

### `search`
Query the brain by keyword. Returns relevance-ranked results with contextual snippets and related pages.

```json
{ "query": "enchanter crowd control", "max_results": 10 }
```

### `get_page`
Fetch a page by name, or specific line ranges.

```json
{ "page": "Crowd Control", "lines": ["10-25", "45"] }
```

### `write_page`
Create a new page or fully replace an existing one.

```json
{ "page": "Enchanter", "content": "The enchanter specializes in [[Mez]] and [[Haste]]." }
```

### `patch_page`
Targeted edits: replace text, replace lines, append, or prepend. `append` and `prepend` create the page if it doesn't exist.

```json
{
  "page": "Crowd Control",
  "operations": [
    { "op": "replace", "old": "Enchanter is the only CC class", "new": "[[Enchanter]] is the primary CC class" },
    { "op": "append", "content": "\n## Related\n\nSee [[Diminishing Returns]]." }
  ]
}
```

### `rename_page`
Rename a page and update all `[[wikilinks]]` across the brain atomically.

```json
{ "page": "Crowd Control", "new_name": "Crowd Control Mechanics" }
```

### `delete_page`
Remove a page from the brain.

```json
{ "page": "Obsolete Concept" }
```

### `list_pages`
Retrieve a sorted, paginated list of page names. Sort by `alphabetical`, `least_linked` (orphan discovery), or `most_linked` (hub concepts).

```json
{ "sort_by": "least_linked", "limit": 50, "offset": 0 }
```

---

## Design Philosophy

- **Links over tags:** Every concept worth tagging is worth having a page for. Wikilinks create navigable structure.
- **Flat over hierarchical:** No premature categorization. Discovery comes from search and graph traversal.
- **Freeform over templates:** Pages organize themselves. A one-sentence page is fine; it can grow later.
- **Readable names over terse slugs:** `[[Aggro From Healing]]` is better than `[[aggro]]`.
- **Git as source of truth:** Markdown files are canonical. The in-memory index is built from files, never the other way around.
- **Skills define purpose:** The server is neutral. What the brain is *for* is determined by the skills wired to it.

---

## Learn More

- **DESIGN.md** — Complete specification: architecture, search algorithm, design decisions
- **example-patterns/** — Example patterns (skills + rules) for second brain and knowledge brain patterns

---

## Feedback & Contributions

This is early-stage. If you find bugs, have ideas, or want to contribute, open an issue.