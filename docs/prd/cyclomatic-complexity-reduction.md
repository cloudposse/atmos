# PRD: Cyclomatic Complexity Reduction

## Goal

Reduce cyclomatic complexity across all Go functions so that every branch is
independently testable, enabling unit-test coverage well above 80% without
requiring real infrastructure (Terraform state, AWS, Docker, TTY, filesystem).

---

## Problem Statement

High cyclomatic complexity is the single largest obstacle to unit testing Atmos.
A function with cyclomatic complexity N requires at least N test cases to reach
full branch coverage. Functions with complexity above 15 are difficult to reason
about, difficult to mock, and produce deeply nested if/else trees that resist
table-driven test patterns.

Several functions were historically built as monoliths:

| Function | Before refactor | After refactor |
|---|---|---|
| `ExecuteTerraform` | ~160 | ~26 |
| `ExecuteDescribeStacks` | ~247 | ~10 |
| `processArgsAndFlags` | ~67 | ~15 |

These successes prove the approach works. This PRD standardises and scales it.

---

## Current State

### Linter thresholds (`.golangci.yml`)

| Linter | Setting | Severity |
|---|---|---|
| `cyclop` | `max-complexity: 15` | error |
| `gocognit` | `min-complexity: 20` | warning |
| `revive cyclomatic` | `10` | warning |
| `revive cognitive-complexity` | `25` | warning |
| `nestif` | `min-complexity: 4` | warning |
| `revive function-length` | 50 stmts / 60 lines | warning |
| `funlen` | 60 lines / 40 stmts | error |

Complexity violations that are classified as **warnings** are invisible in CI
when `--severity error` is used, so they silently accumulate. The gap between
`cyclop` (enforced at 15) and `revive cyclomatic` (warned at 10) creates
contradictory guidance.

### Measurement baseline

Run the following to produce a per-function complexity report:

```bash
# Install gocyclo
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

# Report all functions with cyclomatic complexity >= 10, sorted descending
gocyclo -over 9 -avg . | sort -rn | head -60
```

Run the following for a cognitive-complexity baseline:

```bash
# gocognit is already used by golangci-lint
go install github.com/uudashr/gocognit/cmd/gocognit@latest
gocognit -over 9 . | sort -rn | head -60
```

Capture these baselines in a tracking file (see [Progress Tracking](#progress-tracking)).

---

## Refactoring Techniques

### 1. Extract helper functions

Break a large function into a coordinator that calls focused helpers. Each
helper has a single, clearly-named responsibility. This is the pattern used for
`ExecuteTerraform` → `terraform_execute_helpers.go`.

**Before:**

```go
func ExecuteTerraform(info schema.ConfigAndStacksInfo) error {
    // 160 lines, cyclomatic ~160
    switch info.SubCommand {
    case "plan":
        args = append(args, "-out="+planfile)
        if info.PlanFile != "" {
            // ...
        }
    case "apply":
        if planFile != "" {
            args = append(args, planFile)
        } else {
            // ...
        }
    // ... 20 more cases
    }
}
```

**After:**

```go
// Coordinator – cyclomatic ≤ 5.
func ExecuteTerraform(info schema.ConfigAndStacksInfo) error {
    defer perf.Track(nil, "exec.ExecuteTerraform")()

    args, err := buildSubcommandArgs(info)
    if err != nil {
        return err
    }
    return runTerraform(info, args)
}

// Focused helper – cyclomatic ≤ 8.
func buildSubcommandArgs(info schema.ConfigAndStacksInfo) ([]string, error) {
    builders := map[string]func(schema.ConfigAndStacksInfo) ([]string, error){
        "plan":      buildPlanSubcommandArgs,
        "apply":     buildApplySubcommandArgs,
        "init":      buildInitSubcommandArgs,
        "workspace": buildWorkspaceSubcommandArgs,
    }
    builder, ok := builders[info.SubCommand]
    if !ok {
        return nil, fmt.Errorf("%w: %s", errUtils.ErrTerraformSubcommand, info.SubCommand)
    }
    return builder(info)
}
```

### 2. Replace switch/if chains with dispatch tables

A `switch` with N cases has cyclomatic complexity N. Replace it with a
`map[string]func(...)` lookup. The dispatcher itself stays at complexity 2
(map lookup + error check).

```go
// HIGH COMPLEXITY – cyclomatic = number of cases.
switch flagName {
case "--stack":    value = info.Stack
case "--component": value = info.Component
// ... 15 more cases
}

// LOW COMPLEXITY – cyclomatic = 2 regardless of table size.
var flagValueMap = map[string]func(schema.ConfigAndStacksInfo) string{
    "--stack":     func(i schema.ConfigAndStacksInfo) string { return i.Stack },
    "--component": func(i schema.ConfigAndStacksInfo) string { return i.Component },
    // ... 15 more entries, zero added complexity
}

fn, ok := flagValueMap[flagName]
if !ok {
    return "", fmt.Errorf("%w: %s", errUtils.ErrUnknownFlag, flagName)
}
return fn(info), nil
```

### 3. Early return / guard clauses

Eliminate else branches after return statements. Each eliminated else reduces
nesting and cognitive complexity.

```go
// BEFORE – nesting level 3, nestif fires.
func process(x *Foo) error {
    if x != nil {
        if x.Enabled {
            if err := x.Run(); err != nil {
                return err
            }
            return nil
        }
    }
    return errUtils.ErrFooDisabled
}

// AFTER – nesting level 1, nestif silent.
func process(x *Foo) error {
    if x == nil || !x.Enabled {
        return errUtils.ErrFooDisabled
    }
    return x.Run()
}
```

### 4. Table-driven dispatch for multi-type handling

Replace large `if typeA … else if typeB` chains with a per-type options struct
indexed by a constant. Used in `describe_stacks_component_processor.go`.

```go
type processComponentTypeOpts struct {
    includeWorkspace  bool
    inheritMetadata   bool
    includeIfEmpty    bool
}

var componentTypeOpts = map[string]processComponentTypeOpts{
    "terraform":  {includeWorkspace: true, inheritMetadata: true},
    "helmfile":   {},
    "packer":     {},
    "ansible":    {},
}

opts, ok := componentTypeOpts[componentType]
if !ok {
    return fmt.Errorf("%w: %s", errUtils.ErrUnknownComponentType, componentType)
}
```

### 5. Error-early, value-late

Return errors at the earliest opportunity. Accumulate data only when no error
path is possible.

```go
// Avoids deeply nested success path.
cfg, err := loadConfig()
if err != nil {
    return err
}
stack, err := resolveStack(cfg)
if err != nil {
    return err
}
return applyStack(stack)
```

---

## Enforcement Strategy

### New code – strict thresholds (errors)

All new code must pass tighter limits. Promote existing warnings to errors and
lower thresholds in a phased schedule:

| Phase | Timeline | `cyclop` | `revive cyclomatic` | `gocognit` | `revive cognitive` | `nestif` | Severity |
|---|---|---|---|---|---|---|---|
| Current | Now | 15 | 10 warn | 20 warn | 25 warn | 4 warn | mixed |
| Phase 1 | Month 1–2 | **12** | **10 error** | **15 warn** | **20 warn** | 4 warn | tighter |
| Phase 2 | Month 3–4 | **10** | **8 error** | **12 error** | **15 warn** | **5 error** | stricter |
| Phase 3 | Month 5–6 | **8** | **7 error** | **10 error** | **12 error** | **5 error** | strict |

**Phase 1 `.golangci.yml` diff:**

```yaml
  settings:
    cyclop:
-     max-complexity: 15
+     max-complexity: 12
    gocognit:
-     min-complexity: 20
+     min-complexity: 15
    revive:
      rules:
        - name: cyclomatic
          arguments:
-           - 10
+           - 10
        - name: cognitive-complexity
          arguments:
-           - 25
+           - 20

  severity:
    rules:
      - linters:
          - revive
-       text: "cognitive-complexity|cyclomatic|function-length|function-result-limit|comment-spacings"
+       text: "cognitive-complexity|function-length|function-result-limit|comment-spacings"
        severity: warning
+     - linters:
+         - revive
+       text: "cyclomatic"
+       severity: error
      - linters:
          - nestif
          - nolintlint
          - gocognit
        severity: warning
```

### Old code – nolint budget with tracking

Functions that already exceed the new threshold may be temporarily silenced with
a `//nolint` directive **only if they are registered in the complexity budget
file**.

Create `docs/complexity-budget.yml`:

```yaml
# Complexity Budget
# Each entry documents a temporarily exempted function.
# Target: zero entries by end of Phase 3.
# Owner: Platform Engineering
# Updated: (date of last review)

entries:
  - file: internal/exec/terraform.go
    function: ExecuteTerraform
    cyclomatic: 26
    cognitive: 31
    target_quarter: Q3-2026
    owner: "@aknysh"
    issue: "#1234"

  - file: internal/exec/describe_stacks.go
    function: ExecuteDescribeStacks
    cyclomatic: 10
    cognitive: 14
    target_quarter: DONE
    owner: "@osterman"
    issue: "#1201"
```

CI rejects any `//nolint:cyclop,revive,gocognit` comment that is **not** in
`docs/complexity-budget.yml`. A shell script in `.github/scripts/` validates
this by grep-comparing nolint annotations against the budget file entries.

**`.github/scripts/check-complexity-budget.sh`:**

```bash
#!/usr/bin/env bash
# Verify that every //nolint complexity directive is registered in the budget.
set -euo pipefail

BUDGET="docs/complexity-budget.yml"
VIOLATIONS=0

while IFS= read -r file; do
    while IFS= read -r line_content; do
        # Extract the file:function from the nolint annotation line
        annotation_file=$(echo "$line_content" | awk -F: '{print $1}')
        if ! grep -q "file: $annotation_file" "$BUDGET"; then
            echo "ERROR: Unregistered nolint complexity in $annotation_file"
            VIOLATIONS=$((VIOLATIONS + 1))
        fi
    done < <(grep -n "//nolint:.*\(cyclop\|gocognit\|revive\)" "$file" || true)
done < <(git diff --name-only HEAD~1 HEAD -- '*.go' || git ls-files '*.go')

if [ "$VIOLATIONS" -gt 0 ]; then
    echo "Found $VIOLATIONS unregistered complexity exemptions."
    echo "Add them to $BUDGET or refactor the function."
    exit 1
fi
echo "Complexity budget check passed."
```

---

## Tools

### Static analysis (already configured)

| Tool | Purpose | Config |
|---|---|---|
| `cyclop` | Cyclomatic complexity gate | `.golangci.yml` → `cyclop.max-complexity` |
| `gocognit` | Cognitive complexity gate | `.golangci.yml` → `gocognit.min-complexity` |
| `revive cyclomatic` | Per-function cyclomatic report | `.golangci.yml` → `revive.rules.cyclomatic` |
| `revive cognitive-complexity` | Per-function cognitive report | `.golangci.yml` → `revive.rules.cognitive-complexity` |
| `nestif` | Nested-if depth gate | `.golangci.yml` → `nestif.min-complexity` |
| `revive function-length` | Lines-per-function gate | `.golangci.yml` → `revive.rules.function-length` |
| `funlen` | Statement/line count gate | `.golangci.yml` → `funlen` |

### Standalone measurement tools

```bash
# Per-function cyclomatic complexity (sorted descending)
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
gocyclo -over 9 -avg . 2>/dev/null | sort -rn

# Per-function cognitive complexity (sorted descending)
go install github.com/uudashr/gocognit/cmd/gocognit@latest
gocognit -over 9 . 2>/dev/null | sort -rn

# Full lint run (includes all linters above)
make lint

# Coverage (requires build tag)
make testacc-cover
```

### New vs. old code differentiation

`golangci-lint` supports `new-from-rev` to lint only lines changed since a
given commit. Use this in PR CI to enforce the new threshold only on changed
lines, without blocking existing violations:

```yaml
# .github/workflows/lint.yml (excerpt)
- name: Lint new code only
  run: |
    golangci-lint run \
      --new-from-rev=origin/main \
      --severity error \
      ./...
```

The full-repo lint job (nightly or on main merge) runs without `--new-from-rev`
to surface existing violations and track overall progress.

---

## Progress Tracking

### Baseline snapshot

Create `docs/complexity-baseline.txt` at the start of each phase by running:

```bash
gocyclo -over 0 -avg . 2>/dev/null \
  | awk '{print $1, $3}' \
  | sort -rn \
  > docs/complexity-baseline.txt
```

Commit this file so progress is visible in git history.

### Complexity trend in CI

Add a GitHub Actions step to the nightly workflow that:

1. Generates the current complexity report.
2. Computes the number of functions above each threshold.
3. Posts a summary to `$GITHUB_STEP_SUMMARY`.

```yaml
# .github/workflows/complexity-trend.yml
name: Complexity Trend
on:
  schedule:
    - cron: "0 6 * * 1"  # Mondays at 06:00 UTC
  workflow_dispatch:

jobs:
  complexity:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Install tools
        run: |
          go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
          go install github.com/uudashr/gocognit/cmd/gocognit@latest

      - name: Report cyclomatic complexity
        run: |
          echo "## Cyclomatic Complexity Report" >> "$GITHUB_STEP_SUMMARY"
          echo "" >> "$GITHUB_STEP_SUMMARY"
          echo "### Functions with complexity > 10" >> "$GITHUB_STEP_SUMMARY"
          echo '```' >> "$GITHUB_STEP_SUMMARY"
          gocyclo -over 10 -avg . 2>/dev/null | sort -rn | tee -a "$GITHUB_STEP_SUMMARY"
          echo '```' >> "$GITHUB_STEP_SUMMARY"
          OVER10=$(gocyclo -over 10 . 2>/dev/null | wc -l | tr -d ' ')
          OVER15=$(gocyclo -over 15 . 2>/dev/null | wc -l | tr -d ' ')
          echo "" >> "$GITHUB_STEP_SUMMARY"
          echo "| Threshold | Count |" >> "$GITHUB_STEP_SUMMARY"
          echo "|---|---|" >> "$GITHUB_STEP_SUMMARY"
          echo "| > 10 | $OVER10 |" >> "$GITHUB_STEP_SUMMARY"
          echo "| > 15 | $OVER15 |" >> "$GITHUB_STEP_SUMMARY"

      - name: Report cognitive complexity
        run: |
          echo "## Cognitive Complexity Report" >> "$GITHUB_STEP_SUMMARY"
          echo "" >> "$GITHUB_STEP_SUMMARY"
          echo "### Functions with cognitive complexity > 15" >> "$GITHUB_STEP_SUMMARY"
          echo '```' >> "$GITHUB_STEP_SUMMARY"
          gocognit -over 15 . 2>/dev/null | sort -rn | tee -a "$GITHUB_STEP_SUMMARY"
          echo '```' >> "$GITHUB_STEP_SUMMARY"
```

### Coverage correlation

Use `go test -coverprofile` output to show which high-complexity functions
have the lowest branch coverage. A high-complexity function with low coverage
is the highest-priority refactoring target.

```bash
# Generate coverage profile
go test ./... -coverprofile=cover.out -coverpkg=./...

# Show uncovered lines in files with high-complexity functions
gocyclo -over 12 . \
  | awk '{print $3}' \
  | cut -d: -f1 \
  | sort -u \
  | xargs -I{} go tool cover -func=cover.out | grep "^{}"
```

### Sprint checklist template

Use the following as a recurring sprint issue template:

```markdown
## Complexity Reduction Sprint N

**Target:** Reduce functions with cyclomatic > 10 from X to Y.

### Functions to refactor this sprint

| File | Function | Cyclomatic | Cognitive | Owner | PR |
|---|---|---|---|---|---|
| | | | | | |

### Definition of Done

- [ ] All listed functions refactored below threshold.
- [ ] Unit tests cover all new branches (≥ 80 % per file).
- [ ] `make lint` passes with zero warnings on changed files.
- [ ] `docs/complexity-budget.yml` updated (entries removed or added).
- [ ] Coverage diff shows improvement (attach `go test -cover` output).
```

---

## Implementation Phases

### Phase 1 – Tighten thresholds and establish baseline (Weeks 1–4)

1. Run `gocyclo -over 9 -avg .` and `gocognit -over 9 .`; commit output as
   `docs/complexity-baseline.txt`.
2. Lower `cyclop.max-complexity` from 15 → 12 in `.golangci.yml`.
3. Promote `revive cyclomatic` from warning to error.
4. Create `docs/complexity-budget.yml` with all existing violations.
5. Add `check-complexity-budget.sh` script and wire it into CI.
6. Add `complexity-trend.yml` nightly workflow.

**Success criteria:** `make lint` passes; budget file is complete; nightly
workflow produces its first report.

### Phase 2 – Refactor highest-complexity functions (Weeks 5–12)

Priority order (highest cyclomatic first):

1. Any function in `internal/exec/` with cyclomatic > 20.
2. Any function in `pkg/` with cyclomatic > 15.
3. Any function in `cmd/` with cyclomatic > 12.

For each function:

- Extract helpers into a `*_helpers.go` file (co-located).
- Replace switch/if dispatch with table-driven maps.
- Write unit tests covering all extracted helpers.
- Remove entry from `docs/complexity-budget.yml`.
- Verify with `make testacc-cover` that patch coverage improves.

**Success criteria:** Zero functions above cyclomatic 15 in `internal/exec/`.

### Phase 3 – Tighten thresholds further (Weeks 13–20)

1. Lower `cyclop.max-complexity` from 12 → 10.
2. Lower `revive cyclomatic` from 10 → 8.
3. Promote `gocognit` from warning to error at 12.
4. Promote `nestif` from warning to error.
5. Budget file should contain fewer than 20 entries.

**Success criteria:** `make lint --severity error` passes for all files touched
in the last 90 days.

### Phase 4 – Eliminate the budget (Weeks 21–26)

1. Refactor all remaining budget entries.
2. Remove budget CI script (no longer needed).
3. Delete `docs/complexity-budget.yml` or leave empty as documentation.
4. Final coverage report: target ≥ 80 % overall.

**Success criteria:** `docs/complexity-budget.yml` is empty; overall test
coverage ≥ 80 %.

---

## Refactoring Patterns Reference

### Pattern A – Coordinator + helpers

Applicable when a function does many independent things sequentially.

```
Original:  func Big() { A(); B(); C(); D(); E() }
Refactored:
  func Big() { a(); b(); c() }          // cyclomatic 1
  func a() { A_step1(); A_step2() }      // cyclomatic ≤ 5
  func b() { B_step1(); B_step2() }      // cyclomatic ≤ 5
  func c() { C_step1(); C_step2() }      // cyclomatic ≤ 5
```

### Pattern B – Dispatch table

Applicable when a switch/if-chain selects a sub-algorithm by key.

```
Original:  switch key { case "a": ...; case "b": ... }  // cyclomatic = N
Refactored:
  var handlers = map[string]func() error{"a": handleA, "b": handleB}
  fn := handlers[key]; fn()              // cyclomatic = 2
```

### Pattern C – Options struct

Applicable when a function behaves differently based on many boolean flags.

```
Original:  func F(a, b, c, d bool) { if a { ... } if b { ... } ... }
Refactored:
  type FOptions struct { A, B, C, D bool }
  func F(opts FOptions) { applyA(opts); applyB(opts) }
```

### Pattern D – Predicate extraction

Applicable when `if` conditions are long or repeated.

```
Original:  if x != nil && x.Cfg != nil && x.Cfg.Enabled && len(x.Items) > 0 { ... }
Refactored:
  func isReady(x *T) bool { return x != nil && x.Cfg != nil && x.Cfg.Enabled && len(x.Items) > 0 }
  if isReady(x) { ... }
```

---

## Relationship to Other PRDs

| PRD | Relationship |
|---|---|
| `testability-refactoring-strategy.md` | Dependency-injection patterns that complement complexity reduction |
| `test-coverage-improvement-plan.md` | Coverage targets driven by complexity reduction |
| `avoiding-deep-exits-pattern.md` | Deep exits often appear in high-complexity functions |
| `sentinel-error-enforcement.md` | Sentinel errors simplify error-path branches |
| `error-handling-linter-rules.md` | Error handling rules reduce branch counts |

---

## References

- [gocyclo](https://github.com/fzipp/gocyclo) – cyclomatic complexity tool.
- [gocognit](https://github.com/uudashr/gocognit) – cognitive complexity tool.
- [cyclop golangci-lint linter](https://golangci-lint.run/usage/linters/#cyclop).
- [revive cyclomatic rule](https://revive.run/r#cyclomatic).
- [nestif golangci-lint linter](https://golangci-lint.run/usage/linters/#nestif).
- [funlen golangci-lint linter](https://golangci-lint.run/usage/linters/#funlen).
- McCabe, T.J. (1976). "A Complexity Measure". IEEE Transactions on Software Engineering.
- Cognitive complexity: [Sonar white paper](https://www.sonarsource.com/docs/CognitiveComplexity.pdf).
