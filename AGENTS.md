# Repo guidelines for Codex agents

## Testing
- Always run `go test ./...` after changes. If the command fails due to network restrictions, note that in the PR description.

## Spelling corrections
- **Do not modify files under `tests/test-cases/` or `tests/testdata/` unless explicitly instructed.** These files contain golden snapshots and their content is sensitive to even minor changes.

