# MVP Implementation Checklist

## D1: Scaffolding + Pages Names & Parser

- [x] Create `go.mod` (`github.com/edgetools/memento`)
- [x] Add dependencies: `github.com/mark3labs/mcp-go`, `github.com/stretchr/testify`
- [x] Write `pages/names_test.go` (failing)
- [x] Write `pages/parser_test.go` (failing)
- [x] Create `testdata/*.md` fixtures
- [x] Implement `pages/names.go` (`Normalize`, `NameToFilename`, `FilenameToName`, `NamesMatch`)
- [x] Implement `pages/parser.go` (`Page` type, `Parse`)
- [x] `go test ./pages/...` — all pass
- [x] `go vet ./pages/...` — clean

---

## D2: Pages Store

- [x] Write `pages/store_test.go` (failing)
- [x] Implement `pages/store.go` (`NewStore`, `Write`, `Load`, `Delete`, `Rename`, `Scan`, `Exists`)
- [x] `go test ./pages/...` — all pass
- [x] `go vet ./pages/...` — clean

---

## D3: Index BM25 + Trigram *(parallel with D2)*

- [x] Write `index/bm25_test.go` (failing)
- [x] Write `index/trigram_test.go` (failing)
- [x] Implement `index/bm25.go`
- [x] Implement `index/trigram.go`
- [x] `go test ./index/...` — all pass
- [x] `go vet ./index/...` — clean

---

## D4: Index Graph + Composite

- [x] Write `index/graph_test.go` (failing)
- [x] Write `index/index_test.go` (failing)
- [x] Implement `index/graph.go`
- [x] Implement `index/index.go`
- [x] `go test ./index/...` — all pass
- [x] `go vet ./index/...` — clean

---

## D5: Tools — write_page, get_page, delete_page

- [x] Write `tools/tools_test.go` with helpers (`setupTestServer`, `callTool`, etc.) (failing)
- [x] Write `write_page` tests (failing)
- [x] Write `get_page` tests (failing)
- [x] Write `delete_page` tests (failing)
- [x] Implement `tools/tools.go` (shared wiring)
- [x] Implement `tools/write_page.go`
- [x] Implement `tools/get_page.go`
- [x] Implement `tools/delete_page.go`
- [x] `go test ./tools/...` — all pass
- [x] `go vet ./tools/...` — clean

---

## D6: Tools — patch_page, rename_page, search

- [x] Write `patch_page` tests (failing)
- [x] Write `rename_page` tests (failing)
- [x] Write `search` tests (failing)
- [x] Implement `tools/patch_page.go`
- [x] Implement `tools/rename_page.go`
- [x] Implement `tools/search.go`
- [x] `go test ./tools/...` — all pass
- [x] `go vet ./tools/...` — clean

---

## D7: main.go + Auto-Commit

- [x] Write auto-commit tests in `tools/autocommit_test.go` (failing — `tools.RegisterAutoCommit` undefined)
- [x] Implement `main.go` (flag parsing, store+index wiring, `server.ServeStdio`)
- [x] `go test ./...` — all pass
- [x] `go vet ./...` — clean
- [x] Build binary: `go build -o memento .`
- [ ] End-to-end smoke test with MCP client
- [ ] Verify git commits created for write operations
