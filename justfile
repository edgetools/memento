set windows-shell := ["pwsh.exe", "-NoLogo", "-Command"]

test:
  go test ./...

vet:
  go vet ./...

fmt:
  go fmt ./...

write-tests PROMPT:
  claude --agent test-writer "{{PROMPT}}"

review-tests PROMPT:
  claude --agent test-reviewer "{{PROMPT}}"

write-code PROMPT:
  claude --agent engineer "{{PROMPT}}"
