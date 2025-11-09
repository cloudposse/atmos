# PRD: Experimental PTY Mode with Output Masking

## Overview

This PRD describes the experimental PTY (pseudo-terminal) feature for Atmos devcontainer commands. The feature enables automatic output masking in interactive terminal sessions by proxying TTY data through a PTY layer where masking can be applied.

## Problem Statement

### Current Limitation

Atmos has a robust masking system (PR #1714) that automatically redacts sensitive data (AWS keys, GitHub tokens, etc.) in command output. However, masking **does not work in interactive TTY sessions** due to architectural constraints.

**Why masking fails in TTY mode:**

1. Interactive commands use direct TTY connections (`docker exec -it`)
2. Data flows at kernel level between terminal file descriptors
3. Never passes through Go `io.Writer` streams where masking happens
4. This is a fundamental limitation of how TTYs work

**Affected commands:**
- `atmos devcontainer exec --interactive`
- `atmos devcontainer shell`
- `atmos devcontainer attach`
- `atmos auth shell`

### User Impact

Users must choose between **security** (non-interactive with masking) OR **usability** (interactive without masking):

```bash
# Secure but limited: Output masked, no tab completion
atmos devcontainer exec geodesic -- env | grep AWS
# AWS_SECRET_ACCESS_KEY=***MASKED***

# Usable but insecure: Full TTY features, no masking
atmos devcontainer shell geodesic
# Inside shell: echo $AWS_SECRET_ACCESS_KEY
# Output: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY (exposed!)
```

### Business Value

**Without PTY masking:**
- ❌ Secrets exposed in interactive sessions (screen shares, recordings)
- ❌ Compliance issues (SOC2, HIPAA, etc.)
- ❌ Users must remember to avoid echoing secrets

**With PTY masking:**
- ✅ Secrets automatically masked even in interactive mode
- ✅ Users can safely share screens during pair programming
- ✅ Session recordings don't leak credentials
- ✅ Full TTY features (colors, tab completion, cursor control)

## Proposed Solution

### High-Level Architecture

Introduce experimental `--pty` flag that creates a PTY proxy layer between the user's terminal and the container:

```
Current (No Masking):
User Terminal ←─────→ docker exec -it ←─────→ Container
                Direct kernel-level TTY connection
                ❌ No masking possible

Proposed (With PTY Proxy):
User Terminal ←─→ Atmos PTY Proxy ←─→ docker exec -it ←─→ Container
                       ↓
                  Masking Layer
                  ✅ Masking works!
```

### How It Works

1. **Atmos starts command with PTY** using `github.com/creack/pty`
2. **Two goroutines proxy data bidirectionally**:
   - Input: User Terminal → PTY → Container (no masking)
   - Output: Container → PTY → **Mask** → User Terminal (masking applied)
3. **User gets both**:
   - Full TTY features (colors, tab completion, etc.)
   - Automatic secret masking

### Integration with Existing IO Package

The PTY layer integrates seamlessly with the existing `pkg/io` masking infrastructure:

```go
// pkg/terminal/pty/pty.go uses existing masking infrastructure
type Options struct {
    Masker        iolib.Masker     // From pkg/io
    EnableMasking bool
}

func ExecWithPTY(ctx context.Context, cmd *exec.Cmd, opts *Options) error {
    ptmx, _ := pty.Start(cmd)

    // Output goroutine wraps PTY with masking
    go func() {
        var writer io.Writer = os.Stdout

        if opts.EnableMasking && opts.Masker != nil {
            // Use maskedWriter to apply masking
            writer = &maskedWriter{
                underlying: os.Stdout,
                masker:     opts.Masker,  // Uses existing masker!
            }
        }

        io.Copy(writer, ptmx)
    }()
}

// maskedWriter applies masking from pkg/io.Masker
type maskedWriter struct {
    underlying io.Writer
    masker     iolib.Masker
}

func (w *maskedWriter) Write(p []byte) (n int, err error) {
    masked := w.masker.Mask(string(p))  // Uses existing Mask() method
    return w.underlying.Write([]byte(masked))
}
```

**Key integration points:**
1. Uses `iolib.GetContext().Masker()` to get configured masker
2. Respects `--mask` flag state via `Masker.Enabled()`
3. Uses same patterns from `pkg/io/global.go` (AWS keys, GitHub tokens, etc.)
4. Honors custom patterns from `atmos.yaml`

## Requirements

### Functional Requirements

#### FR1: Experimental Flag
- Add `--pty` flag to `devcontainer exec` and `devcontainer shell`
- **Default: Disabled** (opt-in for testing)
- Flag behavior:
  - `--pty` → Enable PTY mode
  - `--pty=false` → Explicitly disable (for clarity)
  - Omit flag → Use standard TTY (current behavior)

#### FR2: Platform Support
- **Supported**: macOS, Linux
- **Unsupported**: Windows (PTYs don't exist on Windows)
- Clear error message when used on Windows:
  ```
  Error: --pty flag not supported on Windows
  ```

#### FR3: Masking Integration
- Integrate with existing `pkg/io` masking system
- Respect global `--mask` flag
- Use same patterns as non-PTY commands
- Apply masking to output only (not input)

#### FR4: Commands in Scope
Initial implementation (Phase 1):
- ✅ `atmos devcontainer exec --pty`
- ✅ `atmos devcontainer shell --pty`

Future extensions (out of scope):
- ⏳ `atmos auth shell --pty`
- ⏳ `atmos terraform shell --pty` (if implemented)

#### FR5: Reusable Package
- Create `pkg/terminal/pty` package for reusability
- Interface-driven design for testability
- Documented with examples

### Non-Functional Requirements

#### NFR1: Performance
- Target: < 10% latency overhead vs direct TTY
- Acceptable for interactive use
- No noticeable typing lag

#### NFR2: Terminal Compatibility
- Support common terminal emulators (iTerm2, Terminal.app, gnome-terminal)
- Preserve ANSI color codes
- Handle basic cursor control
- **Known limitations**: May not handle all edge cases (documented)

#### NFR3: Error Handling
- Graceful degradation on errors
- Clear error messages
- Proper cleanup on exit/crash

#### NFR4: Documentation
- Mark as **experimental** in all docs
- Document limitations clearly
- Provide usage examples
- Explain when to use vs not use

## Design Details

### Package Structure

```
pkg/terminal/pty/
  ├── pty.go           # Core PTY execution with masking
  ├── pty_test.go      # Unit tests
  ├── setup_unix.go    # Unix-specific terminal setup
  ├── setup_windows.go # Windows-specific terminal setup
  └── README.md        # Package documentation
```

### Core Interface

```go
package pty

// Options configures PTY execution behavior.
type Options struct {
    Masker        iolib.Masker  // Masker from pkg/io
    EnableMasking bool          // Whether to apply masking
}

// ExecWithPTY executes a command in a PTY with optional output masking.
func ExecWithPTY(ctx context.Context, cmd *exec.Cmd, opts *Options) error

// IsSupported returns true if PTY is supported on this platform.
func IsSupported() bool
```

### Command Flow

**devcontainer exec without `--pty`** (current behavior):
```
User → Cobra → ExecuteDevcontainerExec()
              ↓
         execInContainer()
              ↓
         runtime.Exec() with Tty: true
              ↓
         Direct TTY (no masking)
```

**devcontainer exec with `--pty`** (new behavior):
```
User → Cobra → ExecuteDevcontainerExec()
              ↓
         execInContainer() detects --pty
              ↓
         execInContainerWithPTY()
              ↓
         pty.ExecWithPTY() with masker
              ↓
         PTY Proxy (masking applied)
```

### Configuration Precedence

Masking behavior (highest to lowest priority):
1. `--mask=false` flag → Disables masking entirely
2. `--pty` flag → Enables PTY mode (masking works if `--mask` not false)
3. Default → Standard TTY (masking doesn't work)

### Error Cases

| Scenario | Behavior |
|----------|----------|
| `--pty` on Windows | Error: "PTY not supported on Windows" |
| `--pty` with `--mask=false` | PTY mode active, masking disabled |
| PTY creation fails | Fall back to standard TTY with warning |
| Process crashes | PTY cleanup in defer, terminal restored |

## Implementation Plan

### Phase 1: Core PTY Package (Week 1)
- [x] Create `pkg/terminal/pty/pty.go` with `ExecWithPTY()`
- [x] Implement `IsSupported()` platform check
- [x] Add `maskedWriter` wrapper
- [x] Unit tests for `pkg/terminal/pty`
- [x] Basic integration test

**Deliverable**: Reusable PTY package tested on macOS/Linux

### Phase 2: Devcontainer Integration (Week 1)
- [ ] Add `--pty` flag to `cmd/devcontainer/exec.go`
- [ ] Add `--pty` flag to `cmd/devcontainer/shell.go`
- [ ] Update `ExecuteDevcontainerExec()` signature
- [ ] Implement `execInContainerWithPTY()`
- [ ] Update `ExecuteDevcontainerShell()` for PTY
- [ ] Implement `attachToContainerWithPTY()`

**Deliverable**: PTY mode available in exec and shell commands

### Phase 3: Documentation (Week 1)
- [ ] Update CLI usage markdown
- [ ] Update Docusaurus documentation
- [ ] Add experimental warnings
- [ ] Document limitations
- [ ] Create usage examples

**Deliverable**: Complete user-facing documentation

### Phase 4: Testing & Refinement (Week 2)
- [ ] Manual testing on macOS
- [ ] Manual testing on Linux
- [ ] Verify Windows error handling
- [ ] Performance testing
- [ ] Terminal compatibility testing
- [ ] Bug fixes

**Deliverable**: Production-ready experimental feature

## Testing Strategy

### Unit Tests
```go
// Test platform detection
func TestIsSupported(t *testing.T)

// Test basic PTY execution
func TestExecWithPTY_BasicCommand(t *testing.T)

// Test masking integration
func TestExecWithPTY_WithMasking(t *testing.T)

// Test Windows error
func TestExecWithPTY_UnsupportedPlatform(t *testing.T)
```

### Integration Tests
```bash
# Test PTY mode works
atmos devcontainer exec geodesic --pty -- echo AKIAIOSFODNN7EXAMPLE
# Expected: ***MASKED***

# Test PTY with shell
atmos devcontainer shell geodesic --pty
# Inside: echo $AWS_SECRET_ACCESS_KEY
# Expected: ***MASKED***

# Test colors preserved
atmos devcontainer exec geodesic --pty -- ls --color=auto
# Expected: Colors display correctly

# Test Windows error
# On Windows:
atmos devcontainer exec geodesic --pty -- echo test
# Expected: Clear error message
```

### Manual Test Checklist
- [ ] Tab completion works in PTY mode
- [ ] Colors display correctly
- [ ] Cursor control works (vim, nano)
- [ ] Ctrl+C interrupts properly
- [ ] AWS keys masked in output
- [ ] GitHub tokens masked in output
- [ ] Custom patterns from atmos.yaml work
- [ ] `--mask=false` disables masking
- [ ] Performance acceptable (< 100ms latency)

## Risks & Mitigations

### Risk 1: ANSI Escape Sequence Corruption
**Risk**: Masking might break ANSI color codes or cursor control sequences
**Likelihood**: Medium
**Impact**: High (visual corruption, unusable terminal)
**Mitigation**:
- Extensive testing with colored output
- Document known issues
- Provide `--pty=false` escape hatch

### Risk 2: Windows Platform Confusion
**Risk**: Users on Windows try to use `--pty` and get frustrated
**Likelihood**: High
**Impact**: Medium (user confusion)
**Mitigation**:
- Clear error message explaining Windows limitation
- Documentation mentions platform support upfront
- Consider hiding flag on Windows builds

### Risk 3: Performance Degradation
**Risk**: PTY proxy adds noticeable latency
**Likelihood**: Low
**Impact**: High (poor user experience)
**Mitigation**:
- Benchmark against direct TTY
- Optimize masking regex patterns
- Use efficient buffer sizes
- Make it opt-in so users can disable if slow

### Risk 4: Terminal Compatibility Issues
**Risk**: Some terminal emulators behave unexpectedly
**Likelihood**: Medium
**Impact**: Medium (broken features)
**Mitigation**:
- Test on common terminals (iTerm2, Terminal.app, gnome-terminal)
- Document tested terminals
- Provide troubleshooting guide
- Flag is opt-in, users can disable if issues

## Success Metrics

### Adoption Metrics
- Track `--pty` flag usage in telemetry (if enabled)
- Monitor GitHub issues for PTY-related feedback
- Survey users about PTY experience

### Quality Metrics
- Zero critical bugs in first release
- < 5 bug reports per month after release
- > 90% success rate in CI tests
- Performance: < 10% latency overhead

### User Satisfaction
- Documentation clarity: positive feedback
- Feature usefulness: users adopt flag
- Issue resolution: < 48 hours for P0 bugs

## Out of Scope

The following are **explicitly out of scope** for the initial implementation:

1. **Terminal size handling** - SIGWINCH signal handling is implemented via pty.InheritSize for Unix-like systems (see pkg/terminal/pty/setup_unix.go); edge cases may still exist for complex resize scenarios
2. **Advanced signal forwarding** - Basic signals only (Ctrl+C)
3. **Terminal state management** - No raw mode terminal state handling
4. **Configuration in atmos.yaml** - Flag-only for now
5. **Default enabled** - Remains experimental/opt-in
6. **Binary data handling** - Assumes text output
7. **auth shell / terraform shell** - Devcontainer only initially
8. **Performance optimizations** - Focus on correctness first

These may be addressed in future iterations based on user feedback.

## Dependencies

### External Libraries
- `github.com/creack/pty` (already in use for test CLI)

### Internal Dependencies
- `pkg/io` (masking infrastructure)
- `pkg/terminal` (terminal capabilities and PTY operations)
- `pkg/container` (runtime interface)
- `pkg/devcontainer` (configuration)

### Existing Patterns
Leverages patterns from `tests/cli_test.go`:
- `simulateTtyCommand()` function
- `ptyError()` error handling
- PTY lifecycle management

## Documentation Requirements

### User-Facing Documentation
1. **CLI Usage Examples** (`cmd/markdown/*.md`)
   - Show `--pty` flag usage
   - Explain when to use vs not use
   - Mark as experimental

2. **Docusaurus Guide** (`website/docs/cli/commands/devcontainer/*.mdx`)
   - Dedicated "Experimental PTY Mode" section
   - Platform support table
   - Troubleshooting guide
   - Known limitations

3. **Release Notes** (when shipped)
   - Announce experimental feature
   - Link to documentation
   - Request feedback

### Developer-Facing Documentation
1. **PRD** (this document)
   - Architecture and design
   - Integration points
   - Implementation plan

2. **Package README** (`pkg/terminal/pty/README.md`)
   - Usage examples for other commands
   - API documentation
   - How to extend

3. **Architecture Decision Record** (optional)
   - Why PTY approach chosen
   - Alternatives considered
   - Trade-offs documented

## Open Questions

1. **Should we eventually make PTY the default?**
   - Decision: Gather feedback first, revisit in Q2 2025

2. **Should we expose PTY buffer size as configuration?**
   - Decision: No, use sensible default (4096), add later if needed

3. **Should we extend to auth/terraform shell immediately?**
   - Decision: No, devcontainer first to validate approach

4. **How to handle terminal resize?**
   - Decision: Out of scope for Phase 1, add if users request

## Alternatives Considered

### Alternative 1: Don't Implement PTY Masking
**Approach**: Document limitation, recommend non-interactive exec
**Pros**: Zero implementation cost, no maintenance burden
**Cons**: Users can't have both TTY features and masking
**Decision**: Rejected - Users need both security and usability

### Alternative 2: Parse ANSI Escape Sequences Intelligently
**Approach**: Don't mask within ANSI escape codes
**Pros**: Prevents breaking terminal control sequences
**Cons**: Complex parsing, high implementation cost
**Decision**: Deferred - Start simple, add if needed

### Alternative 3: Post-Processing Hook
**Approach**: Record session, mask after the fact
**Pros**: Simpler than real-time masking
**Cons**: Secrets still visible during session (security issue)
**Decision**: Rejected - Doesn't solve real-time exposure

## References

- PR #1714: IO/UI Framework with Masking
- `tests/cli_test.go`: Existing PTY usage patterns
- `github.com/creack/pty`: PTY library documentation
- CLAUDE.md: Comment preservation, error handling guidelines

## Appendix A: Code Examples

### Example Usage

```bash
# Non-interactive with masking (current, works)
atmos devcontainer exec geodesic -- env | grep AWS
# Output: AWS_SECRET_ACCESS_KEY=***MASKED***

# Interactive without PTY (current, no masking)
atmos devcontainer exec geodesic --interactive -- bash
# In bash: echo $AWS_SECRET_ACCESS_KEY
# Output: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY (exposed!)

# Interactive with PTY (new, has masking!)
atmos devcontainer exec geodesic --interactive --pty -- bash
# In bash: echo $AWS_SECRET_ACCESS_KEY
# Output: ***MASKED***

# Shell without PTY (current, no masking)
atmos devcontainer shell geodesic
# Inside: echo $AWS_SECRET_ACCESS_KEY
# Output: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY (exposed!)

# Shell with PTY (new, has masking!)
atmos devcontainer shell geodesic --pty
# Inside: echo $AWS_SECRET_ACCESS_KEY
# Output: ***MASKED***
```

### Example Integration

```go
// cmd/devcontainer/exec.go
var execUsePTY bool

func init() {
    execCmd.Flags().BoolVar(&execUsePTY, "pty", false,
        "Experimental: Use PTY mode with masking support")
}

// internal/exec/devcontainer.go
func execInContainer(ctx, runtime, containerID, interactive, usePTY, command) error {
    if usePTY {
        return execInContainerWithPTY(ctx, runtime, containerID, command)
    }
    // ... existing implementation
}

func execInContainerWithPTY(ctx, runtime, containerID, command) error {
    // Get masker from IO context
    ioCtx := iolib.GetContext()
    masker := ioCtx.Masker()

    // Create PTY options
    ptyOpts := &pty.Options{
        Masker:        masker,
        EnableMasking: viper.GetBool("mask"),
    }

    // Build docker/podman command
    cmd := buildContainerExecCommand(runtime, containerID, command)

    // Execute with PTY
    return pty.ExecWithPTY(ctx, cmd, ptyOpts)
}
```

## Appendix B: Platform Matrix

| Platform | PTY Support | Status | Notes |
|----------|-------------|--------|-------|
| macOS (Intel) | ✅ Yes | Supported | Primary development platform |
| macOS (ARM) | ✅ Yes | Supported | M1/M2/M3 Macs |
| Linux (x86_64) | ✅ Yes | Supported | Most CI/CD environments |
| Linux (ARM64) | ✅ Yes | Supported | AWS Graviton, etc. |
| Windows | ❌ No | Not Supported | PTYs don't exist on Windows |
| WSL2 | ✅ Yes | Supported | Linux PTY within WSL |

## Approval

**Product Owner**: Erik Osterman
**Engineering Lead**: Claude (AI Agent)
**Date**: 2025-01-04
**Status**: Approved for Implementation

---

**Next Steps**: Proceed with Phase 1 implementation (Core PTY Package).
