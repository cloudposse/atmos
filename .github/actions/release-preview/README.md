# Release previews

Runs `release:previews` to create and refresh one bot-authored informational issue
per stable versioned milestone. PR links and merge states are refreshed from
GitHub, with descriptions condensed by the same AI summarizer used for final
release notes. Feature and nightly releases do not create issues.

Here, a preview is a draft release-notes issue, not a website deployment.
The workflow uses the dedicated `release-notes` GitHub environment for credentials.
Configure `OPENAI_API_KEY` there; do not add release-signing or publishing secrets.
GitHub issue updates use the job's built-in token with the declared permissions.
Pass `openai-api-key` to enable AI summaries (`OPENAI_API_KEY` for direct Mage
invocations); `OPENAI_MODEL` optionally overrides the shared release summary model.
Without a key, previews use CodeRabbit summaries or truncated descriptions.
Summaries are rendered before checking the issue body limit; model failures or
previews that still exceed the limit leave the existing issue untouched.

Unchanged previews reuse the saved body without calling the model. A hidden
fingerprint covers PR content, merge states, categories, milestone details, and
summary mode/model; it never contains the API key. Changed inputs or edits to
the saved body trigger regeneration.

Use the workflow's GitHub Actions token (issues: write, pull-requests: read).
The marker associates the preview with its milestone; it is never authorization.
Only issues authored by github-actions[bot] are updated. Duplicate markers on
bot issues fail reconciliation. Open previews are pinned when a slot is available;
closed previews are unpinned. Existing unrelated pins are never removed.

Tests: `go test -tags=mage ./magefiles/release/...`.
