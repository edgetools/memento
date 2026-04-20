# memento: agent-powered knowledge store mcp server

## Overview

A persistent, evolving knowledge store that agents can read from and write to
across sessions. Agents search it to build context, update it to capture decisions
and documentation, and reorganize it over time through interaction.

**Repo:** `github.com/edgetools/memento` (the MCP server code)

A **memento brain** is any directory of markdown files the server points at — a
git repo, an existing Obsidian vault, or a new directory. The server is generic;
the content and purpose are yours to define. Two common patterns:

- **Second brain**: a personal, cross-session memory store. Agents capture
  decisions, terminology, and reasoning during work and recall it in future
  sessions. The brain grows organically through agent interaction.
- **Knowledge brain**: a structured, project-scoped documentation workspace.
  Agents read it for context and update it when content changes. Think of it
  like Claude Projects — but local, git-backed, and Obsidian-browsable.

What distinguishes them isn't the server or the file format — it's the skills
you wire up and the MCP server name you register. The same memento binary serves
both patterns.

---

## Architecture

Go MCP server using `mark3labs/mcp-go`, stdio transport. Single binary, no runtime
dependencies. Takes a `-content-dir` flag pointing at a directory of markdown files.

On startup, parses all `.md` files in `-content-dir` to build an in-memory index:

- **BM25 inverted index** with weighted field scoring (title, wikilinks, body)
  and Porter stemming for relevance-ranked keyword search
- **Vector index** of 384-dimensional sentence embeddings, chunked per page,
  for semantic similarity search
- **Bidirectional link graph** parsed from `[[wikilinks]]` in page content

Index is rebuilt on startup and updated in-place on writes. Embedding vectors
are persisted to a `.memento-vectors` sidecar file in the content directory
(keyed by content hash + model identity), so only changed pages are re-embedded
on subsequent startups. The sidecar is derived data — safe to delete, rebuilds
on next startup, belongs in `.gitignore`.

A filesystem watcher (`fsnotify`) runs alongside the MCP server: when markdown
files in the content directory change from outside memento (editor, `git pull`,
another memento instance), the affected pages are re-parsed and the index is
updated in-place, with no restart required.

---

## Content Model

### Pages and Names

Every page is a markdown file in a flat directory (no hierarchy). Pages are
identified by their **page name**, which is a human-readable string like
"Crowd Control" or "Enchanter Mez Strategy". The page name is the single
identifier used everywhere: in tool calls, in `[[wikilinks]]`, and as the
concept's identity in the brain.

**Filename is the page name.** The filename on disk is `{page name}.md` —
the page name is used directly, preserving its original casing and spaces.
For example, a page named "Crowd Control" lives at `Crowd Control.md`. This
means `[[Crowd Control]]` links resolve to the correct file in both memento
and Obsidian without any mapping layer. Page names must not contain
characters that are forbidden in filenames across operating systems:

- **Forbidden characters:** `* " [ ] # ^ | < > : ? / \`
- **Forbidden patterns:** filenames must not start with `.`

These restrictions are the union of forbidden filename characters across
Windows, macOS, Linux, iOS, and Android (matching Obsidian's own
restrictions). The MCP rejects page names containing forbidden characters
with a clear error rather than silently stripping them.

**Name resolution:** Page name lookups are case-insensitive with whitespace
normalization (collapsing multiple spaces, trimming). So `[[Crowd Control]]`,
`[[crowd control]]`, and `[[Crowd  Control]]` all resolve to the same page.
The canonical casing is whatever was used when the page was first created.
On case-insensitive filesystems (Windows, macOS), the OS naturally handles
this. On case-sensitive filesystems (Linux), the MCP performs
case-insensitive lookup against its in-memory page index.

### Wikilinks as the Only Taxonomy

Pages reference each other with `[[wikilinks]]` using page names. There is no
separate tagging system. Every concept worth tagging is worth having a page for,
even if that page starts as a single sentence. Links create bidirectional
relationships in the graph index: if a page contains `[[Enchanter]]`, then a
search related to "Enchanter" can surface that page.

`[[wikilinks]]` serve double duty:

1. **Graph structure** for search ranking and discovery
2. **Reading instructions** for agents — a `[[link]]` in content signals "you
   might need to understand this concept to understand what you're reading"

Links to pages that don't exist yet are valid. They represent aspirational
connections that might be filled in later. Broken links (to deleted pages) are
also tolerated. Cleanup is a separate activity, not a gate.

Because page names are descriptive phrases (not terse slugs), agents naturally
produce self-documenting links like `[[Aggro From Healing]]` rather than
ambiguous ones like `[[aggro]]`. This reduces disambiguation problems at the
source.

### No Required Structure

Pages are freeform. No enforced templates, sections, or frontmatter. The agent
decides how to organize content on a page based on what's useful. The brain
evolves its own organizational patterns over time.

### Git as the Audit Trail

The brain content directory is expected to live in a git repo. Git history serves
as the audit trail and time machine. Pages can be freely rewritten because
previous versions are always recoverable. With `-auto-commit`, the MCP commits
after every write operation automatically. Without it, committing is the caller's
responsibility.

---

## Search and Indexing

The search system runs two ranking layers in parallel — BM25 keyword scoring
and semantic vector similarity — and merges their results before applying link
graph boosting and a relevance threshold.

### Search Pipeline

```
Query → [BM25 (stemmed, weighted fields)]  ┐
      → [Vector cosine search over chunks] ┘ → Merge & Normalize
      → Graph Boost
      → Relevance Filter
      → Snippet Generation → Return
```

Vector search is the semantic gate: only pages that clear the vector similarity
floor enter the merged result set. BM25 provides a score boost for pages that
also match keywords. This prevents a coincidental keyword hit in a semantically
unrelated page from surviving as a top result. When vector returns nothing,
BM25 scores pass through so keyword-only queries still work.

### BM25 (Best Match 25)

BM25 is the primary ranking algorithm. It improves on TF-IDF by adding term
frequency saturation (diminishing returns after a term appears many times) and
document length normalization (longer pages don't unfairly dominate).

**Scoring formula (per query term per page):**
```
score = IDF(term) * (TF * (k1 + 1)) / (TF + k1 * (1 - b + b * |page| / avgPageLen))
```

- `k1 = 1.5` (term frequency saturation, hardcoded default)
- `b = 0.75` (length normalization, hardcoded default)
- `IDF = log((N - DF + 0.5) / (DF + 0.5) + 1)` where N is page count, DF is
  how many pages contain the term

**Weighted field scoring:** Not all text on a page carries equal signal. The BM25
index scores three fields separately with different weights:

- **Page title** (the `# heading`): **10x weight**. A search term matching the
  page's own name is the strongest possible signal.
- **Wikilink targets** (`[[link text]]`): **3x weight**. A page linking to a
  concept is a deliberate declaration of relevance.
- **Body text**: **1x weight** (baseline).

The final BM25 score for a page is the weighted sum across all three fields.

**Build process (on startup):**

1. Parse every page into three fields: title, wikilink targets, body text
2. Tokenize each field: split on whitespace and punctuation, lowercase, strip
   markdown syntax
3. Apply Porter stemming to normalize morphological variants
4. Remove stop words
5. Build the inverted index with per-field term frequencies and document
   frequencies

**Wikilink-aware tokenization:** `[[Crowd Control]]` is indexed as both
individual terms ("crowd", "control") and as a compound term ("crowd control").
Searching for "crowd control" as a phrase ranks higher than pages that mention
crowds and control separately.

### Vector Search

Semantic search runs in parallel with BM25. Pages are split into chunks at
parse time and each chunk is embedded into a 384-dimensional vector using the
`all-MiniLM-L6-v2` sentence-transformer model (loaded in-process via
[`go-sentex`](https://github.com/edgetools/go-sentex), pure Go, no CGo). The
query is embedded into the same space and compared to every chunk vector by
cosine similarity. Chunks above a similarity floor (0.3) are returned; the
best chunk's page name and start line bubble up as the match.

**Chunking strategy:**

1. Primary split on markdown section headings (`##`, `###`, etc.)
2. Fallback split on double-newline paragraph breaks for pages without
   headings (e.g. accumulated snapshot pages)
3. Each chunk is prefixed with the page's `# Title` line so the embedding
   carries page identity
4. Chunks smaller than ~50 tokens are merged with an adjacent chunk
5. Maximum size is bounded by the model's 256-token context window

**Storage and scale:** Vectors are kept in memory as a flat array and searched
by brute-force cosine. At the expected scale (hundreds of pages, thousands of
chunks), this is sub-millisecond. No approximate-nearest-neighbor index is
needed.

**Embedding cache:** Embeddings are persisted to a `.memento-vectors` sidecar
file in the content directory, keyed by a content hash per page and stamped
with the model identity (`all-MiniLM-L6-v2` + `go-sentex` module version).
On startup, pages whose content hash matches the cache are re-indexed from
the cache; changed or new pages are re-embedded. The cache auto-invalidates
when the embedding model identity changes. It is derived data — deleting it
is harmless, it rebuilds on next startup, and it belongs in `.gitignore`.

**Model acquisition:** `go-sentex` loads the ONNX model from the standard
HuggingFace Hub cache (honors `HF_HOME`). On first run with no cached model,
it downloads ~87 MB once; subsequent runs are offline. A failed model load
is fatal — the server exits rather than silently falling back to keyword-only
search.

### Link Graph Boost

After BM25 produces a ranked list of direct matches, the graph boost step adds
connected pages and adjusts scores.

**Algorithm:**

1. Start with BM25 ranked results (direct matches)
2. For each direct match, collect all pages it links to and all pages that
   link to it (one hop)
3. Score each linked page with a dampened weight (linked pages are relevant
   but less so than direct matches)
4. If a linked page also appears as a direct match, boost its score (it
   matched the query AND is connected to other matches)
5. Deduplicate and re-rank the combined results

Boost weights are hardcoded defaults, tuned from real usage over time. Inbound
and outbound links are weighted equally initially.

### Relevance Threshold

Results scoring below 50% of the top result's score are dropped. This prevents
low-quality noise from cluttering results as the brain grows. The threshold is a
hardcoded default.

### Snippet Generation

Each search result includes a contextual snippet (~250 characters) showing why
it matched.

**For direct matches:** Find the position in the page with the highest density
of query terms. Extract surrounding context (roughly one sentence on each side).
Respect markdown structure: don't break in the middle of a code block or
paragraph.

**For linked-page results** (surfaced through the graph, not direct term
matches): The snippet comes from the *referring* page, showing the `[[link]]`
in its surrounding context. A search for "enchanter" might surface the "Crowd
Control" page with a snippet like "...typically the [[Enchanter]] is assigned
mez duty..." — showing how the term is used in context, not just that the page
exists.

**For title matches** (the search term matches a page's own name): The first
paragraph of that page is used as the snippet, since the whole page is "about"
the query term.

---

## Timestamps

Every page carries a `last_updated` field — an ISO 8601 UTC timestamp (ending
in `Z`) reflecting the last time that page's content was modified. It is
included in the output of `get_page`, `search`, and `list_pages` (for
`newest`/`oldest` sorts).

### What counts as a content-modifying write

- `write_page`
- `patch_page` (any operation)
- `rename_page` (rewrites the page heading)

### Timestamp source

The MCP derives `last_updated` from external sources in priority order:

1. **Git commit time** (when the content directory is inside a git repo): the
   commit timestamp of the most recent commit that touched the page's file
   (`git log -1` for that file). This is the preferred source — commit
   timestamps survive `git clone` and `git pull`, making them reliable across
   machines.

2. **Filesystem mtime** (fallback): used when the content directory is not a
   git repo, or when a file exists in the directory but has never been
   committed (e.g. a newly created page not yet staged).

3. **Omitted**: if neither source is available, `last_updated` is omitted from
   the response rather than returning a zero value or fabricated date.

**Known limitation:** When `-auto-commit` is not enabled and the user edits a
page outside the MCP (e.g. in Obsidian) without committing, the git-derived
timestamp will reflect the last committed state rather than the actual last
edit. The last committed state is the most recent *settled* content, which is
the intended behavior — uncommitted changes are in-flight.

---

## Filesystem Watching

memento runs an `fsnotify` watcher over the content directory for the lifetime
of the server. On create/modify/delete events for `.md` files, the affected
page is re-parsed and fed into `idx.Add` or `idx.Remove`, which updates the
BM25, graph, and vector indexes together and writes the new embeddings through
to the `.memento-vectors` cache.

Rapid event bursts are debounced per-file (100 ms window) so that an editor
writing via a temp-file-then-rename sequence produces a single reindex. The
watcher does not distinguish self-triggered events (from memento's own writes)
from external ones — the re-parse is cheap and the simplicity is worth the
negligible cost.

The watcher is always on and has no opt-in flag. If it fails to initialize
(for example, when the OS `inotify` watch limit is exhausted), the server
logs a warning and continues serving without live reload; the index is still
updated on every write through the MCP tools.

---

## Tools

### `search`

Queries the brain and returns relevance-ranked results with contextual snippets
and graph-connected pages.

**Input:**
```json
{
  "query": "enchanter crowd control",
  "max_results": 10,
  "max_tokens": 3000
}
```

`max_results` is optional, defaults to 10. `max_tokens` is optional; when
provided, the MCP returns ranked results up to the token budget, which may be
fewer than `max_results`. If both are provided, whichever limit is hit first
wins. Token counting is approximate (whitespace-split word count as a proxy).

**Output:**
```json
{
  "results": [
    {
      "page": "Crowd Control",
      "relevance": 0.87,
      "last_updated": "2024-11-03T09:15:42Z",
      "snippet": "...typically the [[Enchanter]] is assigned mez duty during pulls...",
      "line": 14,
      "linked_pages": ["Enchanter", "Pulling"]
    }
  ],
  "linked_page_details": [
    {
      "page": "Enchanter",
      "last_updated": "2024-08-21T14:07:30Z",
      "snippet": "The enchanter is a utility class specializing in mesmerize...",
      "line": 3
    },
    {
      "page": "Pulling",
      "last_updated": "2023-05-12T11:44:00Z",
      "snippet": "Pull strategy depends on available CC — [[Crowd Control]]...",
      "line": 8
    }
  ]
}
```

`linked_pages` on each result is a name-only array of outbound links from that
page. `linked_page_details` is a top-level, deduplicated list containing the
snippet and line number for each linked page that is **not** already present in
`results`. Pages that appear in `results` are excluded from `linked_page_details`
because they already have their own snippet. Each linked page appears at most
once in `linked_page_details`, regardless of how many results reference it.

`last_updated` is an ISO 8601 UTC timestamp (ending in `Z`) reflecting the last
time each page's content was modified. See the Timestamps section below for how
it is derived.

---

### `get_page`

Returns the full markdown content of a page by name, or specific line ranges.

**Input (full page):**
```json
{
  "page": "Crowd Control"
}
```

**Input (multiple line ranges, 1-indexed, inclusive):**
```json
{
  "page": "Crowd Control",
  "lines": ["10-25", "34", "52-68", "95-110"]
}
```

Single lines and ranges can be mixed freely. A single range also works:
`"lines": ["10-25"]`

Page name lookup is case-insensitive with whitespace normalization.

**Output (full page, no `lines` parameter):**
```json
{
  "page": "Crowd Control",
  "content": "# Crowd Control\n\nCrowd control (CC) refers to abilities that limit enemy actions...\n\n[[Enchanter]] is the primary CC class...",
  "total_lines": 142,
  "last_updated": "2024-11-03T09:15:42Z",
  "links_to": ["Enchanter", "Mez", "Pulling", "Root"],
  "linked_from": ["Party Composition", "Dungeon Strategy"]
}
```

**Output (line ranges):**
```json
{
  "page": "Crowd Control",
  "sections": [
    { "lines": "10-25", "content": "The primary CC classes are [[Enchanter]] and [[Bard]]..." },
    { "lines": "52-68", "content": "Mez duration is affected by level difference..." },
    { "lines": "95-110", "content": "Root spells share diminishing returns with..." }
  ],
  "total_lines": 142,
  "last_updated": "2024-11-03T09:15:42Z",
  "links_to": ["Enchanter", "Mez", "Pulling", "Root"],
  "linked_from": ["Party Composition", "Dungeon Strategy"]
}
```

When `lines` is omitted, `content` is the full page as a flat string. When `lines`
is provided, `sections` contains each requested range as a separate entry. The
typical workflow is: `search` returns hits with line numbers, then `get_page` with
ranges fetches context around multiple matches in a single call. `total_lines` is
always included so the agent can request additional ranges if needed. `links_to`
and `linked_from` always reflect the full page regardless of requested ranges.

---

### `write_page`

Creates a new page or fully replaces an existing page's content.

**Input:**
```json
{
  "page": "Enchanter",
  "content": "The enchanter is a utility class specializing in [[Mez]] and [[Haste]] spells.\n\nIn the party hierarchy, the enchanter's primary role is [[Crowd Control]]..."
}
```

`page` is the page name. This is the same name used in `[[wikilinks]]` and in
all other tools. If a page with the same name already exists (case-insensitive
match), its content is fully replaced. If it doesn't exist, a new file is
created. The in-memory index is updated after the write.

The agent provides the page name and the content body. The MCP handles file
format details (heading, filename) internally. The agent does not need to
include a `# heading` in the content; if one is present, the MCP replaces it
with the page name to ensure consistency.

To change a page's name, use `rename_page`.

**Output:**
```json
{
  "page": "Enchanter",
  "links_to": ["Mez", "Haste", "Crowd Control"],
  "commit_failures": ["git commit failed: exit status 128: ..."]
}
```

When `-auto-commit` is enabled and commit operations fail for any reason, the failed commit attempts will be listed in `commit_failures`.

---

### `patch_page`

Performs a targeted edit on an existing page without rewriting the entire content.
Supports multiple operations in a single call. Text-based replacement follows the
same `str_replace` pattern used by Claude's built-in file editing tools. Line-based
replacement targets specific line ranges identified from prior `search` or
`get_page` calls.

**Input:**
```json
{
  "page": "Crowd Control",
  "operations": [
    {
      "op": "replace",
      "old": "Enchanter is the only CC class",
      "new": "[[Enchanter]] is the primary CC class, though [[Bard]] has limited CC"
    },
    {
      "op": "replace_lines",
      "lines": "45-52",
      "new": "Root spells now share diminishing returns with all other CC types.\nSee [[Diminishing Returns]] for the full table."
    },
    {
      "op": "append",
      "content": "\n\n## Open Questions\n\nShould root break on damage? See [[Root Mechanics]]."
    }
  ]
}
```

**Supported operations:**

- **`replace`**: find `old` text (must match exactly and uniquely), replace with
  `new` text. Fails if `old` is not found or appears more than once. Requires
  the page to already exist.
- **`replace_lines`**: replace the content at `lines` (a range like `"45-52"` or
  a single line like `"45"`) with `new` text. The replacement content can be
  shorter or longer than the original range. All line numbers in a single call
  refer to the page as it exists *before* any operations are applied; the MCP
  computes the necessary offsets internally. Requires the page to already exist.
- **`append`**: add `content` to the end of the page. **Creates the page if it
  doesn't exist** (the MCP generates the page heading automatically).
- **`prepend`**: add `content` to the beginning of the page body. **Creates the
  page if it doesn't exist.**

This create-on-write behavior for `append` and `prepend` is key to the "jot it
down and move on" workflow. The agent doesn't need to search for or check whether
a page exists before appending a note. If the page is there, the note is added.
If it's not, the page is created with just the note. Cleanup and consolidation
happen in separate maintenance sessions.

Operations are applied in order. If any operation fails, none are applied
(atomic). The in-memory index is updated after a successful patch.

**Output:**
```json
{
  "page": "Crowd Control",
  "links_to": ["Enchanter", "Bard", "Mez", "Pulling", "Root", "Root Mechanics"]
}
```

---

### `rename_page`

Renames a page and updates all `[[wikilinks]]` across the brain that reference
the old name. The page content is preserved. This is atomic: the rename and all
link updates happen together (and produce a single commit if `-auto-commit` is
enabled).

**Input:**
```json
{
  "page": "Crowd Control",
  "new_name": "Crowd Control Mechanics"
}
```

**Behavior:**

1. Find the page by current name (case-insensitive).
2. Update the page name (file heading and filename are handled internally).
3. Scan all other pages for `[[Crowd Control]]` links (case-insensitive) and
   replace them with `[[Crowd Control Mechanics]]`.
4. Update the in-memory index (name lookup, link graph, search index).

**Output:**
```json
{
  "page": "Crowd Control Mechanics",
  "old_name": "Crowd Control"
}
```

---

### `delete_page`

Removes a page from the brain. Does not update other pages that link to the
deleted page (broken links are tolerated and caught by future maintenance).

**Input:**
```json
{
  "page": "Obsolete Concept"
}
```

**Output:**
```json
{
  "page": "Obsolete Concept"
}
```

---

### `list_pages`

Returns a sorted, paginated list of page names. Useful for dream sessions that
need to systematically discover orphaned or underlinked pages, and for recall
sessions that want to survey the brain's top hub concepts before diving in.

**Input:**
```json
{
  "sort_by": "least_linked",
  "limit": 50,
  "offset": 0,
  "filter": ["combat", "enchanter"]
}
```

All fields are optional. Defaults: `sort_by: "alphabetical"`, `limit: 50`,
`offset: 0`, no filter.

**`sort_by` values:**

- **`alphabetical`** (default): page names sorted A–Z.
- **`least_linked`**: pages with the fewest inbound links first. Surfaces
  orphans and isolated concepts — the primary entry point for dream sessions.
- **`most_linked`**: pages with the most inbound links first. Surfaces hub
  concepts — useful for recall sessions building broad context.
- **`newest`**: pages sorted by `last_updated` descending — most recently
  written first. Useful for finding pages that were recently added or edited
  but may not yet have been linked into the graph.
- **`oldest`**: pages sorted by `last_updated` ascending — least recently
  written first. Useful for stale-content review, working through the oldest
  pages systematically.

**`filter`**: array of keywords. Page name must contain all keywords
(case-insensitive, AND match). Narrows the list to pages whose names contain
every specified term. This is name substring matching, not search — use
`search` for semantic queries.

**`limit`** and **`offset`**: standard pagination. An agent can walk the full
page list by incrementing `offset` by `limit` until `offset >= total`.

**Output format depends on `sort_by`.** For `alphabetical`, `least_linked`,
and `most_linked`, `pages` is a flat array of name strings:

```json
{
  "pages": ["Crowd Control", "Enchanter", "Pulling"],
  "total": 247,
  "offset": 0,
  "limit": 50
}
```

For `newest` and `oldest`, `pages` is an array of objects that include
`last_updated` so timestamps are available without a separate `get_page` call:

```json
{
  "pages": [
    { "page": "Crowd Control", "last_updated": "2026-04-01T10:00:00Z" },
    { "page": "Enchanter",     "last_updated": "2025-09-14T08:44:21Z" },
    { "page": "Pulling",       "last_updated": "2023-02-28T17:03:55Z" }
  ],
  "total": 247,
  "offset": 0,
  "limit": 50
}
```

`total` is the count of matching pages before pagination is applied, so the
agent knows how many more pages remain.

---

## CLI Interface

### MCP Mode (default)

```
memento -content-dir ./brain
```

Starts the MCP server over stdio.

**Optional flags:**

- `-auto-commit`: After every write operation (`write_page`, `patch_page`,
  `rename_page`, `delete_page`), stage only the specific files that operation
  modified and create a git commit. Commit messages are descriptive but terse
  (e.g., `memento: updated "Crowd Control"`). Changes outside the content
  directory are never staged. On startup with this flag, the MCP verifies that
  `-content-dir` is inside a git repo and exits with an error if it is not.
  Without this flag, no git operations are performed.

---

## Registration

User-global Claude settings:

```json
{
  "mcpServers": {
    "memento": {
      "command": "/path/to/memento",
      "args": [
        "-content-dir", "/path/to/brain",
        "-auto-commit"
      ]
    }
  }
}
```

The `-content-dir` path can be absolute or relative to the working directory.
Multiple memento instances can be registered under different names:

```json
{
  "mcpServers": {
    "memento-personal": {
      "command": "/path/to/memento",
      "args": ["-content-dir", "/path/to/personal-brain"]
    },
    "memento-shared": {
      "command": "/path/to/memento",
      "args": ["-content-dir", "/path/to/shared-brain"]
    }
  }
}
```

---

## Repo Structure (MCP server)

```
memento/
├── main.go              # CLI flags, model load, index build, watcher, stdio serve
├── go.mod
├── go.sum
├── embed/
│   └── model.go         # Sentence-embedding model wrapper (go-sentex, all-MiniLM-L6-v2)
├── index/
│   ├── bm25.go          # BM25 inverted index with weighted field scoring
│   ├── chunk.go         # Page → chunks for embedding (heading / paragraph split)
│   ├── vector.go        # In-memory flat vector index, cosine search
│   ├── cache.go         # .memento-vectors sidecar persistence
│   ├── trigram.go       # Trigram fuzzy matching (legacy, unused when embeddings are available)
│   ├── graph.go         # Bidirectional wikilink graph
│   └── index.go         # Composite index (search pipeline, merge, relevance filter)
├── pages/
│   ├── store.go         # Filesystem ops (read, write, delete, scan)
│   ├── names.go         # Page name validation, case-insensitive lookup
│   └── parser.go        # Markdown parsing, wikilink extraction, heading extraction
├── watcher/
│   └── watcher.go       # fsnotify-based content-dir watcher with debouncing
├── tools/
│   ├── search.go
│   ├── get_page.go
│   ├── write_page.go
│   ├── patch_page.go
│   ├── rename_page.go
│   ├── delete_page.go
│   └── list_pages.go
├── testdata/
│   └── ...
└── README.md
```

---

## What memento Does NOT Do

- **No git operations by default.** With `-auto-commit`, the MCP commits after
  each write. Without it, committing is the caller's responsibility.
- **No page types or templates.** A page is a page.
- **No access control.** The MCP assumes it is the sole writer.
- **No validation enforcement.** Broken links and orphaned pages are tolerated.
- **No ordering or ranking of pages** outside of search relevance.
- **No notifications or triggers to clients.** memento does not push events
  over MCP; it watches the filesystem internally to keep its index fresh, but
  clients only ever see tool responses.
- **No Obsidian dependency.** Wikilink syntax is just a convention in markdown
  files. Obsidian can view the files if desired, but is not required.
- **No disambiguation enforcement.** The MCP stores pages; skill instructions
  teach agents to search before writing and use descriptive page names.

---

## Example Skill Patterns

Skills are prompts you wire to a memento brain instance. The skills define what
the brain is *for* — swap them out and the same server serves a completely
different purpose. The MCP server name you register (e.g. `memento`, `kb`,
`gamedesign`) should match the skill name prefix so agents naturally target the
right brain when multiple instances are active.

Example skill sets are located under `example-patterns/`.

### Second Brain Pattern

located at `example-patterns/memento/claude/skills/`

For a personal, cross-session memory store. Register the MCP server as `memento`.

- **`memento-recall`** — Search the brain continuously during a task whenever a
  term or concept surfaces that might have prior context. Follow `[[links]]` to
  build deeper understanding. Use retrieved context to avoid re-deriving decisions
  already made.
- **`memento-snapshot`** — Jot a concept mid-task while the context is fresh. One
  concept, one page, done in seconds. Uses `patch_page append` so no read-before-write
  is needed — creates the page automatically if it doesn't exist.
- **`memento-sleep`** — End-of-session sweep. Review the conversation and capture
  durable knowledge: decisions, constraints, terminology, relationships. Err toward
  writing; noise is cheap and `memento-dream` cleans it up later.
- **`memento-dream`** — Dedicated maintenance. Find orphaned pages, consolidate
  duplicates, split oversized pages, strengthen cross-links, rename poorly scoped pages.

### Knowledge Brain Pattern

located at `example-patterns/kb/claude/skills/`

For a structured, project-scoped documentation workspace. Register the MCP server
as `kb` (or a project-specific name like `gamedesign`). Use matching skill name
prefixes so agents target the right brain when multiple instances are active.

- **`kb-explore`** — Find relevant pages before answering or writing. Used
  automatically when context might live in the knowledge brain.
- **`kb-update`** — Write or revise a specific page when the user asks to update
  the docs. Deliberate and targeted — not a session sweep.

---

## Design Decisions

**Why use the page name directly as the filename?**
The filename on disk is `{page name}.md` — no lowercasing, no slug transformation.
`[[Crowd Control]]` resolves to `Crowd Control.md`, which is exactly what Obsidian
expects when resolving wikilinks. This eliminates a mapping layer, makes files
browsable in Obsidian without configuration, and keeps filenames human-readable.
The tradeoff is that page names must avoid characters forbidden in filenames across
operating systems (`* " [ ] # ^ | < > : ? / \`), but these rarely appear in
natural-language concept names.

**Why reject forbidden characters instead of stripping them?**
Silently stripping characters would create a disconnect between the page name the
agent intended and the one stored, which could cause confusion when searching or
linking. Rejecting with a clear error lets the agent choose an alternative name
intentionally. The forbidden set is the union of restrictions across Windows, macOS,
Linux, iOS, and Android — matching Obsidian's own validation — so content directories
are portable across platforms.

**Why `[[wikilinks]]` instead of `#tags`?**
Every concept worth tagging is worth having a page for. A `[[link]]` creates a node
in the knowledge graph that can accumulate its own context over time. Tags are dead
ends; links are connective tissue.

**Why flat directory, no hierarchy?**
Hierarchy forces premature categorization. A page about "Enchanter" could belong
under "classes/" or "crowd-control/" or "party-roles/". Flat structure with rich
linking avoids this problem entirely. Discovery comes from search and graph
traversal, not folder browsing.

**Why freeform pages, no templates?**
The brain is an extension of the agent's thinking. Rigid templates constrain how
knowledge can be represented. The agent should organize content however is most
useful for the concept at hand. Some pages might be a single paragraph. Others
might have detailed sections. The structure emerges from the content.

**Why BM25 instead of TF-IDF?**
BM25 adds term frequency saturation (diminishing returns after a term appears many
times) and document length normalization (longer pages don't dominate). Both matter
for the brain: pages vary wildly in length, and some concepts are mentioned
repeatedly in long pages. BM25 uses the same inverted index as TF-IDF but produces
better rankings. It's the standard across the search/retrieval space (Elasticsearch,
SQLite FTS5, and every Context7-style tool uses it).

**Why weighted field scoring (title 10x, wikilinks 3x)?**
A search for "Crowd Control" should reliably return the page *named* "Crowd Control"
first, not a page that mentions it once in passing. Weighting the page title heavily
ensures concept pages rank highest for their own name. Weighting wikilink targets
recognizes that `[[Crowd Control]]` appearing on another page is a deliberate
declaration of relevance, stronger than the words appearing incidentally in body text.

**Why combine BM25 and vector search instead of picking one?**
Keyword search and semantic search fail in different ways. BM25 misses synonyms
and paraphrases ("deployment strategy" ↛ "CICD Pipeline"). Pure vector search
surfaces thematically-similar pages that don't contain the exact term the agent
asked about, and loses the strong signal of a rare keyword that appears in a
page title. Running them in parallel and merging — with vector as the semantic
gate and BM25 as a boost on top — captures both signals. The relevance
threshold then cuts the long tail on either side.

**Why flat vector scan instead of an ANN index?**
At the expected scale (hundreds of pages, low thousands of chunks), a brute-force
cosine comparison across 384-dim vectors completes in under a millisecond. An
approximate-nearest-neighbor index (HNSW, IVF) would add dependencies, recall
loss, and build-time complexity for no perceivable latency gain. If a brain
ever grows into the tens of thousands of chunks, swapping the flat scan for an
ANN index is a local change behind the same `vector.Search` interface.

**Why chunk pages instead of embedding the whole page?**
`all-MiniLM-L6-v2` has a 256-token context window; a long page would be silently
truncated, making the back half invisible to semantic search. Chunking on
section headings also means a vector hit carries a useful line anchor, so the
snippet can point at the paragraph that actually matched rather than the top
of the page. Prefixing every chunk with the page's `# Title` line keeps page
identity in the embedding so chunks don't drift free of their page.

**Why a sidecar cache instead of re-embedding on every startup?**
Embedding hundreds of pages takes several seconds even with a fast model. The
cache makes cold start effectively free for unchanged content: only pages
whose hash has changed are re-embedded. The cache is keyed on model identity
(model ID + go-sentex version) so a model change transparently invalidates
everything, preventing mixed-model drift. Keeping it as a sidecar file —
rather than burying it in a user cache directory — makes it discoverable and
trivially resettable by deleting it.

**Why is a model-load failure fatal?**
Semantic search is a primary part of the retrieval experience. A server that
silently degrades to BM25-only would produce noticeably worse search results
in a way that's hard to notice and easy to blame on the content. Failing loudly
at startup makes the misconfiguration obvious and fixable (install the model,
set `HF_HOME`, check network on first run).

**Why watch the filesystem instead of only updating on tool writes?**
A memento brain is a directory of plain markdown files, by design browsable
and editable outside the MCP: in Obsidian, a text editor, a `git pull`, or
another memento instance pointed at the same directory. Without live reload,
external edits would be invisible to search until the server restarts — a
foot-gun that silently degrades accuracy. Watching `fsnotify` events and
reindexing in-place keeps the in-memory index faithful to the filesystem at
all times, which is the stronger invariant.

**Why debounce, and why per-file?**
Many editors save by writing to a temp file and renaming, producing a burst
of create/modify/delete events within milliseconds. Debouncing per-file
coalesces that burst into a single reindex. Debouncing globally would delay
independent edits to unrelated files; per-file keeps reaction time tight
while still collapsing bursts for each individual page.

**Why a relevance threshold (50% of top score)?**
As the brain grows, a broad search might match dozens of pages. Most of those matches
are noise. Dropping results below 50% of the top result's score keeps results focused.
The threshold is hardcoded; if it proves too aggressive or too lenient in practice, it
can be tuned.

**Why token budgeting in search results?**
The consumer of search results is an LLM with finite context. Returning 10 results
that total 15,000 tokens is wasteful when the agent will only use the first few.
`max_tokens` lets the agent precisely control how much context it consumes, which
directly translates to more room for actual work in the same session.

**Why stemming?**
Agents invent their own search terms and may use different morphological forms than
what appears in page content. Stemming ("enchanting" and "enchanter" both match
"enchant") improves recall. Porter stemming is the standard across the search/retrieval
space.

**Why allow broken links?**
Over-validation creates friction that discourages writing. The brain should be easy
to write to. Broken links are caught by maintenance sessions, not prevented at
write time. An aspirational link to a page that doesn't exist yet is a feature,
not a bug — it signals that a concept is referenced but not yet elaborated.

**Why no disambiguation enforcement in the MCP?**
Disambiguation requires judgment about whether two uses of the same term refer to
the same concept. That's the agent's job (guided by skill instructions), not the
MCP's. The MCP stores pages; the skill teaches agents to search before writing and
use descriptive names when needed.

**Why both text-based and line-based replacement in `patch_page`?**
Text-based replace (`str_replace` pattern) is best when the agent knows a specific
phrase to swap. Line-based replace is best when the agent has already read a section
via line ranges and wants to rewrite it entirely. The exact-match requirement on
text-based replace can fail on long or variable passages; line-based replace is
guaranteed to target the right location. Both patterns are useful for different
situations.

**Why a `rename_page` tool instead of delete + create?**
Renaming a concept is a graph-wide operation. Without `rename_page`, the agent would
need to delete the old page, create the new one, then manually find and update every
page that linked to the old name. That's error-prone and the agent might miss
references. Since the MCP has the full link graph in memory, it can atomically rename
the page and update all references in a single operation (and a single git commit
with `-auto-commit`).

**Why does `append`/`prepend` create pages that don't exist?**
The brain has two modes: accumulation (jotting notes during a session) and
consolidation (cleaning up and reorganizing). During accumulation, the agent
shouldn't need to search for a page or check if it exists before writing a note.
`patch_page` with `append` is the "fire and forget" write tool: if the page
exists, the note is added; if it doesn't, the page is created. This keeps the
jotting workflow to a single tool call with no preconditions. `replace` and
`replace_lines` still require the page to exist because they reference specific
content that must be present.

**Why separate `write_page` and `patch_page`?**
`write_page` is for creating new pages or fully rewriting existing ones when the
agent has a complete picture of what the page should say. `patch_page` is for
targeted edits — adding a paragraph, fixing a link, appending a note — without
risking accidental content loss. Both are needed because the write patterns are
genuinely different.

**Why build a custom MCP instead of using an existing Obsidian MCP?**
Existing Obsidian MCPs (`mcpvault`, `mcp-obsidian`) wrap Obsidian's REST API, which
requires Obsidian to be running. memento operates directly on the filesystem: no
GUI dependency, works in headless environments, works in CI, works wherever Claude
Code runs. The `[[wikilink]]` convention is just markdown — Obsidian can view the
files if desired but is never required.

**Why in-memory indexing instead of SQLite?**
Agents read and write markdown natively. Keeping the source of truth as markdown
files means no serialization overhead and the content is readable outside of the
MCP. The in-memory index (built on startup from the markdown files) provides fast
search without a second representation that could drift. The one exception is the
embedding vector cache (`.memento-vectors`), which is pure derived data keyed by
content hash — it never serves as a source of truth, it only skips recomputation
on startup, and discarding it always produces a correct index.

**Why line numbers in search results and multi-range support in `get_page`?**
This mirrors the grep-then-read pattern agents already use with source code: search
finds relevant locations with line numbers, then a single `get_page` call with
multiple ranges fetches context around all matches at once. Without multi-range
support, an agent getting three search hits on the same page would need three
separate `get_page` calls. As pages grow over time through appends and updates,
this prevents unnecessary token consumption and round trips.

**Why `-auto-commit`?**
Git history is the audit trail for brain content, but remembering to commit is
friction that discourages use. Auto-commit makes every write operation produce a
git commit transparently, so the history builds itself. Per-tool-call commits (not
batched) keep the granularity useful: each commit corresponds to one logical change.
Only the files that operation modified are staged — never unrelated changes in the
wider repository. This keeps memento's commits clean even when the content
directory is a subdirectory of a larger repo. The flag is opt-in because not every
content directory will be a git repo.

**Why `list_pages` instead of relying solely on `search`?**
Dream and recall skills use search as the primary discovery mechanism, but search
requires knowing what to look for. Orphaned pages and underlinked concepts are
exactly the ones an agent won't think to search for. `list_pages` sorted by
`least_linked` gives the dream skill a systematic entry point into the graph's
edges — pages that exist but have been forgotten by the link structure.
`most_linked` gives recall sessions a "what are the core concepts" shortcut.
Pagination and name-only output keep token cost low even over large brains.

**Why server-side search instead of exposing the raw index?**
The search tool encapsulates ranking logic (BM25 + vector merge + graph boost) that
agents shouldn't need to reason about. The agent asks a question; the MCP returns
ranked, contextualized results.

**Why expose `last_updated` timestamps?**
Agents interacting with the brain have no shell or git access — the MCP is their only
window into the content. Without timestamps, an agent reading a page has no way to know
whether a decision was captured last week or three years ago. This affects how
confidently it should overwrite, how much it should trust the content as current, and
how it should prioritize maintenance work. Surfacing timestamps on every read tool lets
agents calibrate trust without extra round trips.

**Why derive timestamps from git rather than storing them internally?**
Git commit timestamps are durable — they survive `git clone` and `git pull`, keeping
timestamps consistent across machines. Storing timestamps in file content or a sidecar
would create a second source of truth that could drift. The MCP is stateless between
restarts; any internal store would be lost. Filesystem mtime is the fallback for
non-git or uncommitted files, which covers single-machine setups and newly created pages.

**Why does `list_pages` return objects for `newest`/`oldest` but strings for other sorts?**
The alphabetical, `least_linked`, and `most_linked` sorts are link-graph and
name-oriented — the agent typically needs the names to make subsequent `get_page` or
`search` calls, and timestamps add no signal. Returning flat strings keeps token cost
low and preserves backward compatibility. The `newest` and `oldest` sorts are
timestamp-oriented by nature: the agent needs the timestamps to act on the sort order
(e.g. to skip pages updated recently, or to prioritize pages not touched in years).
Embedding `last_updated` in the list entry avoids a separate `get_page` call per page
for the common case of working through a timestamp-sorted list.
