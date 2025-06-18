# Codex Agent Guidelines

This repository is a Golang CLI project.

## Developer workflow
- Format code with `gofumpt` and `goimports`.
- Run `go mod tidy`, `make lint`, and `make testacc-cover` before committing.
- Update `docs/` and `README.md` when CLI behaviour changes.

## Pull requests
- Keep PRs focused on a single logical change.
- Follow `.github/PULL_REQUEST_TEMPLATE.md` for PR descriptions.
