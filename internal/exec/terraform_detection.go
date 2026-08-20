package exec

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// DetectionCache caches the results of OpenTofu detection by command path.
	detectionCache    = make(map[string]bool)
	detectionCacheMux sync.RWMutex
)

// IsOpenTofu detects whether the configured terraform command is OpenTofu or Terraform.
// It uses a two-tier detection strategy:
//  1. Fast path: Check if the executable basename contains "tofu"
//  2. Slow path: Execute the version command and check the output
//
// The tenv parameter provides toolchain path resolution. If nil, the command
// is used as-is (system PATH only). Pass the ToolchainEnvironment from the
// current command invocation so toolchain-installed executables are found.
// Results are cached by resolved command path to avoid repeated subprocess execution.
func IsOpenTofu(atmosConfig *schema.AtmosConfiguration, tenv *dependencies.ToolchainEnvironment) bool {
	defer perf.Track(atmosConfig, "exec.IsOpenTofu")()

	command := atmosConfig.Components.Terraform.Command
	if command == "" {
		command = "terraform" // Default to terraform if not specified.
	}

	// Resolve through toolchain so detection works when the tool is
	// installed via `atmos toolchain install` and not on system PATH.
	if tenv != nil {
		command = tenv.Resolve(command)
	}

	// Check cache first.
	detectionCacheMux.RLock()
	if cached, exists := detectionCache[command]; exists {
		detectionCacheMux.RUnlock()
		return cached
	}
	detectionCacheMux.RUnlock()

	// Fast path: Check basename for "tofu".
	baseName := filepath.Base(command)
	if strings.Contains(strings.ToLower(baseName), "tofu") {
		if atmosConfig.Logs.Level == u.LogLevelTrace {
			log.Debug("Detected OpenTofu by executable name: " + baseName)
		}
		cacheDetectionResult(command, true)
		return true
	}

	// Slow path: Execute version command to detect.
	isTofu := detectByVersionCommand(atmosConfig, command)

	// Cache result.
	cacheDetectionResult(command, isTofu)

	return isTofu
}

// detectByVersionCommand executes the version command and checks if the output contains "OpenTofu".
func detectByVersionCommand(atmosConfig *schema.AtmosConfiguration, command string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If we can't detect, assume Terraform (safer default for strict validation).
		log.Warn("Could not detect if '" + command + "' is OpenTofu or Terraform (" + err.Error() + "), assuming Terraform")
		return false
	}

	outputStr := string(output)
	isTofu := strings.Contains(outputStr, "OpenTofu")

	if atmosConfig.Logs.Level == u.LogLevelTrace {
		if isTofu {
			log.Debug("Detected OpenTofu by version command: " + command)
		} else {
			log.Debug("Detected Terraform by version command: " + command)
		}
	}

	return isTofu
}

// cacheDetectionResult stores the detection result in the cache.
func cacheDetectionResult(command string, isTofu bool) {
	detectionCacheMux.Lock()
	defer detectionCacheMux.Unlock()
	detectionCache[command] = isTofu
}

// isKnownModuleSourceInterpolationDiagnostic checks if the error message matches a known,
// non-fatal diagnostic that terraform-config-inspect emits when a module's `source` (or
// similar) attribute references a variable.
//
// Terraform-config-inspect decodes these attributes via gohcl.DecodeExpression with a nil
// hcl.EvalContext, so ANY variable reference in that position produces the HCL diagnostic
// "Variables not allowed" -- regardless of whether the configured tool/version actually
// supports evaluating it. This is valid, modern syntax under both OpenTofu 1.8+ (module
// source interpolation) and Terraform 1.15+ (via `const = true` variables); it is not a
// tool-specific feature gate, so this check intentionally does not depend on which command
// (terraform/tofu) is configured.
func isKnownModuleSourceInterpolationDiagnostic(err error) bool {
	if err == nil {
		return false
	}

	return matchesModuleSourceInterpolationPattern(err.Error())
}

// matchesModuleSourceInterpolationPattern checks whether msg matches a known,
// non-fatal terraform-config-inspect diagnostic pattern. It takes a plain string rather than an
// error so callers checking a diagnostic's Summary/Detail text don't need to allocate a throwaway
// dynamic error just to satisfy an error-typed signature.
func matchesModuleSourceInterpolationPattern(msg string) bool {
	// List of error patterns produced by terraform-config-inspect's inability to evaluate
	// variable references in module source/version attributes.
	moduleSourceInterpolationPatterns := []string{
		"Variables not allowed", // Module source interpolation (OpenTofu 1.8+, Terraform 1.15+ const vars).
		// Add more patterns here as needed.
	}

	for _, pattern := range moduleSourceInterpolationPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}

// diagnosticPositionKey returns a grouping key for a diagnostic based on its source position.
// A diagnostic with no position is never grouped with any other diagnostic (its key is unique to
// its index), so it must independently match the known pattern.
func diagnosticPositionKey(index int, diag tfconfig.Diagnostic) string {
	if diag.Pos == nil {
		return fmt.Sprintf("nopos:%d", index)
	}
	return fmt.Sprintf("%s:%d", diag.Pos.Filename, diag.Pos.Line)
}

// allDiagnosticsAreModuleSourceInterpolation reports whether every error-severity diagnostic in
// diags belongs to a known module-source-interpolation event.
//
// Terraform-config-inspect emits a *companion* diagnostic at the same source position as the
// "Variables not allowed" one -- e.g. "Unsuitable value: value must be known", produced when it
// then tries to decode the resulting unknown value into module.source's target type. Both are
// side effects of the same root cause (decoding with a nil hcl.EvalContext), so diagnostics are
// grouped by position: a group is known-safe if ANY diagnostic in it matches the known pattern.
// Diagnostics at any OTHER position must independently match, so a genuine, unrelated error is
// never absorbed into the same group just because it happens to share the diagnostics list.
//
// This must inspect diags individually rather than the collapsed diags.Err() string: tfconfig's
// Diagnostics.Error() only renders the first diagnostic's Summary/Detail, collapsing any others
// to "(and N other messages)" with no text at all. Matching against that collapsed string means a
// genuine, unrelated diagnostic occurring alongside the known-safe one -- and sorting before it --
// would never surface in the matched text, yet the caller would still skip validation for the
// whole diagnostics set, silently discarding the real error.
func allDiagnosticsAreModuleSourceInterpolation(diags tfconfig.Diagnostics) bool {
	groupIsKnown := make(map[string]bool)
	var order []string

	for i, diag := range diags {
		if diag.Severity != tfconfig.DiagError {
			continue
		}

		key := diagnosticPositionKey(i, diag)
		if _, seen := groupIsKnown[key]; !seen {
			order = append(order, key)
		}

		msg := fmt.Sprintf("%s: %s", diag.Summary, diag.Detail)
		if matchesModuleSourceInterpolationPattern(msg) {
			groupIsKnown[key] = true
		}
	}

	if len(order) == 0 {
		return false
	}

	for _, key := range order {
		if !groupIsKnown[key] {
			return false
		}
	}

	return true
}
