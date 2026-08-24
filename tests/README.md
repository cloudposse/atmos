# Atmos Test Suite

We have automated tests in packages, as well as standalone tests in this directory.

Smoke tests are implemented to verify the basic functionality and expected behavior of the compiled `atmos` binary, simulating real-world usage scenarios.

## Quick Start

```bash
# Run quick tests only (skip long-running tests >2s)
go test -short ./...
atmos test short

# Run all tests including long-running ones (will skip if preconditions not met)
go test ./...
atmos test acc

# Run with verbose output to see skips
go test -v ./...

# Bypass all precondition checks
export ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
go test ./...
```

## Short Mode

Run quick tests only, skipping tests that take more than 2 seconds:

```bash
go test -short ./...
atmos test short
```

Long tests include:
- Network I/O (vendor pulls, OCI registry)
- Git operations (cloning, checkouts)
- Heavy processing (Atlantis config generation)

To run all tests including long ones:
```bash
go test ./...
atmos test acc
```

## Heavy Integration Tests

Some integration tests download large binaries and are skipped by default to avoid unexpected downloads and long test times.

### Playwright Browser Driver Tests

Tests that download Chromium browser drivers (~140 MB) require explicit opt-in:

```bash
# Run Playwright integration tests (downloads Chromium)
RUN_PLAYWRIGHT_INTEGRATION=1 go test -v ./pkg/auth/providers/aws -run TestPlaywrightDriverDownload_Integration

# Without the flag, these tests are skipped
go test ./pkg/auth/providers/aws
# Output: SKIP: Skipping Playwright integration test (set RUN_PLAYWRIGHT_INTEGRATION=1 to run)
```

These tests validate that SAML browser automation actually downloads and installs the Chromium browser correctly.

### Floci Cloud Emulator Tests

Floci-backed E2E tests are opt-in and should use fixtures under
`tests/fixtures/scenarios`, not `gists/`. The gists are manual examples; the
test fixtures are allowed to be parameterized for isolation and CI.

Use `newFlociHarness` from `tests/floci_harness_test.go` for new Floci tests.
It centralizes endpoint discovery, the `ATMOS_TEST_FLOCI=true` opt-in gate,
temporary fixture copies, command env, local Atmos binary execution, and common
AWS verification clients.

#### Running locally (auto-start)

When you opt in with `ATMOS_TEST_FLOCI=true` and leave the `FLOCI_*` endpoint
variables unset, the suite **starts the Floci emulators for you**
(using [testcontainers](https://golang.testcontainers.org/)) and tears them down
when the run finishes. The container runtime is auto-detected via `pkg/container`,
so either **Docker or Podman** works — for Podman the machine's API socket is wired
into testcontainers automatically (and the Ryuk reaper is disabled, since rootless
Podman cannot run it):

```bash
ATMOS_TEST_FLOCI=true go test ./tests -run Floci
```

If no container runtime is available, the Floci tests **skip** with an explanatory
message rather than failing. The emulator images are pinned in
`tests/floci_containers_test.go` and kept in sync with the CI service containers.

#### Bringing your own Floci

To target a Floci instance you manage (or the CI service containers), pre-set the
endpoint variables. Auto-start sees they are already set and does nothing:

```bash
ATMOS_TEST_FLOCI=true \
FLOCI_ENDPOINT_URL=http://localhost:4566 \
FLOCI_GCP_ENDPOINT=http://localhost:4588 \
FLOCI_AZURE_ENDPOINT=http://localhost:4577 \
go test ./tests -run Floci
```

GitHub Actions runs the AWS, GCP, and Azure store/secrets Floci coverage in the dedicated
`[floci] go e2e` job from `.github/workflows/test.yml`. The general acceptance
test matrix does not set `ATMOS_TEST_FLOCI`, so these tests continue to skip
outside the Floci job.

The shared harness supports separate Floci endpoints for AWS-compatible
services, GCP Secret Manager, and Azure Key Vault. AWS uses
`FLOCI_ENDPOINT_URL`, GCP uses `FLOCI_GCP_ENDPOINT`, and Azure uses
`FLOCI_AZURE_ENDPOINT`.

Do not use LocalStack for these tests.

## Understanding Test Skips

When you run tests, you may see output like:
```
--- SKIP: TestLoadAWSConfig (0.00s)
    aws_utils_test.go:23: AWS profile 'cplive-core-gbl-identity' not configured: required for S3 backend testing. Configure AWS credentials or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
```

This means the test was skipped due to a missing precondition, not a code failure. This is expected behavior and allows developers to run tests without having all external dependencies configured.

## Common Preconditions

### AWS Tests
Require AWS profile configuration. Set up with:
```bash
aws configure --profile cplive-core-gbl-identity
```

Or use any valid AWS authentication method:
- Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
- IAM roles (if running on EC2/ECS)
- SSO profiles

### Git Tests
Require being in a Git repository with remotes:
```bash
git init
git remote add origin https://github.com/cloudposse/atmos.git
```

### Network Tests
Require internet connectivity to GitHub. Check with:
```bash
curl -I https://github.com
curl https://api.github.com/rate_limit
```

### Binary Tests
Tests automatically build a temporary atmos binary for each test run:
- When coverage is **disabled** (default): Builds a regular binary
- When coverage is **enabled** (GOCOVERDIR set): Builds with coverage instrumentation

This ensures tests always run with the latest code changes without requiring manual rebuilds.

### OCI Registry Tests
Require GitHub token for pulling OCI images:
```bash
export GITHUB_TOKEN=$(gh auth token)
# or
export ATMOS_GITHUB_TOKEN=<your-token>
```

## Test Directory Structure

```shell
├── cli_test.go                                      # Responsible for smoke testing
├── fixtures/
│   ├── components/
│   │   └── terraform/                               # Components that are conveniently reused for tests
│   ├── scenarios/                                   # Test scenarios consisting of stack configurations (valid & invalid)
│   │   ├── complete/                                # Advanced stack configuration with both broken and valid stacks
│   │   ├── metadata/                                # Test configurations for `metadata.*`
│   │   └── relative-paths/                          # Test configurations for relative imports
│   └── schemas/                                     # Schemas used for JSON validation
├── snapshots/                                       # Golden snapshots (what we expect output to look like)
│   ├── TestCLICommands.stderr.golden                # stderr snapshot for non-TTY test
│   ├── TestCLICommands_which_atmos.stdout.golden    # stdout snapshot for non-TTY test
│   └── TestCLICommands_atmos_list.tty.golden        # Combined snapshot for TTY test
└── test-cases/
    ├── complete.yaml
    ├── core.yaml
    ├── demo-custom-command.yaml
    ├── demo-stacks.yaml
    ├── demo-vendoring.yaml
    ├── metadata.yaml
    ├── relative-paths.yaml
    └── schema.json                                 # JSON schema for validation

```

> ![IMPORTANT]
> #### GitHub API Rate Limits
>
> To avoid API rate limits, make sure you've set `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN`. Atmos will use these automatically for requests to GitHub. Run the following command to set it. This assumes you've already installed the `gh` CLI and logged in.
> ```bash
> export GITHUB_TOKEN=$(gh auth token)
> ```

## Precondition Checking

Tests use intelligent precondition checking to skip when requirements aren't met. This provides a better developer experience by distinguishing between:
- **Environmental issues** (missing AWS profile, no network) → Test skips with helpful message
- **Code failures** (assertion failures, bugs) → Test fails

### Environment Variables

#### `ATMOS_TEST_SKIP_PRECONDITION_CHECKS`
Set to `true` to bypass all precondition checks:
```bash
export ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
go test ./...
```

This is useful when:
- Running in CI with mocked services
- Testing specific functionality without full setup
- Debugging test failures

### Writing Tests with Preconditions

When writing new tests, use the helper functions from `tests/test_preconditions.go`:

```go
import "github.com/cloudposse/atmos/tests"

func TestAWSFeature(t *testing.T) {
    // Check AWS precondition at test start
    tests.RequireAWSProfile(t, "cplive-core-gbl-identity")

    // Test code here...
}

func TestGitHubVendoring(t *testing.T) {
    // Check GitHub access and rate limits
    rateLimits := tests.RequireGitHubAccess(t)

    // Optionally check if we have enough requests
    if rateLimits != nil && rateLimits.Remaining < 20 {
        t.Skipf("Need at least 20 GitHub API requests, only %d remaining", rateLimits.Remaining)
    }

    // Test code here...
}

func TestGitOperations(t *testing.T) {
    // Check for Git repository with valid remotes
    tests.RequireGitRemoteWithValidURL(t)

    // Test code here...
}

func TestOCIVendoring(t *testing.T) {
    // Check for OCI authentication (GitHub token)
    tests.RequireOCIAuthentication(t)

    // Test code here...
}
```

### Available Helper Functions

| Function | Purpose | Skip Condition |
|----------|---------|----------------|
| `RequireAWSProfile(t, profile)` | Check AWS configuration | Profile not available |
| `RequireGitRepository(t)` | Check Git repo | Not in Git repo |
| `RequireGitRemoteWithValidURL(t)` | Check Git remotes | No valid remote URL |
| `RequireGitHubAccess(t)` | Check GitHub connectivity | Network/rate limit issues |
| `RequireNetworkAccess(t, url)` | Check general network | URL unreachable |
| `RequireExecutable(t, name, purpose)` | Check for executable | Not in PATH |
| `RequireEnvVar(t, name, purpose)` | Check environment variable | Not set |
| `RequireFilePath(t, path, purpose)` | Check file/directory exists | Missing path |
| `RequireOCIAuthentication(t)` | Check OCI registry auth | GitHub token not set |

See [Testability Refactoring Strategy](../docs/prd/testability-refactoring-strategy.md) for the complete design document.

## Test Cases

Our convention is to implement a test-case configuration file per scenario. Then place all smoke tests related to that scenario in the file.

### Snapshot Files

The test framework uses three types of snapshot files to capture expected output:

#### 1. `.stdout.golden` - Standard Output (Non-TTY)
Used when `tty: false`. Contains only stdout data (results meant for piping, JSON output, CSV data).

**Example**: `TestCLICommands_atmos_list_components.stdout.golden`

#### 2. `.stderr.golden` - Standard Error (Non-TTY)
Used when `tty: false`. Contains only stderr data (UI messages, warnings, progress indicators).

**Example**: `TestCLICommands_atmos_list_components.stderr.golden`

#### 3. `.tty.golden` - TTY Output (Combined)
Used when `tty: true`. Contains **both stdout and stderr merged** as they appear in a real terminal.

**Example**: `TestCLICommands_atmos_list_components.tty.golden`

#### Why Different Snapshot Types?

**PTY/TTY Behavior**: When `tty: true`, the test uses a pseudo-terminal (PTY) that **merges stderr and stdout into a single stream**. This mimics how real terminals work - there's no separate "stderr screen" and "stdout screen". Everything appears on the same display.

**Non-TTY Behavior**: When `tty: false`, the test uses standard pipes where stdout and stderr remain separate. This enables proper piping and redirection (e.g., `atmos list components | jq`).

#### Example Test Configuration

```yaml
tests:
  # TTY mode: single .tty.golden file
  - name: atmos list instances
    tty: true                         # Uses PTY (combines streams)
    snapshot: true
    # Creates: TestCLICommands_atmos_list_instances.tty.golden
    # Contains: telemetry notices (stderr) + table output (stdout)

  # Non-TTY mode: separate .stdout.golden and .stderr.golden files
  - name: atmos list instances no tty
    tty: false                        # Uses pipes (keeps streams separate)
    snapshot: true
    # Creates: TestCLICommands_atmos_list_instances_no_tty.stdout.golden
    #          TestCLICommands_atmos_list_instances_no_tty.stderr.golden
```

#### Important Notes

- **TTY tests** only create `.tty.golden` (no `.stdout.golden` or `.stderr.golden`)
- **Non-TTY tests** create both `.stdout.golden` and `.stderr.golden` (no `.tty.golden`)
- The snapshot type is automatically determined by the `tty:` flag in the test case
- When regenerating snapshots, old files from the wrong type are **not** automatically deleted

### Environment Variables

The tests will automatically set some environment variables:

- `GO_TEST=1` is always set, so commands in atmos can disable certain functionality during tests
- `TERM` is set when `tty: true` to emulate a proper terminal
- `HOME` is set to an empty temporary directory
- `XDG_*` is set to an empty temporary directory

### Snapshot Regeneration

To regenerate ALL snapshots, use the Atmos development command:

> ![WARNING]
>
> #### This will regenerate all the snapshots
>
> After regenerating, make sure to review the differences:
>
> ```shell
> git diff tests/snapshots
> ```

```shell
atmos dev generate snapshots --all
```

To regenerate the snapshots for a specific test, just run:

(replace `TestCLICommands/check_atmos_--help_in_empty-dir` with your test name)

```shell
atmos dev generate snapshots --filter 'TestCLICommands/check_atmos_--help_in_empty-dir'
```

The `--filter` value is passed to Go's `-run` flag as a regular expression.

After generating new golden snapshots, don't forget to add them.

```shell
git add tests/snapshots/*
```

### Line Ending Normalization

Golden snapshots always use Unix line endings (LF: `\n`) regardless of the platform they're generated on. The test infrastructure automatically normalizes line endings to ensure cross-platform consistency:

- **CRLF (`\r\n`)** is converted to **LF (`\n`)** when writing and comparing snapshots
- **Standalone CR (`\r`)** characters are preserved for spinner and progress indicator output
- This ensures developers on Windows, macOS, and Linux see consistent test results

**What gets normalized:**
- Windows line endings: `"line1\r\nline2\r\n"` → `"line1\nline2\n"`
- Mixed endings: `"line1\r\nline2\n"` → `"line1\nline2\n"`

**What stays the same:**
- Unix line endings: `"line1\nline2\n"` → `"line1\nline2\n"` (unchanged)
- Spinner output: `"Progress\rDone\r"` → `"Progress\rDone\r"` (preserved)

This normalization happens automatically in:
- `updateSnapshot()` - when writing snapshots
- `readSnapshot()` - when reading snapshots
- `verifySnapshot()` - when comparing actual output to snapshots

See `tests/cli_snapshot_test.go` for comprehensive tests of this behavior.

### Custom Output Sanitization

The test framework automatically sanitizes test output to ensure consistent snapshots across different environments. Standard sanitization includes:

- Repository absolute paths → `/absolute/path/to/repo/`
- Request IDs → `<REDACTED>`
- Timestamps → normalized values
- PostHog tokens → `phc_TEST_TOKEN_PLACEHOLDER`
- Temporary file paths → normalized paths

For **test-specific sanitization** that doesn't need to be global, use the `sanitize` field in your test's `expect` section. This accepts a map of regex patterns to replacement strings.

#### When to Use Custom Sanitization

Use the `sanitize` field for one-off cases that are specific to individual tests:

- Session IDs, request IDs, or transaction IDs that vary between runs
- Build timestamps or version numbers in output
- User-specific identifiers (emails, usernames)
- Random tokens or UUIDs generated during test execution
- Environment-specific values that don't fit global patterns

**Don't use it for:**
- Values that should be sanitized globally (add to `sanitizeOutput()` function instead)
- Values that appear across many tests (add to standard sanitization)

#### Example: Sanitizing Session IDs and Timestamps

```yaml
tests:
  - name: deploy with session tracking
    enabled: true
    workdir: "fixtures/scenarios/complete/"
    command: "atmos"
    args: ["terraform", "plan", "component", "-s", "stack"]
    snapshot: true
    expect:
      exit_code: 0
      stdout:
        - "Session session-12345 started"
        - "Build build-2025-01-01-0000 deployed"
      # Custom sanitization for this test only
      sanitize:
        "session-[0-9]+": "session-12345"              # Normalize session IDs
        "build-\\d{4}-\\d{2}-\\d{2}-\\d+": "build-2025-01-01-0000"  # Normalize build IDs
```

#### Example: Sanitizing User-Specific Output

```yaml
tests:
  - name: user authentication flow
    enabled: true
    workdir: "fixtures/scenarios/auth/"
    command: "atmos"
    args: ["auth", "login"]
    snapshot: true
    expect:
      exit_code: 0
      stdout:
        - "User user@example.com logged in"
        - "Token: token-REDACTED"
      # Sanitize user-specific details
      sanitize:
        "[a-z.]+@[a-z.]+\\.[a-z]+": "user@example.com"  # Normalize email addresses
        "token-[a-zA-Z0-9]+": "token-REDACTED"          # Redact tokens
```

#### How It Works

1. **Standard sanitization runs first** - All global patterns (paths, request IDs, etc.) are applied
2. **Custom sanitization runs second** - Your test-specific patterns are applied
3. **Patterns are regular expressions** - Use Go regex syntax (see [regexp package](https://pkg.go.dev/regexp/syntax))
4. **Invalid patterns cause test failures** - Helpful error messages indicate which pattern failed

#### Tips for Regex Patterns

- **Escape special characters**: Use `\\d` for digits, `\\s` for whitespace
- **Use character classes**: `[0-9]+` for numbers, `[a-zA-Z]+` for letters
- **Escape dollar signs in replacements**: Use `$$` to include a literal `$` in the replacement string
- **Test your patterns**: Run tests with `-v` flag to see sanitization in action

#### Debugging Sanitization

If your snapshots aren't matching, check:

1. Run test with `-v` to see actual vs expected output
2. Verify your regex pattern is correct (test at [regex101.com](https://regex101.com/))
3. Check if pattern is already handled by standard sanitization
4. Regenerate snapshot: `go test ./tests -run 'TestName' -regenerate-snapshots`

See `tests/cli_sanitize_test.go` for comprehensive examples of sanitization patterns.

### Example Configuration

We support an explicit type `!not` on the `expect.stdout` and `expect.stderr` sections (not on `expect.diff`)

Snapshots are enabled by setting the `snapshots` flag, and using the `expect.diff` to ignore line-level differences. If no differences are expected, use an empty list. Note, things like paths will change between local development and CI, so some differences are often expected.

We recommend testing incorrect usage with `expect.exit_code` of non-zero. For example, passing unsupported arguments.

```yaml
# yaml-language-server: $schema=schema.json

tests:
  - name: atmos circuit-breaker
    description: >                            # Friendly description of what this test is verifying
      Ensure atmos breaks the infinite loop when shell depth exceeds maximum (10).

    enabled: true                             # Whether or not to enable this check
    skip:                                     # Conditions when to skip
      os: !not windows                        # Do not run on Windows (e.g. PTY not supported)
                                              # Use "darwin" for macOS
                                              # Use "linux" for Linux ;)

    snapshot: true                            # Enable golden snapshot. Use together with `expect.diff`

    clean: true                               # Whether or not to remove untracked files from workdir
    workdir: "fixtures/scenarios/complete/"   # Location to execute command
    env:
      SOME_ENV: true                          # Set an environment variable called "SOME_ENV"
    command: "atmos"                          # Command to run
    args:                                     # Arguments or flags passed to command
      - "help"

    expect:                                   # Assertions
      timeout: 1m                             # Maximum time it should take to run this test
      diff: []                                # List of expected differences
      stdout:                                 # Expected output to stdout or TTY. All TTY output is directed to stdout
      stderr:                                 # Expected output to stderr;
        - "^$"                                # Expect no output
      exit_code: 0                            # Expected exit code
      sanitize:                               # Optional: Custom sanitization rules (pattern -> replacement)
        "session-[0-9]+": "session-12345"    # Replace session IDs with fixed value
        "temp_[a-z0-9]+": "temp_xyz"          # Replace temp identifiers
```
