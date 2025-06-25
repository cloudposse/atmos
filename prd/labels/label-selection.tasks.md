# Label-Based Selection Feature — Implementation Tasks

## 1. Implementation

1. [x] Add `--selector` (`-l`) flag to each relevant `list` and `describe` command.
   - [x] Add `--selector` flag to `list stacks` command
   - [x] Add `--selector` flag to `list components` command
   - [x] Add `--selector` flag to `list values` command
   - [x] Add `--selector` flag to `list vars` command
   - [x] Add `--selector` flag to `describe component` command
   - [x] Add `--selector` flag to `describe stacks` command
   - [x] Add `--selector` flag to `describe affected` command
   - [x] Add `--selector` flag to `list settings` command
   - [x] Add `--selector` flag to `list metadata` command
2. [x] Introduce a label selector parser supporting:
   - [x] Equality (`key=value`)
   - [x] Inequality (`key!=value`)
   - [x] Set inclusion (`key in (a,b)`)
   - [x] Set exclusion (`key notin (a,b)`)
   - [x] Existence (`key`)
   - [x] Non-existence (`!key`)
3. [x] Extend data models to read `metadata.labels` in stacks and components.
4. [x] Implement component-level override and stack-label inheritance for selector evaluation.
5. [x] Integrate selector filtering with existing flags (`--stack`, `--component`, `--type`), ensuring all filters are additive.
   - [x] Implement selector filtering for `list stacks` command
   - [x] Implement selector filtering for `list components` command
   - [x] Implement selector filtering for `list values` command
   - [x] Implement selector filtering for `list vars` command
   - [x] Implement selector filtering for `describe component` command
   - [x] Implement selector filtering for `describe stacks` command
   - [x] Implement selector filtering for `describe affected` command
   - [x] Implement selector filtering for `list settings` command
   - [x] Implement selector filtering for `list metadata` command
6. [x] Provide clear error messaging:
   - [x] No matches → "No resources matched selector." (exit 0)
   - [x] Invalid selector syntax → syntax error (non-zero exit)
7. [x] Update JSON schemas for stacks and components to explicitly allow `metadata.labels` as an object with string values, while still permitting additional properties under `metadata`. Modified all schema files: `pkg/datafetcher/schema/stacks/stack-config/1.0.json`, `pkg/datafetcher/schema/config/global/1.0.json`, `pkg/datafetcher/schema/atmos/manifest/1.0.json`, `website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`, and example schemas.
8. [x] Update test fixtures in `tests/fixtures/scenarios/selector/` with comprehensive labeled stack examples including stack-level and component-level labels for testing all selector operators.

## 2. Testing

1. [x] Unit tests for each selector operator ( =, !=, in, notin, key, !key ).
2. [x] Tests for combined selector expressions.
3. [x] Tests validating syntax error handling for malformed selectors.
4. [x] Tests for zero-result scenarios return correct message & exit code. Verified "No resources matched selector." message with exit code 0.
5. [x] Interaction tests combining `--selector` with `--stack`, `--component`, and `--type` flags. Selector filtering works additively with existing flags.
6. [x] Schema validation tests ensuring new JSON schema changes accept valid labels and reject invalid structures. Updated schemas properly validate `metadata.labels` as string key-value pairs.
7. [x] Inheritance tests confirming component overrides stack labels appropriately. Component-level labels properly override stack-level labels using `MergedLabels()` function.
8. [x] Command-specific tests covering:
   - [x] `list vars` - selector filtering implemented
   - [x] `list values` - selector filtering implemented
   - [x] `list components` - selector filtering implemented
   - [x] `list stacks` - selector filtering implemented
   - [x] `describe component` - selector filtering implemented
   - [x] `describe stacks` - selector filtering implemented
   - [x] `describe affected` - selector filtering implemented
   - [x] `list settings` - selector filtering implemented
   - [x] `list metadata` - selector filtering implemented
9. [x] CLI integration tests in `tests/test-cases/selector-cli.yaml` with comprehensive test scenarios covering all selector operators and commands.
   - [x] Added test cases for `describe affected` command (enabled and ready for testing)
   - [x] Added test cases for `list settings` command (enabled and ready for testing)
   - [x] Added test cases for `list metadata` command (enabled and ready for testing)

## 3. Documentation

1. [x] Update CLI docs for all affected commands with `--selector` usage.
2. [x] Document `metadata.labels` schema and inheritance semantics.
3. [x] Add selector syntax reference (link to Kubernetes docs).
4. [x] Provide illustrative examples for common scenarios.
5. [x] Ensure website pages under `https://atmos.tools` are updated accordingly.
