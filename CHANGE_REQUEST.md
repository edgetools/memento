# Change Request: patch_page Create-on-Write for Append/Prepend

## Problem

The current implementation of `patch_page` requires the target page to already
exist for all operations. This creates friction in the most common write workflow:
jotting down a note about a concept during a planning session.

Today, an agent that wants to append a thought to a page must:

1. Search for the page to see if it exists
2. If it exists: call `patch_page` with `append`
3. If it doesn't exist: call `write_page` to create it, then optionally
   `patch_page` to add more

That's 2-3 tool calls and a conditional decision for what should be a single
"write this thought down" action. This discourages frequent, lightweight writes
and pushes agents toward batching everything into end-of-session dumps, which
produces lower-quality knowledge capture.

## Change

Modify `patch_page` so that `append` and `prepend` operations create the page
if it doesn't exist. The MCP generates the page heading from the page name
automatically, just as `write_page` does.

`replace` and `replace_lines` operations continue to require the page to exist,
since they reference specific content that must be present to match against.

### Behavior Before

```
patch_page("Aggro Mechanics", [append: "New note about aggro"])
→ Error: page "Aggro Mechanics" not found
```

### Behavior After

```
patch_page("Aggro Mechanics", [append: "New note about aggro"])
→ Creates "Aggro Mechanics" page with heading + appended content
→ Returns { page: "Aggro Mechanics", links_to: [...] }
```

If the page already exists, behavior is unchanged: the content is appended to
the end.

### Mixed Operations

If a `patch_page` call contains both create-capable operations (`append`,
`prepend`) and existence-dependent operations (`replace`, `replace_lines`) on
a page that doesn't exist, the call fails. The existence-dependent operations
cannot be applied, and atomicity requires that no operations are applied if any
fail.

In practice, agents won't mix these in a create scenario. The jotting workflow
is a single `append`. Mixed operations are for editing existing pages.

## Files Affected

- `pages/store.go` — page existence check in the write path needs to distinguish
  between operation types. `append` and `prepend` should call through to the
  create path (with auto-generated heading) when the page doesn't exist.
- `tools/patch_page.go` — validation logic needs to check whether all operations
  in the call are create-capable before failing on a missing page. If only
  `append`/`prepend` operations are present and the page doesn't exist, create it
  first, then apply operations.
- `index/index.go` — no changes needed; the index update path already handles
  new pages since `write_page` uses it.

## Tests

- `append` on a nonexistent page creates the page with heading + content
- `prepend` on a nonexistent page creates the page with heading + content
- `replace` on a nonexistent page returns an error
- `replace_lines` on a nonexistent page returns an error
- Mixed `append` + `replace` on a nonexistent page returns an error (atomic
  failure)
- `append` on an existing page still appends (no behavior change)
- `prepend` on an existing page still prepends (no behavior change)
- Created page appears in search index immediately after the call
- Created page is committed if `--auto-commit` is enabled
- Created page's `[[wikilinks]]` in the appended content are parsed into the
  link graph
