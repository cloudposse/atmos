# Mage targets

Atmos keeps repository automation in Go-based [Mage](https://magefile.org/)
targets. Run targets from the repository root with the repository-pinned Mage
version:

```console
go tool mage -l
go tool mage <target> [arguments]
```

The target list and per-target usage are generated from the Go declarations:

```console
go tool mage -h <target>
```

## Target catalog

| Target | Arguments | Purpose |
| --- | --- | --- |
| `acceptance:precompile` | `<targetValue> [-outputDir=<string>]` | Precompile the acceptance-test binaries for one target. The output directory defaults to `build`. |
| `acceptance:run` | `<modeValue> <targetValue>` | Run one acceptance-test shard selected by `ATMOS_TEST_SHARD` and `ATMOS_TEST_SHARD_COUNT`. |
| `acceptance:verify` | `<targetValue> <shardCount> [-binaryDir=<string>]` | Verify that the acceptance workflow matrix and package assignments are complete and unique. The binary directory defaults to `build`. |
| `build:binary` | `<target> <version>` | Build Atmos for `default`, `linux`, `windows`, `macos`, or `macos-intel` and stamp the version and Git commit. Empty values default to `default` and `test`. |
| `ci:checkshardresults` | `<repo> <runID> <runAttempt> <check>` | Wait for all acceptance shards behind a required check and fail unless all succeed. |
| `ci:classifyinfrafailures` | `<repo> <runID> <runAttempt>` | Classify unsuccessful GitHub Actions jobs as infrastructure failures or real failures. |
| `ci:reruninfrafailures` | `<repo> <runID> <event> <headSHA> <prNumbers>` | Rerun failed or cancelled jobs after verifying that a pull request run is still current. |
| `coverage:collect` | none | Run the packages in `TEST` with native subprocess coverage and merge the counters. |
| `coverage:merge` | `<inputs>` | Merge a comma-separated list of native Go coverage directories. |
| `coverage:mergeshards` | none | Merge all downloaded Linux coverage shards. |
| `lint:changed` | none | Run the complete changed-files lint chain: module validation, custom golangci-lint build, and pre-commit lint. This is the default target. |
| `lint:customgcl` | none | Build the custom golangci-lint binary when it is missing or stale. |
| `lint:gomodcheck` | none | Reject root-module `replace` and `exclude` directives that would break consumer installs. |
| `lint:lintroller` | none | Build the Atmos lintroller when stale and analyze all non-testdata packages. |
| `lint:precommit` | none | Run the prebuilt custom golangci-lint binary against files changed from `origin/main`. |
| `notice:generate` | none | Regenerate `NOTICE` from Go dependency licenses. |
| `release:summarizenotes` | `<repo> <releaseID>` | Condense a draft GitHub release's pull-request notes, optionally with OpenAI summaries. |
| `s3:deploy` | `<localDir> <s3URI>` | Deploy a static site with explicit content metadata and content-hash incrementality. Unchanged deployments make no S3 write requests. |
| `test:race` | none | Run all non-acceptance packages with the race detector and shuffled test order. |
| `test:racematrix` | none | Emit the GitHub Actions matrix that partitions race-test packages across shards. |

## Environment variables

Only variables that change target behavior are listed here. Standard Go and
GitHub Actions variables continue to work normally.

| Variable | Targets | Behavior |
| --- | --- | --- |
| `ATMOS_TEST_SHARD`, `ATMOS_TEST_SHARD_COUNT` | `acceptance:run` | Select the one-based acceptance shard and total shard count. `TEST_SHARD_COUNT` is the count fallback. |
| `TEST`, `TESTARGS` | `coverage:collect`, `test:race` | Override the package list and additional `go test` arguments. |
| `COVERAGE_DIR`, `COVERAGE_DATA_OUT`, `COVERAGE_OUT`, `COVERAGE_SHARDS_DIR`, `GO_TEST_TIMEOUT` | `coverage:*` | Override coverage input, intermediate, output, and timeout paths. |
| `TEST_SHARD_COUNT` | `coverage:mergeshards`, `ci:checkshardresults` | Set the expected acceptance-shard count. |
| `ATMOS_BUILD_NO_CACHE` | `build:binary` | Set to `true` to force package recompilation without deleting the shared Go cache. |
| `GITHUB_OUTPUT`, `GITHUB_STEP_SUMMARY` | `ci:*`, `test:racematrix` | Write structured workflow outputs and summaries. |
| `ATMOS_CI_STUCK_GAP` | `ci:classifyinfrafailures` | Override the infrastructure-zombie detection interval with a positive Go duration such as `300s`. |
| `GITHUB_TOKEN` | `ci:*`, `release:summarizenotes` | Authenticate GitHub API requests. |
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `RELEASE_NOTES_DRY_RUN` | `release:summarizenotes` | Enable model summaries, select a model, and preview without updating the release. |
| `PROTECTED_PATTERNS` | `s3:deploy` | Newline-separated glob patterns that must not be deleted from the destination. |
| `RACE_SHARD_COUNT`, `ATMOS_TEST_RACE_SHARD_SEED` | `test:racematrix` | Set the number of race shards and deterministic shuffle seed. |
| `ATMOS_TEST_RACE_PARALLEL` | `test:race` | Override per-package test parallelism; defaults to `4`. |

## S3 deployment behavior

`s3:deploy` stores `.cloudposse-deploy-manifest-v1.json` in the destination.
The manifest records each managed object's SHA-256 digest, size, and content
type. On later runs, the target uploads only added or content/metadata-changed
files and deletes only removed managed files. Files matched by
`PROTECTED_PATTERNS` are never deleted.

The first deployment uploads every managed file once with explicit metadata,
then lists the destination and deletes stale, unprotected remote objects.
Browser-significant types are selected by extension; the existing `mimetype`
magic-number library provides the fallback for unknown extensions. Textual
types receive explicit UTF-8 metadata, such as `text/html; charset=utf-8`. The
manifest is uploaded only after the deployment succeeds, so a failed run cannot
record an incomplete state as current.

`aws s3 sync` cannot append `charset=utf-8` selectively while preserving each
file's distinct MIME type: `--content-type` supplies one value for the entire
invocation. The Mage target computes and sends the exact header for each object,
so correct browser metadata does not require a second S3-to-S3 copy pass.

All S3 operations use the repository's existing AWS SDK for Go v2 dependency.
The target does not shell out to the AWS CLI or create temporary request files.
SDK calls preserve typed request metadata, built-in retries, context
cancellation, and per-object `DeleteObjects` error handling.

```console
PROTECTED_PATTERNS=$'previews/**\nrobots.txt' \
  go tool mage s3:deploy ./dist s3://example-docs/site
```
