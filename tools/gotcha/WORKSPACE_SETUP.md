# Gotcha Workspace Setup

## Module Independence

Gotcha is a **completely standalone Go module** that is independent from Atmos. This separation is enforced through Go workspaces.

### Module Structure

- **Gotcha Module**: `github.com/cloudposse/gotcha` (in `tools/gotcha/`)
- **Atmos Module**: Main repository module  
- **Separation**: Enforced via `go.work` file at repository root

## Problem Solved

Atmos and gotcha are completely independent tools, but when they exist in the same directory structure, 
Go module operations can sometimes create unwanted interactions. The go.work file prevents this.

## Solution: go.work File

The `go.work` file explicitly defines which modules are part of the workspace:

```go
go 1.25

use .
// Explicitly excludes tools/gotcha from the workspace
```

This configuration ensures:
1. ✅ Atmos go mod operations don't affect gotcha
2. ✅ Gotcha go mod operations don't affect Atmos  
3. ✅ No accidental cross-dependencies
4. ✅ Clean module boundaries

## Working with the Modules

### When developing gotcha:
```bash
# Navigate to gotcha directory
cd tools/gotcha

# Work with gotcha's go.mod independently
go mod download
go mod tidy
go test ./...
go build .
```

### When working on Atmos:
```bash
# Stay in repository root
go mod download
go mod tidy
# Gotcha is completely ignored
```

## CI/CD Configuration

GitHub Actions workflows handle gotcha separately:
- Separate test job: `gotcha` in `.github/workflows/test-tools.yml`
- Independent Go module cache
- Isolated build and test environment
- Go version 1.25 for consistency

## Why This Matters

1. **Clean Dependencies**: Atmos doesn't pull in gotcha's test dependencies
2. **Version Independence**: Each module can use different dependency versions
3. **Build Isolation**: Building Atmos doesn't require gotcha's dependencies
4. **Test Isolation**: Test failures in one don't affect the other

## Testing the Separation

```bash
# From repository root - should show no gotcha
go list -m all | grep gotcha

# Verify workspace setup
cat go.work

# Test independence
go mod tidy           # Only affects Atmos
cd tools/gotcha && go mod tidy  # Only affects gotcha
```

## Troubleshooting

If you encounter module-related issues:

1. **Verify workspace setup**:
   ```bash
   # Should exist at repository root
   cat go.work
   ```

2. **Check module independence**:
   ```bash
   # From repository root - should show no gotcha
   go list -m all | grep gotcha
   ```

3. **Reset workspace if needed**:
   ```bash
   # Remove and recreate workspace file
   rm go.work
   go work init .
   # Note: Don't add tools/gotcha to the workspace
   ```

## Development Best Practices

- ✅ **DO**: Treat gotcha as a separate project
- ✅ **DO**: Run gotcha tests from `tools/gotcha/` directory
- ✅ **DO**: Keep gotcha's dependencies minimal and focused
- ❌ **DON'T**: Import gotcha packages from Atmos code
- ❌ **DON'T**: Import Atmos packages from gotcha code
- ❌ **DON'T**: Add `tools/gotcha` to the go.work file

## Alternative Solutions Considered

1. **Separate Repository**: Move gotcha to github.com/cloudposse/gotcha (most isolation)
2. **Build Tags**: Use `//go:build tools` to exclude from default builds
3. **Submodules**: Use git submodules for complete separation

The workspace solution was chosen as it provides good isolation while keeping the code in one repository.

## Current Status
✅ go.work file created and configured
✅ Atmos and gotcha are now completely independent
✅ No module interference between the two tools
✅ CI/CD properly configured with Go 1.25
