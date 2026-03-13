package analyze

import (
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	pkgversion "github.com/cloudposse/atmos/pkg/version"
)

// Package-level function vars for dependency injection in tests.
var (
	startCaptureFunc  = StartCapture
	analyzeOutputFunc = AnalyzeOutput
	exitFunc          = errUtils.Exit
)

// Context holds the state for AI output analysis across the command lifecycle.
// It manages the capture session, skill prompts, and analysis execution.
type Context struct {
	enabled        bool
	skillNames     []string
	skillPrompt    string
	commandName    string
	captureSession *CaptureSession
	atmosConfig    *schema.AtmosConfiguration
}

// Setup validates AI configuration, loads skills if specified, and starts output capture.
// Returns a ready-to-use Context. The caller MUST call Cleanup() (via defer) to restore stdout/stderr.
func Setup(atmosConfig *schema.AtmosConfiguration, skillNames []string, commandName string) (*Context, error) {
	defer perf.Track(nil, "analyze.Setup")()

	ctx := &Context{
		enabled:     true,
		skillNames:  skillNames,
		commandName: commandName,
		atmosConfig: atmosConfig,
	}

	// Validate AI configuration early to fail fast with helpful errors.
	if err := ValidateAIConfig(atmosConfig); err != nil {
		return nil, err
	}

	// If skills specified, load and validate all skills.
	if len(skillNames) > 0 {
		loader := newSkillLoader()
		validSkills, err := skills.LoadAndValidate(atmosConfig, skillNames, loader)
		if err != nil {
			return nil, err
		}
		ctx.skillPrompt = skills.BuildPrompt(validSkills)
	}

	// Start capturing stdout/stderr while still teeing to the terminal.
	captureSession, err := startCaptureFunc()
	if err != nil {
		ui.Warning(fmt.Sprintf("Failed to set up AI output capture, continuing without AI analysis: %v", err))
		ctx.enabled = false
		return ctx, nil
	}
	ctx.captureSession = captureSession

	return ctx, nil
}

// newSkillLoader creates a marketplace skill loader, returning nil if unavailable.
func newSkillLoader() skills.SkillLoader {
	installer, err := marketplace.NewInstaller(pkgversion.Version)
	if err != nil {
		log.Debug("Marketplace skill loader unavailable, using config-only skills", "error", err)
		return nil
	}
	return installer
}

// NewDisabledContext returns a Context with AI disabled (used when --ai is not set).
func NewDisabledContext() *Context {
	return &Context{enabled: false}
}

// Enabled returns whether AI analysis is active.
func (c *Context) Enabled() bool {
	return c != nil && c.enabled && c.captureSession != nil
}

// Cleanup restores stdout/stderr if capture is active.
// Safe to call multiple times (Stop is idempotent).
func (c *Context) Cleanup() {
	if c != nil && c.captureSession != nil {
		c.captureSession.Stop()
	}
}

// RunAnalysis stops the capture session and sends captured output to the AI provider.
// When there's an error, it prints the formatted error BEFORE the AI analysis so the user
// sees the error first, followed by the AI explanation.
// Returns true if the error was already printed (caller should not re-print it).
func (c *Context) RunAnalysis(cmdErr error) bool {
	defer perf.Track(nil, "analyze.Context.RunAnalysis")()

	if !c.Enabled() {
		return false
	}

	stdout, stderrCaptured := c.captureSession.Stop()

	// If the command failed, print the error to stderr first so it appears before the AI analysis.
	// Also append it to captured stderr so the AI sees the full error context.
	if cmdErr != nil {
		formatted := errUtils.Format(cmdErr, errUtils.DefaultFormatterConfig())
		_, _ = iolib.MaskWriter(os.Stderr).Write([]byte(formatted + "\n"))

		if stderrCaptured != "" {
			stderrCaptured += "\n"
		}
		stderrCaptured += formatted
	}

	analyzeOutputFunc(c.atmosConfig, &AnalysisInput{
		CommandName: c.commandName,
		Stdout:      stdout,
		Stderr:      stderrCaptured,
		CmdErr:      cmdErr,
		SkillNames:  c.skillNames,
		SkillPrompt: c.skillPrompt,
	})

	if cmdErr != nil {
		// Error was already printed. Exit with proper code.
		exitFunc(errUtils.GetExitCode(cmdErr))
	}
	return true
}

// BuildCommandName reconstructs the command name from os.Args for AI analysis context.
func BuildCommandName() string {
	return strings.Join(os.Args, " ")
}
