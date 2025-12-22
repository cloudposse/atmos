# Terraform Streaming UI Mode

## Problem Statement

Terraform's default output is overwhelming, especially for new users. When running `terraform plan` or `terraform apply`, users are confronted with walls of text containing:

- Detailed resource attribute diffs with JSON-like syntax
- Verbose state refresh messages for every resource
- Provider initialization logs
- Backend configuration details
- Technical identifiers and ARNs that obscure the actual changes

For a simple infrastructure change, users might see hundreds of lines of output. This creates several problems:

1. **Information overload**: Important changes are buried in noise. Users struggle to identify what's actually being created, modified, or destroyed.

2. **Progress blindness**: During long-running operations, there's no clear indication of progress. Users see occasional "Still creating..." messages but have no sense of completion percentage or estimated time.

3. **Error obscurity**: When errors occur, they're often buried in the output stream. Users must scroll through logs to find what went wrong.

4. **Context switching**: Users must mentally parse technical Terraform syntax to understand business-level changes.

5. **CI/CD unfriendliness**: While verbose output is useful for debugging, it creates noise in CI logs and makes it harder to scan for issues.

## Solution

Add an optional streaming TUI (Terminal User Interface) mode that transforms Terraform's raw output into a Docker-build-style progress display. This mode:

- Shows real-time resource status with clear visual indicators
- Displays a progress bar with completion percentage and elapsed time
- Condenses completed operations into toast-style summaries
- Highlights errors and warnings prominently
- Auto-disables in non-TTY environments (piped output, CI)

### Example Output

**During execution:**
```
⠋ apply plat-ue2-dev/vpc Creating aws_security_group.default (5.2s)  ████████░░░░ 2/5

  ✓ Created  aws_vpc.main (2.1s)
  ✓ Created  aws_subnet.public[0] (1.3s)
```

The inline progress bar shows spinner, command, current activity, and completion status on a single line.

**Dependency tree with attribute changes:**
```
     plat-ue2-dev/myapp
  ●  ├── aws_s3_object.file
     │     content_type  "text/html"  →  "text/plain"
     │     source        "hello.html" →  "hello.txt"
  ●  └── aws_instance.new
           ami            (none)       →  "ami-12345"
           instance_type  (none)       →  "t3.micro"
```

Resources are marked with colored dots (●): green for create, yellow for update, red for delete. Attribute changes display in a two-column layout with color-coded keys.

**Multi-line value changes:**
```
  ●  └── aws_s3_object.weather
           content
          - Current weather: Sunny, 72°F
          - Humidity: 45%
          + Current weather: Cloudy, 65°F
          + Humidity: 80%
          + Wind: 15 mph NW
```

**Interactive confirmation:**
```
  Do you want to apply these changes?
  > Yes   No
```

**On completion:**
```
✓ Apply plat-ue2-dev/vpc completed (15.2s)
```

**On error:**
```
✗ Apply plat-ue2-dev/vpc failed: 1 error (12.1s)
  Error: aws_instance.web[0]: InvalidAMIID.NotFound
```

## Design Decisions

### Enabling the Feature

The streaming UI mode is controlled through a tri-state flag system:

| Flag | Config | Result |
|------|--------|--------|
| `--ui` | any | Enabled (if TTY available) |
| `--ui=false` | any | Disabled |
| not set | `ui.enabled: true` | Enabled (if TTY available) |
| not set | `ui.enabled: false` | Disabled |

**Configuration in `atmos.yaml`:**
```yaml
components:
  terraform:
    ui:
      enabled: true
```

**Environment variable:** `ATMOS_TERRAFORM_UI=true`

### Auto-Disable Conditions

The streaming UI automatically falls back to standard output when:

1. **No TTY attached**: Output is being piped or redirected
2. **CI environment detected**: `CI=true` environment variable is set
3. **Unsupported command**: Commands other than `plan`, `apply`, `init`, `refresh`

This ensures scripts and CI pipelines continue to work without modification.

### Technical Approach

The implementation leverages Terraform's machine-readable JSON output format (`-json` flag):

1. Atmos intercepts the terraform command
2. Adds the `-json` flag automatically
3. Parses the line-delimited JSON output in real-time
4. Renders a Bubbletea-based TUI that updates as messages arrive
5. Preserves exit codes for downstream tooling (critical for `plan -detailed-exitcode`)

### Message Types Handled

Terraform's JSON streaming format includes:

- `version` - Terraform version info
- `planned_change` - Resource changes detected during plan
- `apply_start` / `apply_progress` / `apply_complete` / `apply_errored` - Apply lifecycle
- `refresh_start` / `refresh_complete` - State refresh lifecycle
- `change_summary` - Final count of additions/changes/deletions
- `diagnostic` - Warnings and errors

### Exit Code Preservation

Exit codes are critical for automation:

- `0` - Success
- `1` - Error
- `2` - Success with changes (for `plan -detailed-exitcode`)

The streaming executor captures and propagates these codes via `errUtils.ExitCodeError`.

## Supported Commands

| Command | Streaming UI Support |
|---------|---------------------|
| `plan` | ✓ |
| `apply` | ✓ |
| `deploy` | ✓ (Atmos-specific apply wrapper) |
| `init` | ✓ |
| `refresh` | ✓ |
| `destroy` | ✓ (via apply -destroy) |

## Implementation Details

### Package Structure

```
pkg/terraform/ui/
├── types.go       # Terraform JSON message type definitions
├── parser.go      # Line-by-line JSON parser
├── resource.go    # Resource state machine and tracker
├── model.go       # Bubbletea TUI model for apply/plan progress
├── init_model.go  # Bubbletea TUI model for init command
├── tree.go        # Dependency tree builder and renderer
├── confirm.go     # Interactive confirmation prompts
├── executor.go    # Streaming execution orchestrator
└── *_test.go      # Unit tests
```

### Key Components

**Parser**: Reads Terraform's JSON output line-by-line, parsing each message into typed Go structs.

**ResourceTracker**: Thread-safe state machine that tracks all resources through their lifecycle (pending → in-progress → complete/error).

**Model**: Bubbletea model that renders the progress display, handling terminal resize, spinner animation, and progress bar updates.

**DependencyTree**: Parses terraform plan JSON output to build a visual tree of resource changes with:
- Box-drawing characters for hierarchical structure
- Colored dots (●) for resource actions (green=create, yellow=update, red=delete)
- Attribute-level changes with color-coded keys and before → after values in two-column layout
- Multi-line value support for file content and similar attributes

**ConfirmApply/ConfirmDestroy**: Interactive confirmation prompts using the `huh` library with:
- Atmos-styled theme integration
- Left-aligned button layout
- Proper margins for visual separation

**Executor**: Orchestrates the subprocess, pipes stdout to the parser, and manages the TUI lifecycle.

### Integration Points

1. **`cmd/terraform/flags.go`**: Adds `--ui` flag to shared terraform flags
2. **`cmd/terraform/options.go`**: Parses UI flag with tri-state detection
3. **`cmd/terraform/{plan,apply,deploy,init,refresh,destroy}.go`**: Uses `ParseTerraformRunOptions(v, cmd)`
4. **`internal/exec/terraform.go`**: Routes to streaming executor when enabled
5. **`pkg/schema/schema.go`**: Adds `TerraformUI` config struct

## Previous Attempts

This feature was previously attempted via pipeform integration (Issue #926, PRs #981, #983). Those attempts were closed due to:

1. **External dependency**: Required installing pipeform separately
2. **CGO requirement**: Some implementations required CGO
3. **Plan output concerns**: JSON format lacks full resource attribute details

Our implementation addresses these:

1. **Pure Go**: Uses charmbracelet/bubbletea (already a dependency)
2. **No CGO**: Entirely Go-based
3. **Fallback behavior**: Standard output remains available; plan details can be viewed with `terraform show <planfile>`

## Completed Enhancements

1. **Dependency tree visualization**: Visual tree of resource changes with box-drawing characters
2. **Attribute-level changes**: Shows before → after values for each changed attribute
3. **Multi-line value support**: Displays full content of multi-line values (e.g., file content)
4. **Interactive confirmations**: Styled confirmation prompts before apply/destroy operations
5. **Init command support**: Streaming UI for terraform init with module installation progress

## Future Enhancements

1. **`terraform test` support**: Add streaming UI for the test command
2. **Customizable themes**: Allow color/icon customization
3. **Log persistence**: Option to save raw JSON logs alongside TUI display
4. **Resource grouping**: Group resources by module for large infrastructures
5. **Estimated time**: Learn from past runs to estimate completion time

## References

- [Terraform Machine-Readable UI](https://developer.hashicorp.com/terraform/internals/machine-readable-ui)
- [Issue #926: Pipeform Integration](https://github.com/cloudposse/atmos/issues/926)
- [PR #981: Initial pipeform attempt](https://github.com/cloudposse/atmos/pull/981)
- [PR #983: Alternative pipeform approach](https://github.com/cloudposse/atmos/pull/983)
- [Charmbracelet Bubbletea](https://github.com/charmbracelet/bubbletea)
