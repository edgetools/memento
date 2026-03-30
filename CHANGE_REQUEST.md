# Change Request: D1 Test Review

Findings from reviewing `pages/names_test.go` and `pages/parser_test.go`
against `MVP_PLAN.md` and `MVP_DESIGN.md`.

---

## CR-01: `long_name_truncated` and `roundtrip` are top-level functions, not subtests

**File:** `pages/names_test.go`

`TestNameToFilename_LongNameTruncated` and `TestNameToFilename_Roundtrip` are
standalone `Test*` functions instead of `t.Run(...)` subtests inside
`TestNameToFilename`. The plan names them `NameToFilename/long_name_truncated`
and `NameToFilename/roundtrip`, meaning they should live inside the parent.
As top-level functions they cannot be selected with `-run TestNameToFilename`
and their naming deviates from the plan's subtest hierarchy.

**Fix:** Move both into `TestNameToFilename` as `t.Run` cases, matching the
table-driven structure already used for the other `NameToFilename` cases.

---

## CR-02: `FilenameToName` has no subtest structure

**File:** `pages/names_test.go`

`TestFilenameToName` calls `pages.FilenameToName` directly with a single
assertion and no `t.Run`. The plan specifies it as `FilenameToName/simple`,
implying a subtest. This is inconsistent with every other test group in the
file and makes it harder to extend.

**Fix:** Wrap the single case in a `t.Run("simple", ...)` block, or expand
it into a table-driven test consistent with the rest of the file.

---

## CR-03: `long_name_truncated` checks byte length, not rune length

**File:** `pages/names_test.go`

```go
assert.LessOrEqual(t, len(result), 204)
```

`len()` returns byte count. The plan specifies "200 chars before `.md`" and
the design preserves Unicode letters. For a name composed of multi-byte runes
(e.g., `"Ü"` is 2 bytes), a 200-rune stem could produce a result whose byte
length exceeds 204. The test passes today only because the input is all-ASCII
(`strings.Repeat("a", 300)`).

**Fix:** Assert on rune length: `assert.LessOrEqual(t, len([]rune(result)), 204)`.
Also add a second case with a 300-rune Unicode name to cover the multi-byte path.

---

## CR-04: `Parse/multiline` — `Lines` assertion too weak

**File:** `pages/parser_test.go`

```go
assert.Greater(t, p.Lines, 0)
```

The plan says `multiline` should verify "correct line count". The test only
checks that `Lines` is non-zero, which any non-empty input would satisfy. The
test content has a known, countable number of lines; the assertion should be
exact.

**Fix:** Count the lines in the input string and assert equality:
```go
assert.Equal(t, strings.Count(content, "\n")+1, p.Lines)
```

---

## CR-05: `Parse/body_excludes_title` — partial content validation only

**File:** `pages/parser_test.go`

The test checks that the H1 line is absent and one specific string is present,
but does not verify the body is otherwise complete and intact. An implementation
that drops arbitrary lines would still pass this test.

**Fix:** Assert the exact expected body string rather than two `Contains`/`NotContains`
checks:
```go
assert.Equal(t, "Body text here.", p.Body)
```
