# Memento MCP Server -- TDD Implementation Plan

## Context

Memento is a greenfield Go MCP server that acts as a persistent knowledge store ("second brain") for LLM agents. The design is fully specified in `MVP_DESIGN.md`. This plan breaks the implementation into 7 deliverables following TDD: we write black-box tests first, then implement to make them pass. The repo currently has zero code -- only the design doc.

Module: `github.com/edgetools/memento` | Primary dep: `github.com/mark3labs/mcp-go`

---

## Test Strategy

**BEFORE WRITING ANY IMPLEMENTATION CODE (go source files that don't end in _test.go), ALL TEST IMPLEMENTATIONS (in the go source files that end in _test.go) MUST BE COMPLETE SPEFICIATIONS THAT FULLY TEST THE BEHAVIOR**

When writing tests, think of yourself as an engineer whose job is to write complete tests that will satisfy the goals of the spec, and that you'll be handing off those fully implemented tests to another engineer who will then make those tests pass, but who won't modify the tests.

**Three tiers, no mocks:**

1. **Unit tests** (`pages_test`, `index_test`) -- black-box tests of each package's public API using `t.TempDir()` for filesystem, hand-constructed data for indexing
2. **Integration tests** (`tools_test`) -- black-box tests via `client.NewInProcessClient(server)` exercising the full MCP tool stack. These are the contract tests derived directly from the design doc
3. **No mocks** -- `pages/` uses real filesystem via `t.TempDir()`, `index/` is pure in-memory, integration tests compose both layers naturally

**Conventions:** External test packages (`pages_test`, `index_test`, `tools_test`). Table-driven subtests. `t.TempDir()` for isolation. `t.Parallel()` where safe.

---

## Dependency Graph

```
D1: pages/names + pages/parser     (no deps)
 |
 +-----+-----+
 v           v
D2: pages/store   D3: index/bm25 + index/trigram   (parallel)
 |           |
 +-----+-----+
       v
D4: index/graph + index/index
       |
       v
D5: tools/write_page, get_page, delete_page
       |
       v
D6: tools/patch_page, rename_page, search
       |
       v
D7: main.go + auto-commit
```

D2 and D3 can be done in parallel (both depend only on D1).

---

## Deliverable 1: Scaffolding + Pages Names & Parser

**Creates:** `go.mod`, `pages/names.go`, `pages/parser.go`, `pages/names_test.go`, `pages/parser_test.go`, `testdata/*.md`

### pages/names_test.go -- Name normalization & filename mapping

Public API: `Normalize(name) string`, `NameToFilename(name) string`, `FilenameToName(filename) string`, `NamesMatch(a, b) bool`

**Filename mapping strategy:** Cross-platform safe (Linux + Mac + Windows). Lowercase, spaces/whitespace -> hyphens, strip characters illegal on any OS (`<>:"/\|?*`, control chars), collapse consecutive hyphens, trim leading/trailing hyphens/dots. Append `.md`. Unicode letters are preserved (valid on all three OSes). Long names truncated to a safe length (200 chars before `.md`).

| Test | Input | Expected |
|------|-------|----------|
| `Normalize/lowercase` | `"Crowd Control"` | `"crowd control"` |
| `Normalize/mixed_case` | `"cRoWd CoNtRoL"` | `"crowd control"` |
| `Normalize/extra_whitespace` | `"Crowd  Control"` | `"crowd control"` |
| `Normalize/leading_trailing` | `"  Crowd Control  "` | `"crowd control"` |
| `Normalize/tabs` | `"Crowd\tControl"` | `"crowd control"` |
| `Normalize/single_word` | `"Enchanter"` | `"enchanter"` |
| `Normalize/empty` | `""` | `""` |
| `NameToFilename/simple` | `"Crowd Control"` | `"crowd-control.md"` |
| `NameToFilename/special_chars` | `"Aggro From Healing"` | `"aggro-from-healing.md"` |
| `NameToFilename/strips_illegal_chars` | `"What: A <Test>?"` | `"what-a-test.md"` |
| `NameToFilename/unicode_preserved` | `"Über Strategy"` | `"über-strategy.md"` |
| `NameToFilename/collapses_hyphens` | `"A -- B"` | `"a-b.md"` |
| `NameToFilename/long_name_truncated` | 300-char name | filename <= 204 chars |
| `NameToFilename/roundtrip` | Various | `FilenameToName(NameToFilename(n))` recovers usable name |
| `FilenameToName/simple` | `"crowd-control.md"` | `"crowd control"` |
| `NamesMatch/case_insensitive` | `"Crowd Control"`, `"crowd control"` | `true` |
| `NamesMatch/whitespace_norm` | `"Crowd  Control"`, `"Crowd Control"` | `true` |
| `NamesMatch/different` | `"Crowd Control"`, `"Enchanter"` | `false` |

### pages/parser_test.go -- Markdown parsing & wikilink extraction

Public API: `Parse(name string, content []byte) Page` returning Title, Body, WikiLinks, Lines

| Test | Description |
|------|-------------|
| `Parse/extracts_title_from_h1` | `"# Crowd Control\n\nBody"` -> Title: `"Crowd Control"` |
| `Parse/no_heading_uses_page_name` | `"Just body"` -> Title = page name arg |
| `Parse/extracts_single_wikilink` | `"See [[Enchanter]]"` -> WikiLinks: `["Enchanter"]` |
| `Parse/extracts_multiple_wikilinks` | `"[[A]] and [[B]] and [[C]]"` -> all three |
| `Parse/deduplicates_wikilinks` | `"[[A]] ... [[A]]"` -> single entry |
| `Parse/no_wikilinks` | Plain text -> empty slice |
| `Parse/ignores_wikilink_in_code_block` | Fenced code block -> not extracted |
| `Parse/ignores_wikilink_in_inline_code` | Backtick-wrapped -> not extracted |
| `Parse/empty_wikilink_ignored` | `"[[]]"` -> not extracted |
| `Parse/body_excludes_title` | Body field doesn't contain h1 line |
| `Parse/h2_not_treated_as_title` | `"## Not Title"` -> title = page name |
| `Parse/preserves_wikilink_casing` | `"[[Crowd Control]]"` -> preserves original case |
| `Parse/empty_content` | No crash, empty fields |
| `Parse/multiline` | Multi-paragraph -> correct line count, all links found |

---

## Deliverable 2: Pages Store

**Creates:** `pages/store.go`, `pages/store_test.go`

**Depends on:** D1

Public API: `NewStore(dir)`, `Write(name, content) (Page, error)`, `Load(name) (Page, error)`, `Delete(name) error`, `Rename(old, new) error`, `Scan() []Page`, `Exists(name) bool`

### Test helpers
- `writeTestPage(t, dir, name, content)` -- writes .md file for setup
- `readTestPage(t, dir, name) string` -- reads back for assertions

### pages/store_test.go

| Test | Description |
|------|-------------|
| `Write/creates_new_page` | File exists on disk with correct content |
| `Write/adds_heading` | Content gets `# Page Name` prepended |
| `Write/replaces_agent_heading` | `# Wrong` in content replaced with `# Page Name` |
| `Write/overwrites_existing` | Second write replaces first |
| `Write/case_insensitive_overwrite` | Write "Foo", write "foo" -> same file |
| `Write/returns_parsed_page` | Return value has correct WikiLinks |
| `Load/reads_existing` | Content matches what was written |
| `Load/case_insensitive` | Write "Foo Bar", load "foo bar" works |
| `Load/whitespace_normalized` | Write "Foo Bar", load "Foo  Bar" works |
| `Load/not_found_errors` | Descriptive error |
| `Load/returns_parsed_fields` | Title, WikiLinks, Body populated |
| `Delete/removes_file` | File is gone |
| `Delete/case_insensitive` | Delete "foo" removes "Foo" |
| `Delete/not_found_errors` | Error returned |
| `Rename/changes_filename` | Old gone, new exists |
| `Rename/preserves_content` | Body survives |
| `Rename/updates_heading` | `# New Name` in file |
| `Rename/case_insensitive_source` | Finds original by normalized name |
| `Rename/target_exists_errors` | Can't overwrite existing page |
| `Rename/source_not_found_errors` | Error returned |
| `Scan/returns_all` | 3 pages written, 3 returned |
| `Scan/empty_dir` | Empty slice |
| `Scan/ignores_non_md` | .txt file skipped |
| `Exists/true_for_existing` | Returns true |
| `Exists/false_for_missing` | Returns false |
| `Exists/case_insensitive` | Write "Foo", Exists("foo") = true |

---

## Deliverable 3: Index BM25 + Trigram

**Creates:** `index/bm25.go`, `index/trigram.go`, `index/bm25_test.go`, `index/trigram_test.go`

**Depends on:** D1 (for `pages.Page` type). **Can run in parallel with D2.**

### Test helper
- `makePage(name, body string, links []string) pages.Page` -- construct Page without filesystem

### index/bm25_test.go

| Test | Description |
|------|-------------|
| `Search/title_match_ranks_highest` | Page titled "Enchanter" > page mentioning in body |
| `Search/wikilink_outranks_body` | `[[Enchanter]]` link > "enchanter" in body only |
| `Search/body_match_works` | Term in body appears in results |
| `Search/multi_term_query` | "crowd control" matches both-term pages higher |
| `Search/stemming` | "enchanting" matches "enchanter" |
| `Search/stop_words_ignored` | "the enchanter" = "enchanter" |
| `Search/no_results` | Absent term -> empty |
| `Search/length_normalization` | Short dense page scores well vs long sparse page |
| `Search/case_insensitive` | "ENCHANTER" matches "enchanter" |
| `Add_Remove/add_then_search` | Added page is findable |
| `Add_Remove/remove_then_search` | Removed page is gone |
| `Add_Remove/update_page` | Re-add with new content, search reflects change |
| `Search/compound_wikilink` | `[[Crowd Control]]` boosts "crowd control" phrase |
| `Search/empty_query` | No crash |
| `Search/multiple_pages_ranked` | 5+ pages, results descending by score |

### index/trigram_test.go

| Test | Description |
|------|-------------|
| `FuzzyMatch/exact` | "enchanter" matches itself, similarity 1.0 |
| `FuzzyMatch/one_char_typo` | "enchaner" matches "enchanter" |
| `FuzzyMatch/two_char_typo` | "enchner" still matches (above threshold) |
| `FuzzyMatch/completely_different` | "wizard" doesn't match "enchanter" |
| `FuzzyMatch/short_term` | "mez" handled correctly |
| `FuzzyMatch/multiple_matches` | Returns all terms above threshold |
| `FuzzyMatch/empty_query` | No crash |
| `FuzzyMatch/compound_term` | "crowd contrl" matches "crowd control" |
| `Add/builds_trigrams` | Verify correct trigram set generated |
| `Similarity/jaccard` | Known inputs produce expected coefficient |

---

## Deliverable 4: Index Graph + Composite

**Creates:** `index/graph.go`, `index/index.go`, `index/graph_test.go`, `index/index_test.go`

**Depends on:** D1, D3

### index/graph_test.go

| Test | Description |
|------|-------------|
| `LinksTo/outbound` | A links to B,C -> LinksTo(A) = [B,C] |
| `LinkedFrom/inbound` | A links to B -> LinkedFrom(B) = [A] |
| `Bidirectional/symmetry` | A->B implies B linked-from A |
| `Remove/cleans_both_directions` | Remove A, gone from all LinkedFrom |
| `Add/deduplicates` | Same target twice -> stored once |
| `LinksTo/nonexistent` | Empty, no crash |
| `LinkedFrom/nonexistent` | Empty, no crash |
| `Update/replaces_links` | Re-add with different links, old gone |
| `Remove/nonexistent_target` | A->X (no page X), remove A works |
| `Graph/case_insensitive` | Lookups normalized |

### index/index_test.go (composite search pipeline)

| Test | Description |
|------|-------------|
| `Search/bm25_primary` | Good BM25 matches returned ranked |
| `Search/trigram_fallback_fires` | Typo query, BM25 <3 results, trigram expands |
| `Search/trigram_fallback_skipped` | BM25 >=3 results, no trigram |
| `Search/graph_boost_linked` | Linked page gets boosted |
| `Search/graph_boost_direct_and_linked` | Direct match + linked = extra boost |
| `Search/relevance_threshold` | Below 50% of top score dropped |
| `Search/max_results` | Honors limit |
| `Search/snippet_direct_match` | Shows query terms in context |
| `Search/snippet_title_match` | Shows first paragraph |
| `Search/snippet_linked_page` | Shows referring context |
| `Search/snippet_length` | ~250 chars |
| `Search/empty_index` | Empty results |
| `Build/indexes_all` | 5 pages, all searchable |
| `Update/reflects_changes` | Updated page content searchable |
| `Search/line_numbers` | Correct line numbers in results |

---

## Deliverable 5: Tools -- write_page, get_page, delete_page

**Creates:** `tools/tools.go`, `tools/write_page.go`, `tools/get_page.go`, `tools/delete_page.go`, `tools/tools_test.go`

**Depends on:** D1, D2, D4

### Test helpers (in tools_test package)
- `setupTestServer(t) (*client.InProcessClient, string)` -- temp dir, Store, Index, registers all tools, creates+initializes InProcessClient, `t.Cleanup` teardown
- `callTool(t, client, name, args) *mcp.CallToolResult` -- wraps with require.NoError
- `callToolExpectError(t, client, name, args) *mcp.CallToolResult` -- asserts IsError
- `parseJSON(t, result, v)` -- extract JSON from text content

### write_page tests

| Test | Description |
|------|-------------|
| `WritePage/creates_new` | write then get confirms content |
| `WritePage/returns_links_to` | Response has wikilinks |
| `WritePage/replaces_existing` | Second write wins |
| `WritePage/case_insensitive_overwrite` | "Foo" then "foo" -> same page |
| `WritePage/heading_managed` | No heading in content, get shows `# Name` |
| `WritePage/agent_heading_replaced` | `# Wrong` replaced |
| `WritePage/empty_content` | Page with just heading |
| `WritePage/updates_search_index` | Immediately searchable |
| `WritePage/missing_page_errors` | Tool error |
| `WritePage/missing_content_errors` | Tool error |

### get_page tests

| Test | Description |
|------|-------------|
| `GetPage/full_content` | Has content, total_lines, links_to, linked_from |
| `GetPage/case_insensitive` | Normalized lookup works |
| `GetPage/not_found_errors` | Tool error |
| `GetPage/line_range_single` | `lines: ["10-25"]` -> sections array |
| `GetPage/line_range_multiple` | `lines: ["1-5", "10-15"]` -> two sections |
| `GetPage/line_range_single_line` | `lines: ["5"]` -> one line |
| `GetPage/line_range_out_of_bounds` | Error or clamp |
| `GetPage/links_to` | Correct outbound links |
| `GetPage/linked_from` | Correct inbound links |
| `GetPage/total_lines_correct` | Matches actual count |

### delete_page tests

| Test | Description |
|------|-------------|
| `DeletePage/removes` | get_page returns not found after |
| `DeletePage/case_insensitive` | "foo" deletes "Foo" |
| `DeletePage/not_found_errors` | Tool error |
| `DeletePage/removes_from_index` | Search no longer finds it |
| `DeletePage/preserves_broken_links` | Linking pages unchanged |
| `DeletePage/returns_name` | Response has page name |

---

## Deliverable 6: Tools -- patch_page, rename_page, search

**Creates:** `tools/patch_page.go`, `tools/rename_page.go`, `tools/search.go`, additional tests

**Depends on:** D5

### patch_page tests

| Test | Description |
|------|-------------|
| `PatchPage/replace_text` | old found and replaced |
| `PatchPage/replace_not_found` | Tool error, content unchanged |
| `PatchPage/replace_ambiguous` | Old appears twice, tool error |
| `PatchPage/replace_lines` | Lines 5-10 replaced |
| `PatchPage/replace_lines_single` | Line 5 replaced |
| `PatchPage/replace_lines_shorter` | Fewer lines than original |
| `PatchPage/replace_lines_longer` | More lines than original |
| `PatchPage/append` | Content at end |
| `PatchPage/prepend` | Content at start of body (after heading) |
| `PatchPage/multiple_ops` | Two ops, both applied |
| `PatchPage/atomicity` | Second op fails -> first rolled back |
| `PatchPage/line_numbers_pre_op` | Two replace_lines use original line numbers |
| `PatchPage/page_not_found` | Tool error |
| `PatchPage/returns_links_to` | Updated wikilinks |
| `PatchPage/updates_index` | Search reflects changes |

### rename_page tests

| Test | Description |
|------|-------------|
| `RenamePage/renames` | Old gone, new exists |
| `RenamePage/updates_heading` | `# New Name` |
| `RenamePage/updates_wikilinks` | Other pages' `[[Old]]` -> `[[New]]` |
| `RenamePage/case_insensitive_link_update` | `[[old name]]` updated |
| `RenamePage/preserves_surrounding_text` | No corruption |
| `RenamePage/multiple_pages_updated` | 3 pages updated |
| `RenamePage/self_referential_link` | Handled correctly |
| `RenamePage/target_exists_errors` | Tool error |
| `RenamePage/source_not_found_errors` | Tool error |
| `RenamePage/returns_both_names` | `page` and `old_name` fields |
| `RenamePage/updates_index` | Searchable by new name |
| `RenamePage/case_insensitive_source` | Finds by normalized name |

### search tests

| Test | Description |
|------|-------------|
| `Search/finds_by_title` | Page name -> that page first |
| `Search/finds_by_body` | Term in body -> match |
| `Search/finds_by_wikilink` | Linked term surfaces linker |
| `Search/ranked` | Multiple matches in relevance order |
| `Search/max_results_honored` | Limits output |
| `Search/default_max_results` | Defaults to 10 |
| `Search/max_tokens_limits` | Token budget respected |
| `Search/has_snippets` | Non-empty snippets |
| `Search/has_line_numbers` | Line numbers present |
| `Search/has_linked_pages` | Linked pages with snippets |
| `Search/no_results` | Empty array |
| `Search/empty_query_errors` | Tool error |
| `Search/relevance_threshold` | Low-relevance filtered |
| `Search/reflects_writes` | Immediately searchable |
| `Search/reflects_deletes` | Gone after delete |

---

## Deliverable 7: main.go + Auto-Commit

**Creates:** `main.go`

**Depends on:** D1-D6

Thin glue: flag parsing (`--content-dir`, `--auto-commit`), Store + Index creation, Scan + Build, tool registration, `server.ServeStdio()`. No dedicated test file -- correctness covered by D5/D6 integration tests.

### Auto-commit tests (added to tools_test.go)

| Test | Description |
|------|-------------|
| `AutoCommit/write_creates_commit` | git log shows commit after write |
| `AutoCommit/delete_creates_commit` | git log shows commit after delete |
| `AutoCommit/rename_single_commit` | Multi-file rename = one commit |
| `AutoCommit/patch_creates_commit` | git log shows commit after patch |
| `AutoCommit/disabled_no_commits` | No auto-commit flag = no git ops |

---

## Verification

Each deliverable is verified by:
1. `go test ./...` -- all tests compile (and fail, since no implementation yet)
2. Implement to make tests pass
3. `go test ./... -race` -- no race conditions
4. `go vet ./...` -- no issues

End-to-end verification after D7:
1. Build binary: `go build -o memento .`
2. Create test brain dir with a few .md files
3. Run `memento --content-dir ./test-brain --auto-commit`
4. Connect via MCP client and exercise all 6 tools
5. Verify git commits are created for write operations

---

## Key Design Decisions

- **InProcessClient for tools tests** -- tests the real MCP stack (tool registration, arg parsing, JSON serialization) with zero network overhead. This is the contract test suite.
- **Page type defined in pages/, consumed by index/** -- avoids circular deps. `tools/` is the only package that wires Store + Index together.
- **No fixture files for integration tests** -- state built via `write_page` tool calls in each test. Self-contained, no hidden coupling.
- **Atomicity testing for patch_page is critical** -- must verify rollback on partial failure with a multi-op call where op N+1 fails.
