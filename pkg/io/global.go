package io

import (
	"fmt"
	stdio "io"
	"os"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	// Data is the global writer for machine-readable output (stdout).
	// All writes are automatically masked based on registered secrets.
	// Use this for JSON, YAML, or any output meant for piping/automation.
	//
	// Example:
	//   fmt.Fprintf(io.Data, `{"version": "%s"}\n`, version)
	//
	// Safe default: Falls back to os.Stdout until Initialize() is called.
	Data stdio.Writer = os.Stdout

	// UI is the global writer for human-readable output (stderr).
	// All writes are automatically masked based on registered secrets.
	// Use this for messages, progress, logs meant for terminal display.
	//
	// Example:
	//   fmt.Fprintf(io.UI, "Processing...\n")
	//   logger := log.New(io.UI)
	//
	// Safe default: Falls back to os.Stderr until Initialize() is called.
	UI stdio.Writer = os.Stderr

	// GlobalContext holds the I/O context for advanced usage.
	globalContext Context
	initOnce      sync.Once
	initErr       error
)

// Initialize sets up the global I/O writers with automatic masking.
// This should be called once in cmd/root.go PersistentPreRun.
// If not called explicitly, it will be called automatically on first use.
func Initialize() error {
	defer perf.Track(nil, "io.Initialize")()

	initOnce.Do(func() {
		// Create I/O context
		globalContext, initErr = NewContext()
		if initErr != nil {
			// Fallback to unmasked writers on error
			Data = os.Stdout
			UI = os.Stderr
			return
		}

		// Register common secrets for auto-masking
		registerCommonSecrets(globalContext.Masker())

		// Set global writers (already wrapped with masking)
		Data = globalContext.Streams().Output()
		UI = globalContext.Streams().Error()
	})

	return initErr
}

// registerCommonSecrets automatically registers secrets from common environment variables.
// Note: We use os.Getenv here intentionally (not viper) because these are runtime
// secrets being detected, not configuration values.
func registerCommonSecrets(masker Masker) {
	// Register environment variable secrets
	registerEnvSecrets(masker)

	// Register common token patterns
	registerCommonPatterns(masker)
}

// registerEnvSecrets registers secrets from environment variables.
func registerEnvSecrets(masker Masker) {
	// AWS credentials
	registerEnvValue(masker, "AWS_ACCESS_KEY_ID", false)
	registerEnvValue(masker, "AWS_SECRET_ACCESS_KEY", true)
	registerEnvValue(masker, "AWS_SESSION_TOKEN", true)

	// GitHub tokens
	registerEnvValue(masker, "GITHUB_TOKEN", true)
	registerEnvValue(masker, "GH_TOKEN", true)

	// GitLab tokens
	registerEnvValue(masker, "GITLAB_TOKEN", true)
	registerEnvValue(masker, "CI_JOB_TOKEN", true)

	// Datadog
	registerEnvValue(masker, "DATADOG_API_KEY", true)
	registerEnvValue(masker, "DD_API_KEY", true)

	// Anthropic
	registerEnvValue(masker, "ANTHROPIC_API_KEY", true)
}

// registerEnvValue registers a value from an environment variable.
func registerEnvValue(masker Masker, envVar string, withEncodings bool) {
	value := os.Getenv(envVar) //nolint:forbidigo // Intentional use for runtime secret detection
	if value == "" {
		return
	}

	if withEncodings {
		masker.RegisterSecret(value)
	} else {
		masker.RegisterValue(value)
	}
}

// registerCommonPatterns registers regex patterns for common secret formats.
func registerCommonPatterns(masker Masker) {
	patterns := []string{
		`ghp_[A-Za-z0-9]{36}`,                        // GitHub PAT
		`gho_[A-Za-z0-9]{36}`,                        // GitHub OAuth
		`github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}`, // New GitHub PAT format
		`glpat-[A-Za-z0-9\-_]{20}`,                   // GitLab PAT
		`sk-[A-Za-z0-9]{48}`,                         // OpenAI API key
		`Bearer [A-Za-z0-9\-._~+/]+=*`,               // Bearer tokens
		`AKIA[0-9A-Z]{16}`,                           // AWS Access Key ID
		`[A-Za-z0-9/+=]{40}`,                         // AWS Secret Access Key (40 chars)
	}

	for _, pattern := range patterns {
		_ = masker.RegisterPattern(pattern)
	}
}

// MaskWriter wraps any io.Writer with automatic masking.
// Use this when you need to write to custom file handles with masking enabled.
//
// Example:
//
//	f, _ := os.Create("output.log")
//	maskedFile := io.MaskWriter(f)
//	fmt.Fprintf(maskedFile, "SECRET=%s\n", secret)  // Automatically masked
func MaskWriter(w stdio.Writer) stdio.Writer {
	defer perf.Track(nil, "io.MaskWriter")()

	if globalContext == nil {
		_ = Initialize()
	}
	if globalContext == nil {
		return w // Return unmasked if initialization failed
	}
	return &maskedWriter{
		underlying: w,
		masker:     globalContext.Masker(),
	}
}

// RegisterSecret registers a secret value for masking.
// The secret and its common encodings (base64, URL, JSON) will be masked.
// This adds to the global masker used by io.Data and io.UI.
//
// Example:
//
//	apiToken := getToken()
//	io.RegisterSecret(apiToken)
//	fmt.Fprintf(io.UI, "Token: %s\n", apiToken)  // Automatically masked
func RegisterSecret(secret string) {
	defer perf.Track(nil, "io.RegisterSecret")()

	if secret == "" {
		return
	}

	if globalContext == nil {
		_ = Initialize()
	}
	if globalContext == nil {
		return
	}

	// Delegate to masker's RegisterSecret which handles all encodings
	globalContext.Masker().RegisterSecret(secret)
}

// RegisterValue registers a literal value for masking.
// Use this for values that don't need encoding variants.
//
// Example:
//
//	io.RegisterValue(sessionID)
func RegisterValue(value string) {
	defer perf.Track(nil, "io.RegisterValue")()

	if value == "" {
		return
	}

	if globalContext == nil {
		_ = Initialize()
	}
	if globalContext == nil {
		return
	}

	globalContext.Masker().RegisterValue(value)
}

// RegisterPattern registers a regex pattern for masking.
// Use this to mask values matching a specific pattern.
//
// Example:
//
//	io.RegisterPattern(`api_key=[A-Za-z0-9]+`)
func RegisterPattern(pattern string) error {
	defer perf.Track(nil, "io.RegisterPattern")()

	if pattern == "" {
		return nil
	}

	if globalContext == nil {
		if err := Initialize(); err != nil {
			return fmt.Errorf("failed to initialize global I/O context: %w", err)
		}
	}
	if globalContext == nil {
		return errUtils.ErrIOContextNotInitialized
	}

	return globalContext.Masker().RegisterPattern(pattern)
}

// GetContext returns the global I/O context for advanced usage.
// Most code should use io.Data and io.UI instead.
func GetContext() Context {
	defer perf.Track(nil, "io.GetContext")()

	if globalContext == nil {
		_ = Initialize()
	}
	return globalContext
}

// Reset clears the global I/O context and resets the initialization state.
// This is primarily used in tests to ensure clean state between test executions.
// The next call to Initialize() will pick up the current os.Stdout/os.Stderr values.
func Reset() {
	globalContext = nil
	initOnce = sync.Once{}
	initErr = nil
	// DO NOT set Data/UI to os.Stdout/os.Stderr here - that would capture
	// whatever stdout/stderr happen to be at Reset() time (e.g., a test pipe).
	// Instead, leave them unmodified so Initialize() will set them correctly
	// based on the CURRENT os.Stdout/os.Stderr when Initialize() runs.
	// Initialize() is called automatically by PersistentPreRun in each test.
}
