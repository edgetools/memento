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

- [ ] Write `pages/store_test.go` (failing)
- [ ] Implement `pages/store.go` (`NewStore`, `Write`, `Load`, `Delete`, `Rename`, `Scan`, `Exists`)
- [ ] `go test ./pages/...` — all pass
- [ ] `go vet ./pages/...` — clean

---

## D3: Index BM25 + Trigram *(parallel with D2)*

- [ ] Write `index/bm25_test.go` (failing)
- [ ] Write `index/trigram_test.go` (failing)
- [ ] Implement `index/bm25.go`
- [ ] Implement `index/trigram.go`
- [ ] `go test ./index/...` — all pass
- [ ] `go vet ./index/...` — clean

---

## D4: Index Graph + Composite

- [ ] Write `index/graph_test.go` (failing)
- [ ] Write `index/index_test.go` (failing)
- [ ] Implement `index/graph.go`
- [ ] Implement `index/index.go`
- [ ] `go test ./index/...` — all pass
- [ ] `go vet ./index/...` — clean

---

## D5: Tools — write_page, get_page, delete_page

- [ ] Write `tools/tools_test.go` with helpers (`setupTestServer`, `callTool`, etc.) (failing)
- [ ] Write `write_page` tests (failing)
- [ ] Write `get_page` tests (failing)
- [ ] Write `delete_page` tests (failing)
- [ ] Implement `tools/tools.go` (shared wiring)
- [ ] Implement `tools/write_page.go`
- [ ] Implement `tools/get_page.go`
- [ ] Implement `tools/delete_page.go`
- [ ] `go test ./tools/...` — all pass
- [ ] `go vet ./tools/...` — clean

---

## D6: Tools — patch_page, rename_page, search

- [ ] Write `patch_page` tests (failing)
- [ ] Write `rename_page` tests (failing)
- [ ] Write `search` tests (failing)
- [ ] Implement `tools/patch_page.go`
- [ ] Implement `tools/rename_page.go`
- [ ] Implement `tools/search.go`
- [ ] `go test ./tools/...` — all pass
- [ ] `go vet ./tools/...` — clean

---

## D7: main.go + Auto-Commit

- [ ] Write auto-commit tests in `tools/tools_test.go` (failing)
- [ ] Implement `main.go` (flag parsing, store+index wiring, `server.ServeStdio`)
- [ ] `go test ./...` — all pass
- [ ] `go vet ./...` — clean
- [ ] Build binary: `go build -o memento .`
- [ ] End-to-end smoke test with MCP client
- [ ] Verify git commits created for write operations
