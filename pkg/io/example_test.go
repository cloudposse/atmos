package io_test

import (
	"fmt"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ExampleNewPattern demonstrates the new I/O and UI pattern.
// This shows the clear separation between I/O (channels) and UI (formatting).
func Example_newPattern() {
	// ===== SETUP =====
	// Create I/O context - provides channels and masking.
	ioCtx, _ := iolib.NewContext()

	// Create terminal instance - provides TTY detection and color capabilities.
	term := terminal.New()

	// Create UI formatter - provides formatting functions (returns strings).
	formatter := ui.NewFormatter(ioCtx, term)

	// ===== PATTERN 1: Data Output (stdout) =====
	// Data channel is for pipeable output (JSON, YAML, results).

	// Plain data.
	fmt.Fprintf(ioCtx.Data(), "plain data output\n")

	// Formatted data (markdown help text).
	helpMarkdown, _ := formatter.Markdown("# Help\n\nThis is help text.")
	fmt.Fprint(ioCtx.Data(), helpMarkdown)

	// ===== PATTERN 2: UI Messages (stderr) =====
	// UI channel is for human-readable messages.

	// Plain message.
	fmt.Fprintf(ioCtx.UI(), "Loading configuration...\n")

	// Formatted success message.
	successMsg := formatter.Success("✓ Configuration loaded!")
	fmt.Fprintf(ioCtx.UI(), "%s\n", successMsg)

	// Formatted warning.
	warningMsg := formatter.Warning("⚠ Stack is using deprecated settings")
	fmt.Fprintf(ioCtx.UI(), "%s\n", warningMsg)

	// Formatted error.
	errorMsg := formatter.Error("✗ Failed to load stack")
	details := formatter.Muted("  Check your atmos.yaml syntax")
	fmt.Fprintf(ioCtx.UI(), "%s\n%s\n", errorMsg, details)

	// ===== PATTERN 3: Markdown for UI (stderr) =====
	// Error explanation with rich formatting.

	errorMarkdown, _ := formatter.Markdown("**Error:** Invalid configuration\n\n```yaml\nstack: invalid\n```")
	fmt.Fprint(ioCtx.UI(), errorMarkdown)

	// ===== PATTERN 4: Conditional Formatting =====
	// Only show progress if stderr is a TTY.

	if term.IsTTY(terminal.Stderr) {
		progress := formatter.Info("⏳ Processing 150 components...")
		fmt.Fprintf(ioCtx.UI(), "%s\n", progress)
	}

	// ===== PATTERN 5: Using Terminal Capabilities =====
	// Set terminal title (if supported).

	term.SetTitle("Atmos - Processing")
	defer term.RestoreTitle()

	// Check color support.
	if formatter.SupportsColor() {
		coloredMsg := formatter.Success("Color is supported!")
		fmt.Fprintf(ioCtx.UI(), "%s\n", coloredMsg)
	}

	// Get terminal width for adaptive formatting.
	width := term.Width(terminal.Stdout)
	if width > 0 {
		fmt.Fprintf(ioCtx.UI(), "Terminal width: %d columns\n", width)
	}
}

// Example_comparisonOldVsNew shows the difference between old and new patterns.
func Example_comparisonOldVsNew() {
	ioCtx, _ := iolib.NewContext()
	term := terminal.New()
	formatter := ui.NewFormatter(ioCtx, term)

	// ===== OLD PATTERN (being phased out) =====
	// Mixed concerns - unclear where output goes.
	//
	// out := ui.NewOutput(ioCtx)
	// out.Print("data")           // Where does this go? stdout? stderr?
	// out.Success("done!")        // This goes to stderr, but not obvious.
	// out.Markdown("# Doc")       // Is this data or UI? stdout or stderr?

	// ===== NEW PATTERN (preferred) =====
	// Explicit channels - always clear where output goes.

	// Data → stdout (explicit).
	fmt.Fprintf(ioCtx.Data(), "data\n")

	// UI → stderr (explicit).
	successMsg := formatter.Success("done!")
	fmt.Fprintf(ioCtx.UI(), "%s\n", successMsg)

	// Markdown - developer chooses channel based on context.
	helpMarkdown, _ := formatter.Markdown("# Documentation")

	// Help text → stdout (pipeable).
	fmt.Fprint(ioCtx.Data(), helpMarkdown)

	// Error details → stderr (UI message).
	errorMarkdown, _ := formatter.Markdown("**Error:** Something failed")
	fmt.Fprint(ioCtx.UI(), errorMarkdown)
}

// Example_decisionTree shows how to decide where to output.
func Example_decisionTree() {
	ioCtx, _ := iolib.NewContext()
	term := terminal.New()
	formatter := ui.NewFormatter(ioCtx, term)

	// DECISION TREE:
	// 1. WHERE should it go?
	//    ├─ Pipeable data (JSON, YAML, results)     → ioCtx.Data()
	//    ├─ Human messages (status, errors, help)    → ioCtx.UI()
	//    └─ User input                               → ioCtx.Input()
	//
	// 2. HOW should it look?
	//    ├─ Plain text                               → fmt.Fprintf(channel, text)
	//    ├─ Colored/styled                           → fmt.Fprintf(channel, formatter.Success(text))
	//    └─ Markdown rendered                        → fmt.Fprint(channel, formatter.Markdown(md))
	//
	// 3. WHEN to format?
	//    ├─ Always for UI channel                    → Use formatter.* methods
	//    ├─ Conditionally for data channel           → Check term.IsTTY()
	//    └─ Never for piped output                   → Auto-handled by I/O layer

	// Example: Command help.
	// Help is DATA (can be saved, piped) but uses markdown formatting.
	helpContent := "# atmos terraform apply\n\nApplies Terraform configuration..."
	rendered, _ := formatter.Markdown(helpContent)
	fmt.Fprint(ioCtx.Data(), rendered)

	// Example: Processing status.
	// Status is UI (human-readable only) with formatting.
	status := formatter.Info("⏳ Processing 150 components...")
	fmt.Fprintf(ioCtx.UI(), "%s\n", status)

	// Example: Success message.
	// Success is UI with semantic formatting.
	msg := formatter.Success("✓ Deployment complete!")
	fmt.Fprintf(ioCtx.UI(), "%s\n", msg)

	// Example: JSON output.
	// JSON is DATA (no formatting).
	jsonString := `{"result": "success"}`
	fmt.Fprintf(ioCtx.Data(), "%s\n", jsonString)

	// Example: Error with explanation.
	// Error is UI with markdown for rich explanation.
	errorTitle := formatter.Error("Failed to load stack configuration")
	errorDetails, _ := formatter.Markdown("**Reason:** Invalid YAML syntax\n\n```yaml\nstack: invalid\n```")
	fmt.Fprintf(ioCtx.UI(), "%s\n\n%s\n", errorTitle, errorDetails)
}

// Example_keyPrinciples demonstrates the architectural principles.
func Example_keyPrinciples() {
	ioCtx, _ := iolib.NewContext()
	term := terminal.New()
	formatter := ui.NewFormatter(ioCtx, term)

	// KEY PRINCIPLE 1: I/O layer provides CHANNELS and CAPABILITIES.
	// - Channels: Where does output go? (stdout, stderr, stdin).
	// - Capabilities: What can the terminal do? (color, TTY, width).
	// - NO formatting logic in I/O layer.

	// Access channels.
	_ = ioCtx.Data()  // stdout - pipeable data.
	_ = ioCtx.UI()    // stderr - human messages.
	_ = ioCtx.Input() // stdin - user input.

	// Access capabilities.
	_ = term.IsTTY(terminal.Stdout)
	_ = term.ColorProfile()
	_ = term.Width(terminal.Stdout)

	// KEY PRINCIPLE 2: UI layer provides FORMATTING.
	// - Returns formatted strings (pure functions).
	// - NEVER writes to streams directly.
	// - Uses I/O layer for capability detection.

	// Get formatted strings.
	_ = formatter.Success("Success!")    // Returns string.
	_ = formatter.Warning("Warning!")    // Returns string.
	_ = formatter.Error("Error!")        // Returns string.
	_, _ = formatter.Markdown("# Title") // Returns string.

	// KEY PRINCIPLE 3: Application layer COMBINES both.
	// - Gets formatted string from UI layer.
	// - Chooses channel from I/O layer.
	// - Uses fmt.Fprintf to write.

	msg := formatter.Success("Done!")
	fmt.Fprintf(ioCtx.UI(), "%s\n", msg)
}
