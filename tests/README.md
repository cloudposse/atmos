# Atmos Test Suite

We have automated tests in packages, as well as standalone tests in this directory.

Smoke tests are implemented to verify the basic functionality and expected behavior of the compiled `atmos` binary, simulating real-world usage scenarios.

## Quick Start

```bash
# Run all tests (will skip if preconditions not met)
go test ./...

# Run with verbose output to see skips
go test -v ./...

# Bypass all precondition checks
export ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
go test ./...

# Run tests with make
make test
```

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
Require atmos binary built in repository:
```bash
make build
```

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
│   ├── TestCLICommands.stderr.golden
│   └── TestCLICommands_which_atmos.stdout.golden
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

See [Testing Strategy PRD](../docs/prd/testing-strategy.md) for the complete design document.

## Test Cases

Our convention is to implement a test-case configuration file per scenario. Then place all smoke tests related to that scenario in the file.

### Environment Variables

The tests will automatically set some environment variables:

- `GO_TEST=1` is always set, so commands in atmos can disable certain functionality during tests
- `TERM` is set when `tty: true` to emulate a proper terminal
- `HOME` is set to an empty temporary directory
- `XDG_*` is set to an empty temporary directory

### Flags

To regenerate ALL snapshots pass the `-regenerate-snapshots` flag.

> ![WARNING]
>
> #### This will regenerate all the snapshots
>
> After regenerating, make sure to review the differences:
>
> ```shell
> git diff tests/snapshots
> ```

To regenerate the snapshots for a specific test, just run:

(replace `TestCLICommands/check_atmos_--help_in_empty-dir` with your test name)

```shell
go test ./tests -v -run 'TestCLICommands/check_atmos_--help_in_empty-dir' -timeout 2m -regenerate-snapshots
```

After generating new golden snapshots, don't forget to add them.

```shell
git add tests/snapshots/*
```

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
```
