# Release previews

Runs `release:previews` to create and refresh one bot-authored informational issue
per stable versioned milestone. PR links, merge states and fallback summaries are
refreshed from GitHub. Feature and nightly releases do not create issues.

Use the workflow's GitHub Actions token (issues: write, pull-requests: read).
The marker associates the preview with its milestone; it is never authorization.
Only issues authored by github-actions[bot] are updated. Duplicate markers on
bot issues fail reconciliation. Open previews are pinned when a slot is available;
closed previews are unpinned. Existing unrelated pins are never removed.

Tests: `go test -tags=mage ./magefiles/release/...`.
