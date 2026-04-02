# Memento: Second Brain MCP Server

## Overview

A persistent, evolving knowledge store that agents use as long-term memory across
planning sessions. Agents read from it to build context and write to it to capture
decisions, terminology, and reasoning. The brain grows and reorganizes over time
through agent interaction.

**Repo:** `github.com/edgetools/memento` (the MCP server code)

A memento "instance" (the actual brain content) is any git repo or directory the
user points the server at. The server is generic; the content is personal.

---

## Architecture

Go MCP server using `mark3labs/mcp-go`, stdio transport. Single binary, no runtime
dependencies. Takes a `-content-dir` flag pointing at a directory of markdown files.

On startup, parses all `.md` files in `-content-dir` to build an in-memory index:

- **BM25 inverted index** with weighted field scoring (title, wikilinks, body)
  and Porter stemming for relevance-ranked keyword search
- **Trigram index** for fuzzy matching as a fallback layer (handles typos,
  partial terms, morphological edge cases)
- **Bidirectional link graph** parsed from `[[wikilinks]]` in page content

Index is rebuilt on startup and updated in-place on writes. If search performance
becomes a problem at scale, a SQLite cache layer can be added later without changing
the tool interface.

---

## Content Model

### Pages and Names

Every page is a markdown file in a flat directory (no hierarchy). Pages are
identified by their **page name**, which is a human-readable string like
"Crowd Control" or "Enchanter Mez Strategy". The page name is the single
identifier used everywhere: in tool calls, in `[[wikilinks]]`, and as the
concept's identity in the brain.

The MCP maps page names to filesystem-compatible filenames and file headings
internally. These are implementation details that agents never interact with.
Agents work exclusively with page names.

**Name resolution:** Page name lookups are case-insensitive with whitespace
normalization (collapsing multiple spaces, trimming). So `[[Crowd Control]]`,
`[[crowd control]]`, and `[[Crowd  Control]]` all resolve to the same page.
The canonical casing is whatever was used when the page was first created.

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

The search system uses a multi-layer approach: BM25 keyword ranking with weighted
fields as the primary layer, trigram fuzzy matching as a fallback, and link graph
boosting as a post-processing step.

### Search Pipeline

```
Query → Layer 1: BM25 (stemmed, weighted fields)
         ↓ enough results? → yes → Graph Boost → Relevance Filter → Return
         ↓ no
       Layer 2: Trigram Fuzzy Matching
         ↓ expand query terms with fuzzy matches → re-run BM25
         ↓
       Graph Boost → Relevance Filter → Return
```

Layer 2 only runs when Layer 1 returns fewer than 3 results. This avoids the
noise of fuzzy matching when exact/stemmed matching already found good results.

### Layer 1: BM25 (Best Match 25)

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

### Layer 2: Trigram Fuzzy Matching (Fallback)

Trigrams are 3-character sliding windows used for approximate string matching.
"enchanter" becomes: `enc`, `nch`, `cha`, `han`, `ant`, `nte`, `ter`. A search
for "enchaner" (typo) has high trigram overlap with "enchanter" and still matches.

**This layer only fires when Layer 1 returns fewer than 3 results.** This avoids
polluting good exact results with fuzzy noise.

**Build process:**

1. For each unique term in the BM25 index, generate its trigram set
2. Store a reverse mapping: trigram → list of terms containing it
3. Also generate trigrams for `[[wikilink]]` targets as compound terms

**Fallback process:**

1. Generate trigrams for each query term
2. Find indexed terms with high trigram overlap (Jaccard similarity above a
   threshold)
3. Re-run BM25 with the expanded term set (original terms + fuzzy matches)
4. Fuzzy-matched terms receive a reduced weight to prefer exact matches when
   both are present

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
      "snippet": "...typically the [[Enchanter]] is assigned mez duty during pulls...",
      "line": 14,
      "linked_pages": [
        {
          "page": "Enchanter",
          "snippet": "The enchanter is a utility class specializing in mesmerize...",
          "line": 3
        },
        {
          "page": "Pulling",
          "snippet": "Pull strategy depends on available CC — [[Crowd Control]]...",
          "line": 8
        }
      ]
    }
  ]
}
```

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

## CLI Interface

### MCP Mode (default)

```
memento -content-dir ./brain
```

Starts the MCP server over stdio.

**Optional flags:**

- `-auto-commit`: Automatically `git add` and `git commit` after every write
  operation (`write_page`, `patch_page`, `rename_page`, `delete_page`). Commit
  messages are descriptive but terse (e.g., `memento: updated "Crowd Control"`).
  On startup with this flag, the MCP verifies that `-content-dir` is inside a
  git repo and exits with an error if it is not. Without this flag, no git
  operations are performed.

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
├── main.go              # CLI flags, stdio serve
├── go.mod
├── go.sum
├── index/
│   ├── bm25.go          # BM25 inverted index with weighted field scoring
│   ├── trigram.go        # Trigram fuzzy matching (fallback layer)
│   ├── graph.go          # Bidirectional wikilink graph
│   └── index.go          # Composite index (search pipeline, relevance filter)
├── pages/
│   ├── store.go          # Filesystem ops (read, write, delete, scan)
│   ├── names.go          # Page name ↔ filename mapping, case-insensitive lookup
│   └── parser.go         # Markdown parsing, wikilink extraction, heading extraction
├── tools/
│   ├── search.go
│   ├── get_page.go
│   ├── write_page.go
│   ├── patch_page.go
│   ├── rename_page.go
│   └── delete_page.go
├── testdata/
│   └── ...
└── README.md
```

---

## What Memento Does NOT Do

- **No git operations by default.** With `-auto-commit`, the MCP commits after
  each write. Without it, committing is the caller's responsibility.
- **No page types or templates.** A page is a page.
- **No access control.** The MCP assumes it is the sole writer.
- **No validation enforcement.** Broken links and orphaned pages are tolerated.
- **No ordering or ranking of pages** outside of search relevance.
- **No notifications or triggers.** Memento is a passive tool.
- **No Obsidian dependency.** Wikilink syntax is just a convention in markdown
  files. Obsidian can view the files if desired, but is not required.
- **No disambiguation enforcement.** The MCP stores pages; skill instructions
  teach agents to search before writing and use descriptive page names.

---

## Skills (outline, to be designed separately)

### "Write to the Brain" Skill

For use at the end of a planning session (Claude Desktop or Claude Code
interactive).

Core instructions:
- Review the conversation for key concepts, decisions, and terminology
- Search memento for existing pages related to those concepts
- Update existing pages with new context, using `[[links]]` rather than
  re-explaining concepts that already have pages
- Create new pages for concepts that don't have one yet
- Before writing a new page, search for the term — if a page exists with
  different content (potential disambiguation), use a more descriptive name
- Link everything together: new content should reference concept pages, and
  concept pages should be updated to reference related concepts

### "Read from the Brain" Skill

For use when starting work that might benefit from prior context.

Core instructions:
- When encountering a topic or term, search memento before making assumptions
- Follow `[[links]]` in retrieved pages to build deeper understanding (the
  Wikipedia rabbit hole pattern)
- Use retrieved context to avoid re-deriving decisions already made
- If memento doesn't have relevant content, proceed without it — don't block

### "Maintain the Brain" Skill

For dedicated cleanup sessions.

Core instructions:
- Identify orphaned pages (no links in or out) and evaluate whether to link
  or delete them
- Find pages that re-explain concepts covered by other pages — consolidate
  into the canonical page and replace redundant text with links
- Break up pages that have grown too large into focused sub-pages
- Strengthen cross-links where related concepts aren't yet connected
- Delete pages that are no longer relevant

---

## Design Decisions

**Why readable page names instead of slugs?**
Agents are the primary interface to the brain, not humans browsing files. Readable
names like `[[Enchanter Mez Strategy]]` are self-documenting and natural for agents
to produce. They also reduce disambiguation problems because descriptive names are
inherently less ambiguous than terse identifiers. The MCP handles the mapping to
filesystem-compatible filenames internally; agents never see slugs.

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

**Why fallback layers instead of merged scoring for fuzzy matching?**
Merged scoring runs both exact BM25 and fuzzy trigram matching on every query, then
combines scores. This adds noise when exact matching already found good results. The
fallback approach only runs the fuzzier, noisier trigram layer when BM25 returns
fewer than 3 results. Simpler to implement, easier to reason about, and avoids the
tuning headache of balancing exact vs fuzzy weights.

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
requires Obsidian to be running. Memento operates directly on the filesystem: no
GUI dependency, works in headless environments, works in CI, works wherever Claude
Code runs. The `[[wikilink]]` convention is just markdown — Obsidian can view the
files if desired but is never required.

**Why in-memory indexing instead of SQLite?**
Agents read and write markdown natively. Keeping the source of truth as markdown
files means no serialization overhead and the content is readable outside of the
MCP. The in-memory index (built on startup from the markdown files) provides fast
search without a second representation that could drift. If scale demands it, a
SQLite cache can be added later behind the same tool interface.

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
The flag is opt-in because not every content directory will be a git repo.

**Why no `list_pages` tool?**
An agent would almost never want an unranked list of every page. Search is the
primary discovery mechanism. If the brain has hundreds of pages, a list is noise.
If we find a use case for listing later, we can add it.

**Why server-side search instead of exposing the raw index?**
The search tool encapsulates ranking logic (BM25 + trigram fallback + graph boost) that
agents shouldn't need to reason about. The agent asks a question; the MCP returns
ranked, contextualized results.
