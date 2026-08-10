package exec

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

	errMsg := err.Error()

	// List of error patterns produced by terraform-config-inspect's inability to evaluate
	// variable references in module source/version attributes.
	moduleSourceInterpolationPatterns := []string{
		"Variables not allowed", // Module source interpolation (OpenTofu 1.8+, Terraform 1.15+ const vars).
		// Add more patterns here as needed.
	}

	for _, pattern := range moduleSourceInterpolationPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}
