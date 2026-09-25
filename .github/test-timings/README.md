# Race test timing hints

`toolchain.json` contains top-level test durations from the linked hosted race
run. The source revision identifies the measurement, not the revision to execute.
Workers discover their own tests with `go test -race -list .`; this file only
influences assignment. New tests receive a one-second estimate, deleted tests are
ignored, and equal weights use stable test-name ordering. Missing timing data
falls back to equal weights. Malformed JSON fails with an explicit error.

Each race worker uploads `race-timings-N` for seven days:

- `plan-N.json`: full inventory, selected names, and discovery/compilation time.
- `toolchain.jsonl` and `packages.jsonl`: streamed Go JSON test events.
- `toolchain.json` and `packages.json`: top-level test and package durations,
  plus command wall time. Nested subtests are not added a second time.

To refresh, download all four artifacts from one successful run of the same
revision and runner class. Verify identical discovered inventories and that the
selected sets cover the inventory exactly once. Combine the toolchain summaries'
`tests["github.com/cloudposse/atmos/pkg/toolchain"]` maps into `seconds`, recording
the new source run URL and revision. Review the change like any other CI input;
never let downloaded timing artifacts determine which tests exist or which code
runs. Compare several runs before using noisy network timings as new weights.

The initial hints predate the local installation fixtures in this follow-up.
Those entries deliberately remain conservative until hosted measurements are
available; they cannot omit tests or change their assertions.
