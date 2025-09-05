# Debugging GitHub Workflow Test Failures

This guide explains how to debug test failures in the GitHub Actions workflow using tmate (terminal mate).

## What is tmate?

tmate is a terminal sharing application that allows you to SSH into a GitHub Actions runner while it's running. This is incredibly useful for debugging test failures that only occur in CI.

## How to Enable tmate Debugging

### Method 1: Using PR Labels (Recommended for PRs)

Add one of these labels to your Pull Request:

- **`debug-with-tmate`** - Starts a tmate session BEFORE running tests
  - Useful when you want to inspect the environment setup
  - Session runs for up to 15 minutes in detached mode
  - Tests continue running while you're connected

- **`debug-on-failure`** - Starts a tmate session ONLY if tests fail
  - Useful for investigating test failures
  - Session runs for up to 30 minutes
  - Only activates on test failure

### Method 2: Using Workflow Dispatch (Manual Runs)

1. Go to the Actions tab in GitHub
2. Select the "Tests" workflow
3. Click "Run workflow"
4. Choose the branch
5. Set "Enable tmate debugging" to one of:
   - **`false`** - No debugging (default)
   - **`true`** - Start tmate before tests
   - **`on-failure`** - Start tmate only if tests fail

## How to Connect

When tmate starts, you'll see output like this in the GitHub Actions log:

```
SSH: ssh <random-string>@nyc1.tmate.io
or
Web shell: https://tmate.io/t/<session-id>
```

1. Copy the SSH command
2. Run it in your terminal
3. You'll be connected to the GitHub Actions runner

## What You Can Do in tmate

- Inspect the environment: `env | grep -E "GITHUB|PATH"`
- Check installed tools: `which atmos`, `terraform version`
- Run tests manually: `make testacc-ci`
- Debug specific test: `go test -v -run TestSpecificTest ./...`
- Check file structure: `ls -la`, `tree`
- View logs: `cat test-results.json`
- Install debugging tools: `apt-get update && apt-get install -y vim strace`

## Important Notes

1. **Access is limited to the PR author** when using `limit-access-to-actor: true`
2. **Sessions have timeouts** to prevent hanging workflows
3. **Windows runners** don't support tmate when in draft mode
4. **Costs apply** - GitHub Actions minutes are consumed while tmate is running
5. **Security** - Never share sensitive information or credentials in tmate sessions

## Debugging the testacc-ci Target

The acceptance tests run using gotcha with these steps:

```bash
# What the CI runs:
cd tools/gotcha && go mod download
go install -C tools/gotcha .
gotcha stream ./... \
    --show=all \
    --timeout=40m \
    --coverprofile=coverage.out \
    --output=test-results.json \
    -- -coverpkg=github.com/cloudposse/atmos/...
gotcha parse test-results.json --format=github --coverprofile=coverage.out --post-comment
```

Common issues to check:
- PATH issues: `echo $PATH` and `which atmos`
- Binary permissions: `ls -la ./build/`
- Go module issues: `go mod tidy`
- Test timeouts: Individual tests have 40m timeout

## Example Debugging Session

```bash
# Connect to tmate
ssh randomstring@nyc1.tmate.io

# Check environment
pwd
ls -la
which atmos
atmos version

# Run specific test
go test -v -run TestThatFails ./pkg/...

# Check test artifacts
cat test-results.json | jq '.'
cat test-summary.md

# Run with more verbose output
ATMOS_LOGS_LEVEL=Debug make testacc-ci
```

## Cleanup

Remember to:
1. Remove debug labels from PR when done
2. Exit tmate sessions when finished (`exit` or Ctrl+D)
3. The session will auto-terminate based on timeout settings