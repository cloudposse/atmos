package exec

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
// Results are cached by command path to avoid repeated subprocess execution.
func IsOpenTofu(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "exec.IsOpenTofu")()

	command := atmosConfig.Components.Terraform.Command
	if command == "" {
		command = "terraform" // Default to terraform if not specified.
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

// isKnownOpenTofuFeature checks if the error message matches known OpenTofu-specific features
// that terraform-config-inspect doesn't support.
func isKnownOpenTofuFeature(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// List of error patterns that indicate OpenTofu-specific syntax.
	openTofuPatterns := []string{
		"Variables not allowed", // Module source interpolation (OpenTofu 1.8+).
		// Add more patterns here as OpenTofu adds features that diverge from Terraform.
	}

	for _, pattern := range openTofuPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}
