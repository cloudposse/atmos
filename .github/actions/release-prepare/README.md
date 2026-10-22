# Prepare tagged release

Calls `release:prepare` from trusted checked-out tooling. The Mage operation
revalidates the actor, milestone, labels, source inclusion and expected version,
then creates the tag or verifies the existing tag resolves to the same full SHA.
It writes `$RUNNER_TEMP/release-plan.json` and outputs `tag`, `commit`, `milestone`.
No draft or assets are created here.

Manual `cut`/`candidate` inputs require milestone, version and commit. Nightly
and feature requests have dedicated trusted workflow triggers. `pr` selects a
feature source; feature requests do not create preview issues. The preparation
token needs contents write, issues read and pull requests read. Call only in the
protected release environment, never on a runner that executes build sources.

Tests: `go test -tags=mage ./magefiles/release` (plan/lifecycle suites).

Preview issues belong only to stable release milestones. Nightly and feature
releases do not create issues.
