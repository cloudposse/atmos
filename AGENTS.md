# Codex Agent Guidelines

This repository is a Golang CLI project.

## Developer workflow
- Format code with:

```sh
gofumpt -w . && goimports -w .
```
- Run `go mod tidy`, `make lint`, and `make testacc-cover` before committing.
- Update `docs/` and `README.md` when CLI behaviour changes.

## Pull requests
- Keep PRs focused on a single logical change.
- Follow `.github/PULL_REQUEST_TEMPLATE.md` for PR descriptions.

## Testing
- Always run `go test ./...` after changes. If the command fails due to network restrictions, note that in the PR description.

## Spelling corrections
- **Do not modify files under `tests/test-cases/` or `tests/testdata/` unless explicitly instructed.** These files contain golden snapshots and their content is sensitive to even minor changes.
