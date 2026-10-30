# Quality Gate results from superseded workflow runs

## Problem

A pull request can start multiple runs of the same workflow for one commit. GitHub can return
runs created in the same second in either order. The Quality Gate previously treated the first
returned run as current, so an older cancelled run could override its successful replacement.

Cancelled runs can also leave checks whose names still contain an unexpanded matrix expression,
such as `Acceptance Tests (${{ matrix.os }})`. Selecting the newest check by name does not remove
that placeholder when the replacement run creates checks named `Acceptance Tests (linux)` and
`Acceptance Tests (windows)`. The obsolete cancellation could therefore keep a passing PR blocked.

## Resolution

Select the current run of each workflow using its creation time and run ID, independently of
response order or pagination. When creation times are missing, use run IDs consistently for the
whole workflow group.

Use workflow check-suite IDs to identify checks belonging to superseded runs. Discard those
checks only when they are owned by GitHub Actions and their suite is conclusively obsolete.
Keep current failures and pending results, checks from other apps, and checks with unknown or
ambiguous suite IDs. Apply the same evidence to checks attached to the PR's synthetic merge commit.

Coverage requirements, review requirements, and the criteria for a successful CI result are unchanged.

## Verification

The regression reproduces both failures: an older run returned first and an obsolete matrix
placeholder retained even when the newer run is returned first. Additional cases cover pagination,
current failed and pending runs, missing timestamps, and checks that must remain visible.

Run the reporter tests with:

```shell
go test -race -tags mage ./magefiles/qualitygate
```
