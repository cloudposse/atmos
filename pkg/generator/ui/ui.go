//nolint:revive // file-length-limit: UI orchestration requires cohesive component
package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/filesystem"
	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/scaffoldhooks"
	tmpl "github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/hooks"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/project/config"
	"github.com/cloudposse/atmos/pkg/terminal"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// UI layout constants.
const (
	// Terminal and table width defaults.
	defaultTerminalWidth = 80
	tableMargin          = 20
	tableBorderPadding   = 6
	tableBorderSpacing   = 8

	// Column widths for configuration summary table.
	settingColumnMinWidth = 12
	valueColumnMinWidth   = 45
	sourceColumnMinWidth  = 12

	// Column widths for template table.
	nameColumnMinWidth    = 20
	sourceColumnWidth     = 30
	versionColumnMinWidth = 15
	descColumnMinWidth    = 40

	// File permissions.
	dirPermissions = 0o755

	// Template type identifiers.
	templateTypeScaffold = "scaffold"

	// UI symbols and strings.
	bulletSymbol         = "•"
	skippedText          = "(skipped)"
	currentDirPrefix     = "./"
	newlineStr           = "\n"
	fileStatusFormat     = "  %s %s %s\n"
	failedWriteBlankLine = "Failed to write blank line"

	// Dry-run per-file status labels: the single authoritative signal
	// distinguishing a genuinely new file from an existing one that would be
	// merged. A real conflict is reported via the error branch instead (see
	// executeWithSetup), not one of these two labels.
	dryRunCreateStatus = "(would create)"
	dryRunUpdateStatus = "(would update)"
)

// fileExistsAt reports whether a file (not directory) already exists at
// targetPath/relativePath.
func fileExistsAt(targetPath, relativePath string) bool {
	info, err := os.Stat(filepath.Join(targetPath, relativePath))
	return err == nil && !info.IsDir()
}

// toEngineFile converts a template-loaded file (tmpl.File) to the shape the
// templating engine's Processor consumes (engine.File).
func toEngineFile(file tmpl.File) engine.File {
	return engine.File{
		Path:        file.Path,
		Content:     file.Content,
		IsTemplate:  file.IsTemplate,
		Permissions: file.Permissions,
	}
}

// fileConditionsByPath indexes a scaffold config's spec.files overlay by
// declared path, for O(1) lookup during the file-generation loop. Files not
// listed in the overlay generate unconditionally.
func fileConditionsByPath(scaffoldConfig *config.ScaffoldConfig) map[string]condition.Condition {
	whenByPath := make(map[string]condition.Condition, len(scaffoldConfig.Spec.Files))
	for _, f := range scaffoldConfig.Spec.Files {
		whenByPath[f.Path] = f.When
	}
	return whenByPath
}

// truncateString truncates a string to the specified length and adds "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// resolveDelimiters determines the delimiters to use. A scaffold config's own
// declared delimiters always win: callers (Execute/ExecuteWithBaseRef) pass a
// generic "{{"/"}}"  default through regardless of what the scaffold actually
// declares, so treating that default as an explicit override would silently
// ignore scaffold.yaml's spec.delimiters (e.g. "[[" / "]]") -- this is exactly
// what engine.extractDelimiters already does for per-file rendering via
// ProcessFile, which resolveDelimiters must match so the README summary
// renders with the same delimiters as every other generated file.
func resolveDelimiters(delimiters []string, scaffoldConfig *config.ScaffoldConfig) []string {
	if scaffoldConfig != nil && len(scaffoldConfig.Spec.Delimiters) == 2 {
		return scaffoldConfig.Spec.Delimiters
	}
	if len(delimiters) > 0 {
		return delimiters
	}
	return []string{"{{", "}}"}
}

// delimitersAsScaffoldConfig wraps activeDelimiters in the
// map[string]interface{} shape extractDelimiters understands (see
// tryExtractFromMapConfig in pkg/generator/engine/templating.go), so
// ProcessFile — which has no direct delimiters parameter and instead derives
// them from scaffoldConfig — honors them for templates with no
// *config.ScaffoldConfig of their own. Without this, custom delimiters are
// resolved but never actually reach rendering, always falling back to the
// default {{ / }} delimiters.
func delimitersAsScaffoldConfig(activeDelimiters []string) map[string]interface{} {
	return map[string]interface{}{
		"delimiters": []interface{}{activeDelimiters[0], activeDelimiters[1]},
	}
}

// calculateMaxColumnWidths finds maximum content widths for table columns from rows.
func calculateMaxColumnWidths(rows [][]string, nameWidth, sourceWidth, versionWidth, descWidth int) (int, int, int, int) {
	const minColumns = 4
	for _, row := range rows {
		if len(row) < minColumns {
			continue
		}
		if len(row[0]) > nameWidth {
			nameWidth = len(row[0])
		}
		if len(row[1]) > sourceWidth {
			sourceWidth = len(row[1])
		}
		if len(row[2]) > versionWidth {
			versionWidth = len(row[2])
		}
		if len(row[3]) > descWidth {
			descWidth = len(row[3])
		}
	}
	return nameWidth, sourceWidth, versionWidth, descWidth
}

// buildEmbedsTemplateOptions builds huh options from embedded configuration map.
func buildEmbedsTemplateOptions(configs map[string]tmpl.Configuration) []huh.Option[string] {
	// Build config keys for consistent ordering.
	var templateNames []string
	for key := range configs {
		templateNames = append(templateNames, key)
	}
	sort.Strings(templateNames)

	var options []huh.Option[string]
	for _, key := range templateNames {
		config := configs[key]
		displayText := fmt.Sprintf("%-15s   %-35s   %s", key, config.Name, config.Description)
		options = append(options, huh.NewOption(displayText, key))
	}
	return options
}

// buildScaffoldTemplateOptions builds huh options from scaffold templates in atmos.yaml.
func buildScaffoldTemplateOptions(templates interface{}) ([]huh.Option[string], []string) {
	templatesMap, ok := templates.(map[string]interface{})
	if !ok {
		return nil, nil
	}

	// Collect and sort names for deterministic ordering.
	sortedNames := make([]string, 0, len(templatesMap))
	for templateName := range templatesMap {
		sortedNames = append(sortedNames, templateName)
	}
	sort.Strings(sortedNames)

	var options []huh.Option[string]
	var templateNames []string

	for _, templateName := range sortedNames {
		displayText, valid := buildScaffoldDisplayText(templateName, templatesMap[templateName])
		if !valid {
			continue
		}
		options = append(options, huh.NewOption(displayText, templateName))
		templateNames = append(templateNames, templateName)
	}

	return options, templateNames
}

// buildScaffoldDisplayText constructs display text for a scaffold template.
func buildScaffoldDisplayText(templateName string, templateConfig interface{}) (string, bool) {
	templateMap, ok := templateConfig.(map[string]interface{})
	if !ok {
		return "", false
	}

	description := getStringFromMap(templateMap, "description")
	source := getStringFromMap(templateMap, "source")

	displayText := fmt.Sprintf("%-20s   %s", templateName, description)
	if source != "" {
		displayText += fmt.Sprintf(" (from %s)", source)
	}

	return displayText, true
}

// getStringFromMap safely extracts a string value from a map.
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// spinnerModel wraps the spinner for tea.Model compatibility.
type spinnerModel struct {
	spinner spinner.Model
	message string
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		default:
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) View() string {
	return fmt.Sprintf("\r%s %s", m.spinner.View(), m.message)
}

// InitUI handles the user interface for the init command.
type InitUI struct {
	checkmark    string
	xMark        string
	grayStyle    lipgloss.Style
	successStyle lipgloss.Style
	errorStyle   lipgloss.Style
	output       strings.Builder
	processor    *engine.Processor
	ioCtx        iolib.Context
	term         terminal.Terminal
	skipHooks    func(string) bool
}

// NewInitUI creates a new InitUI instance.
func NewInitUI(ioCtx iolib.Context, term terminal.Terminal) *InitUI {
	return &InitUI{
		checkmark:    "✓",
		xMark:        "✗",
		grayStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		output:       strings.Builder{},
		processor:    engine.NewProcessor(),
		ioCtx:        ioCtx,
		term:         term,
	}
}

// SetThreshold sets the threshold for merge operations.
func (ui *InitUI) SetThreshold(thresholdPercent int) {
	ui.processor.SetMaxChanges(thresholdPercent)
}

// SetDryRun toggles dry-run mode: rendering and 3-way merge still run (so
// real conflicts are reported), but no files are written to disk.
func (ui *InitUI) SetDryRun(dryRun bool) {
	ui.processor.SetDryRun(dryRun)
}

// SetConflictStrategy sets how a real ours/theirs divergence is resolved
// during a 3-way merge (manual/ours/theirs). The zero value
// (merge.ConflictStrategyManual) is today's existing behavior.
func (ui *InitUI) SetConflictStrategy(strategy merge.ConflictStrategy) {
	ui.processor.SetConflictStrategy(strategy)
}

// SetSkipHooks configures the --skip-hooks predicate (see
// hooks.NewSkipPredicate) consulted before running any scaffold hook. A nil
// predicate (the zero value) runs every hook, matching today's behavior for
// templates that don't set one.
func (ui *InitUI) SetSkipHooks(skip func(string) bool) {
	ui.skipHooks = skip
}

// GetTerminalWidth returns the current terminal width with a fallback.
func (ui *InitUI) GetTerminalWidth() int {
	width := ui.term.Width(terminal.Stdout)
	if width == 0 {
		return defaultTerminalWidth
	}
	return width
}

// writeOutput writes to the output buffer instead of using fmt.Printf.
func (ui *InitUI) writeOutput(format string, args ...interface{}) {
	fmt.Fprintf(&ui.output, format, args...)
}

// colorSource returns a colored string for the given source value.
func (ui *InitUI) colorSource(source string) string {
	styles := theme.GetCurrentStyles()
	if styles == nil {
		return source
	}
	switch source {
	case templateTypeScaffold:
		return styles.Command.Render("scaffold")
	case "flag":
		return styles.PackageName.Render("flag")
	default:
		return styles.Muted.Render("default")
	}
}

// flushOutput writes the accumulated output to stderr (UI channel) and clears the buffer.
// The buffered content is UI messages (configuration summaries, progress updates, etc.)
func (ui *InitUI) flushOutput() {
	atmosui.Write(ui.output.String())
	ui.output.Reset()
}

// Execute runs the initialization process with UI.
//
//nolint:revive // argument-limit: public API maintains compatibility
func (ui *InitUI) Execute(embedsConfig *tmpl.Configuration, targetPath string, force, update, useDefaults bool, cmdTemplateValues map[string]interface{}) error {
	return ui.ExecuteWithBaseRef(embedsConfig, targetPath, force, update, useDefaults, "", cmdTemplateValues)
}

// ExecuteWithBaseRef runs the initialization process with UI and specified base ref.
//
//nolint:revive // argument-limit: public API maintains compatibility
func (ui *InitUI) ExecuteWithBaseRef(embedsConfig *tmpl.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}) error {
	return ui.ExecuteWithDelimiters(embedsConfig, targetPath, force, update, useDefaults, baseRef, cmdTemplateValues, []string{"{{", "}}"})
}

// ExecuteWithDelimiters runs the initialization process with UI and custom delimiters.
//
//nolint:revive // argument-limit: public API maintains compatibility
func (ui *InitUI) ExecuteWithDelimiters(embedsConfig *tmpl.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}, delimiters []string) error {
	// Defensive validation: target directory cannot be empty
	if targetPath == "" {
		return errUtils.Build(errUtils.ErrTargetDirRequired).
			WithExplanation("Target directory cannot be empty").
			WithHint("Use ExecuteWithInteractiveFlow for prompting").
			Err()
	}

	// Early validation: check if target directory exists and handle appropriately
	if err := filesystem.ValidateTargetDirectory(targetPath, force, update); err != nil {
		return err
	}

	// Setup git storage for update mode
	if update && baseRef != "" {
		if err := ui.processor.SetupGitStorage(targetPath, baseRef); err != nil {
			return fmt.Errorf("failed to setup git storage: %w", err)
		}
	}

	ui.writeOutput("Generating %s in %s\n\n", embedsConfig.Name, targetPath)

	// Check if this configuration has a scaffold.yaml file (project schema)
	if tmpl.HasScaffoldConfig(embedsConfig.Files) {
		return ui.executeWithSetup(embedsConfig, targetPath, force, update, useDefaults, baseRef, cmdTemplateValues, delimiters)
	}

	// For templates without scaffold.yaml, use command-line values if provided.
	if len(cmdTemplateValues) > 0 {
		return ui.executeWithCommandValues(embedsConfig, targetPath, force, update, cmdTemplateValues, delimiters)
	}

	// For templates without scaffold.yaml and no command values, use empty values.
	return ui.executeWithCommandValues(embedsConfig, targetPath, force, update, make(map[string]interface{}), delimiters)
}

// ExecuteWithInteractiveFlow provides a unified flow for both init and scaffold commands.
// This ensures both commands have identical behavior - the only difference is the source of templates.
//
//nolint:revive // argument-limit: public API maintains compatibility
func (ui *InitUI) ExecuteWithInteractiveFlow(
	embedsConfig *tmpl.Configuration,
	targetPath string,
	force, update, useDefaults bool,
	cmdTemplateValues map[string]interface{},
) error {
	_, err := ui.ExecuteWithInteractiveFlowAndBaseRefResult(embedsConfig, targetPath, force, update, useDefaults, "", cmdTemplateValues)
	return err
}

// ExecuteWithInteractiveFlowAndBaseRef provides a unified flow with base ref support.
//
//nolint:revive // argument-limit: public API maintains compatibility
func (ui *InitUI) ExecuteWithInteractiveFlowAndBaseRef(
	embedsConfig *tmpl.Configuration,
	targetPath string,
	force, update, useDefaults bool,
	baseRef string,
	cmdTemplateValues map[string]interface{},
) error {
	_, err := ui.ExecuteWithInteractiveFlowAndBaseRefResult(embedsConfig, targetPath, force, update, useDefaults, baseRef, cmdTemplateValues)
	return err
}

// ExecuteWithInteractiveFlowResult provides the same flow as ExecuteWithInteractiveFlow
// and returns the final target path selected by the user.
//
//nolint:revive // argument-limit: public API mirrors ExecuteWithInteractiveFlow.
func (ui *InitUI) ExecuteWithInteractiveFlowResult(
	embedsConfig *tmpl.Configuration,
	targetPath string,
	force, update, useDefaults bool,
	cmdTemplateValues map[string]interface{},
) (string, error) {
	return ui.ExecuteWithInteractiveFlowAndBaseRefResult(embedsConfig, targetPath, force, update, useDefaults, "", cmdTemplateValues)
}

// ExecuteWithInteractiveFlowAndBaseRefResult provides the interactive flow with
// base ref support and returns the final target path.
//
//nolint:revive // argument-limit: public API mirrors ExecuteWithInteractiveFlowAndBaseRef.
func (ui *InitUI) ExecuteWithInteractiveFlowAndBaseRefResult(
	embedsConfig *tmpl.Configuration,
	targetPath string,
	force, update, useDefaults bool,
	baseRef string,
	cmdTemplateValues map[string]interface{},
) (string, error) {
	// If no target path was provided (interactive mode), prompt for it after setup.
	// For scaffold templates, promptForTargetPath also returns the values the user
	// already entered so we can avoid running the setup form a second time.
	if targetPath == "" {
		var preCollectedValues map[string]interface{}
		var err error
		targetPath, preCollectedValues, err = ui.promptForTargetPath(embedsConfig, useDefaults, cmdTemplateValues)
		if err != nil {
			return "", err
		}
		if preCollectedValues != nil {
			cmdTemplateValues, useDefaults, err = ui.resolvePreCollectedValues(
				embedsConfig, targetPath, update, useDefaults, preCollectedValues, cmdTemplateValues,
			)
			if err != nil {
				return targetPath, err
			}
		}
	}

	// Now execute with the determined target path. targetPath is returned even on
	// error so callers can retry against the same directory (e.g. offering an
	// update instead of a fresh generation when it already contains files).
	if err := ui.ExecuteWithBaseRef(embedsConfig, targetPath, force, update, useDefaults, baseRef, cmdTemplateValues); err != nil {
		return targetPath, err
	}
	return targetPath, nil
}

// resolvePreCollectedValues reconciles preCollectedValues (gathered by
// promptForTargetPath's temp-dir setup form run before the user picked a
// target) with cmdTemplateValues, now that the real targetPath is known.
// Returns the resolved cmdTemplateValues and useDefaults for the caller to use.
func (ui *InitUI) resolvePreCollectedValues(
	embedsConfig *tmpl.Configuration,
	targetPath string,
	update, useDefaults bool,
	preCollectedValues, cmdTemplateValues map[string]interface{},
) (map[string]interface{}, bool, error) {
	if update {
		// The setup form above ran against a throwaway temp directory (needed
		// to suggest a target directory name before the user picked one), so
		// it never saw config.LoadUserValues for the real target. Re-run setup
		// against the real targetPath instead of reusing those temp-dir-
		// collected values, so any config already saved at this target takes
		// precedence over fresh scaffold defaults, and (unlike the fresh-
		// generation branch below) useDefaults is left as the caller
		// requested rather than forced to true, so the user still gets a
		// chance to review/correct it.
		scaffoldConfig, cfgErr := ui.loadScaffoldConfigFromEmbeds(embedsConfig)
		if cfgErr != nil {
			return nil, useDefaults, cfgErr
		}
		mergedValues, _, setupErr := ui.RunSetupForm(scaffoldConfig, targetPath, useDefaults, cmdTemplateValues)
		if setupErr != nil {
			return nil, useDefaults, fmt.Errorf("failed to run setup form against target: %w", setupErr)
		}
		return mergedValues, useDefaults, nil
	}

	// Fresh generation: merge the pre-collected values into cmdTemplateValues
	// and use --use-defaults so executeWithSetup skips showing the form again.
	merged := make(map[string]interface{}, len(preCollectedValues)+len(cmdTemplateValues))
	for k, v := range preCollectedValues {
		merged[k] = v
	}
	// Caller-supplied flags take highest priority and overwrite.
	for k, v := range cmdTemplateValues {
		merged[k] = v
	}
	return merged, true, nil
}

// ConfirmUpdateInstead prompts whether to update an existing, non-empty target
// directory via a 3-way merge instead of failing outright. Callers should only
// invoke this after confirming a real TTY is available for prompting.
func (ui *InitUI) ConfirmUpdateInstead(targetPath string) (bool, error) {
	var confirmed bool
	prompt := uiutils.NewAtmosConfirm().
		Title(fmt.Sprintf("Directory `%s` already contains files. Update it instead (3-way merge)?", targetPath)).
		Affirmative("Yes, update").
		Negative("No, cancel").
		Value(&confirmed).
		WithTheme(uiutils.NewAtmosHuhTheme())

	if err := prompt.Run(); err != nil {
		return false, fmt.Errorf("confirmation prompt failed: %w", err)
	}
	return confirmed, nil
}

// promptForTargetPath handles interactive target path prompting with scaffold config support.
// It returns the target path, any pre-collected template values (non-nil only for scaffold
// templates whose setup form was already run), and any error.
func (ui *InitUI) promptForTargetPath(embedsConfig *tmpl.Configuration, useDefaults bool, cmdTemplateValues map[string]interface{}) (string, map[string]interface{}, error) {
	// For simple templates without scaffold config, prompt directly.
	if !tmpl.HasScaffoldConfig(embedsConfig.Files) {
		targetPath, err := ui.PromptForTargetDirectory(embedsConfig, nil)
		return targetPath, nil, err
	}

	// For templates with scaffold configuration, we need to run setup first to get proper values.
	return ui.promptForTargetPathWithScaffoldSetup(embedsConfig, useDefaults, cmdTemplateValues)
}

// promptForTargetPathWithScaffoldSetup runs scaffold setup and prompts for target directory.
// It returns the target path, the merged values collected from the setup form, and any error.
// Callers can pass the returned values as cmdTemplateValues on the subsequent Execute call
// to prevent the setup form from appearing a second time.
func (ui *InitUI) promptForTargetPathWithScaffoldSetup(embedsConfig *tmpl.Configuration, useDefaults bool, cmdTemplateValues map[string]interface{}) (string, map[string]interface{}, error) {
	// Create a temporary directory for setup.
	tempDir, err := os.MkdirTemp("", "atmos-setup-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Find and load the scaffold configuration.
	scaffoldConfig, err := ui.loadScaffoldConfigFromEmbeds(embedsConfig)
	if err != nil {
		return "", nil, err
	}

	// Run setup to get configuration values.
	mergedValues, _, err := ui.RunSetupForm(scaffoldConfig, tempDir, useDefaults, cmdTemplateValues)
	if err != nil {
		return "", nil, fmt.Errorf("failed to run setup form: %w", err)
	}

	// Prompt for target directory with evaluated template.
	targetPath, err := ui.PromptForTargetDirectory(embedsConfig, mergedValues)
	if err != nil {
		return "", nil, err
	}

	return targetPath, mergedValues, nil
}

// loadScaffoldConfigFromEmbeds finds and loads scaffold configuration from embedded files.
func (ui *InitUI) loadScaffoldConfigFromEmbeds(embedsConfig *tmpl.Configuration) (*config.ScaffoldConfig, error) {
	var scaffoldConfigFile *tmpl.File
	for i := range embedsConfig.Files {
		if embedsConfig.Files[i].Path == config.ScaffoldConfigFileName {
			scaffoldConfigFile = &embedsConfig.Files[i]
			break
		}
	}

	if scaffoldConfigFile == nil {
		return nil, errUtils.Build(errUtils.ErrScaffoldConfigMissing).
			WithExplanationf("%s not found in configuration", config.ScaffoldConfigFileName).
			Err()
	}

	scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldConfigFile.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to load scaffold configuration: %w", err)
	}

	return scaffoldConfig, nil
}

// generateSuggestedDirectoryWithValues generates a suggested directory name using template values.
func (ui *InitUI) generateSuggestedDirectoryWithValues(config *tmpl.Configuration, mergedValues map[string]interface{}) string {
	// If we have merged values, try to use them for a better suggestion.
	if mergedValues != nil {
		if name, ok := mergedValues["name"].(string); ok && name != "" {
			return currentDirPrefix + name
		}
		if projectName, ok := mergedValues["project_name"].(string); ok && projectName != "" {
			return currentDirPrefix + projectName
		}
	}

	// Fallback to the original logic.
	return currentDirPrefix + filepath.Base(config.Name)
}

// executeWithCommandValues processes files using command-line template values.
//
//nolint:revive // function-length: file processing loop with error handling
func (ui *InitUI) executeWithCommandValues(embedsConfig *tmpl.Configuration, targetPath string, force, update bool, cmdTemplateValues map[string]interface{}, delimiters []string) error {
	// Resolve delimiters, falling back to defaults when none are provided.
	activeDelimiters := resolveDelimiters(delimiters, nil)
	delimitersConfig := delimitersAsScaffoldConfig(activeDelimiters)

	// For now, use the existing processFile method but this should be refactored
	// to use the templating processor properly
	var successCount, errorCount int
	for _, file := range embedsConfig.Files {
		// Skip directory entries. The engine creates parent directories when
		// writing each file, so explicit directory entries are redundant and
		// would otherwise be written as empty files, breaking nested paths.
		if file.IsDirectory {
			continue
		}

		// Process the file using the templating processor.
		err := ui.processor.ProcessFile(toEngineFile(file), targetPath, force, update, delimitersConfig, cmdTemplateValues)

		// Display result using proper UI output
		if err != nil {
			// Check if this is a FileSkippedError
			skipErr := &engine.FileSkippedError{}
			if errors.As(err, &skipErr) {
				// File was intentionally skipped
				ui.writeOutput(fileStatusFormat,
					ui.grayStyle.Render(bulletSymbol),
					skipErr.Path,
					ui.grayStyle.Render(skippedText))
			} else {
				// Actual error occurred
				errorCount++
				ui.writeOutput(fileStatusFormat,
					ui.errorStyle.Render(ui.xMark),
					file.Path,
					ui.grayStyle.Render(fmt.Sprintf("(error: %v)", err)))
			}
		} else {
			successCount++
			ui.writeOutput("  %s %s\n",
				ui.successStyle.Render(ui.checkmark),
				file.Path)
		}
	}

	// Print summary
	ui.writeOutput(newlineStr)
	if errorCount > 0 {
		ui.writeOutput("Initialized %d files. Failed to initialize %d files.\n", successCount, errorCount)
		ui.flushOutput()
		return errUtils.Build(errUtils.ErrInitializationPartialFailure).
			WithExplanationf("Failed to initialize %d files", errorCount).
			Err()
	} else {
		ui.writeOutput("Initialized %d files.\n", successCount)
	}

	// Flush all output before rendering README
	ui.flushOutput()

	// Only render README if all files were successful.
	if embedsConfig.README != "" {
		if err := ui.renderREADME(embedsConfig.README, targetPath, activeDelimiters, cmdTemplateValues); err != nil {
			return err
		}
	}

	return nil
}

// RunSetupForm runs the interactive setup form to collect configuration values
// This method can be used by both init and scaffold commands.
func (ui *InitUI) RunSetupForm(scaffoldConfig *config.ScaffoldConfig, targetPath string, useDefaults bool, cmdTemplateValues map[string]interface{}) (map[string]interface{}, map[string]string, error) {
	// Load existing user values from the scaffold template directory
	userValues, err := config.LoadUserValues(targetPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load user values: %w", err)
	}

	// Deep merge project defaults with user values
	mergedValues := config.DeepMerge(scaffoldConfig, userValues)

	// Override with command-line config values (highest priority)
	for key, value := range cmdTemplateValues {
		mergedValues[key] = value
	}

	// Track value sources for display
	valueSources := make(map[string]string)

	// Start with all values as defaults
	for i := range scaffoldConfig.Spec.Fields {
		if _, exists := mergedValues[scaffoldConfig.Spec.Fields[i].Name]; exists {
			valueSources[scaffoldConfig.Spec.Fields[i].Name] = "default"
		}
	}

	// Mark values that came from existing config (scaffold) - these override defaults
	for key := range userValues {
		valueSources[key] = "scaffold"
	}

	// Override with command-line config values (highest priority)
	for key, value := range cmdTemplateValues {
		mergedValues[key] = value
		valueSources[key] = "flag"
	}

	// Debug: Log valueSources map.
	log.Debug("valueSources map", "valueSources", valueSources)

	// --set (and other external string sources) always supplies raw strings;
	// coerce boolean-typed fields (confirm/bool/boolean) to native bools so
	// When conditions and templates see the same type regardless of whether
	// the value came from a flag, an interactive prompt, or a YAML default.
	if err := config.CoerceFieldValueTypes(scaffoldConfig, mergedValues); err != nil {
		return nil, nil, errUtils.Build(err).
			WithExplanation("Boolean scaffold fields must be set to true or false").
			WithHint("Pass a valid value with --set <field>=true or --set <field>=false").
			WithExitCode(2).
			Err()
	}

	// Prompt the user to edit the configuration values unless --use-defaults is specified
	// This allows them to review and modify values from command line, config, or defaults
	if !useDefaults {
		if err := config.PromptForScaffoldConfig(scaffoldConfig, mergedValues); err != nil {
			return nil, nil, fmt.Errorf("failed to prompt for configuration: %w", err)
		}
	}

	// Validate the final merged answers for both interactive and non-interactive
	// generation. Prompt-level validation provides immediate feedback, while this
	// canonical check also covers defaults, persisted values, and --set values.
	if err := config.ValidateFieldValues(scaffoldConfig, mergedValues); err != nil {
		if errors.Is(err, errUtils.ErrGeneratorFieldRequired) {
			missing := config.MissingRequiredValues(scaffoldConfig, mergedValues)
			return nil, nil, errUtils.Build(err).
				WithExplanationf("Required fields have no value: `%s`", strings.Join(missing, "`, `")).
				WithHintf("Provide values with `--set %s=<value>`", missing[0]).
				WithHint("Or run interactively (in a terminal) to be prompted").
				WithContext("missing_fields", strings.Join(missing, ", ")).
				WithExitCode(2).
				Err()
		}
		return nil, nil, errUtils.Build(err).
			WithExplanation("Scaffold field validation failed").
			WithHint("Correct the invalid field value or update the scaffold configuration").
			WithExitCode(2).
			Err()
	}

	// Show configuration summary after any user input
	// Get configuration summary data and display it
	rows, header := config.GetConfigurationSummary(scaffoldConfig, mergedValues, valueSources)

	// Debug: Log valueSources to verify configuration sources.
	log.Debug("Configuration value sources", "valueSources", valueSources)

	ui.displayConfigurationTable(header, rows)

	// Flush the configuration summary before processing files
	ui.flushOutput()

	return mergedValues, valueSources, nil
}

// executeWithSetup handles any scaffold configuration with interactive prompts.
//
//nolint:gocognit,revive,cyclop,funlen // complex orchestration function with multiple setup phases
func (ui *InitUI) executeWithSetup(embedsConfig *tmpl.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}, delimiters []string) error {
	// Find the scaffold.yaml file in the configuration
	var scaffoldConfigFile *tmpl.File
	for i := range embedsConfig.Files {
		if embedsConfig.Files[i].Path == config.ScaffoldConfigFileName {
			scaffoldConfigFile = &embedsConfig.Files[i]
			break
		}
	}

	if scaffoldConfigFile == nil {
		return errUtils.Build(errUtils.ErrScaffoldConfigMissing).
			WithExplanationf("%s not found in rich-project configuration", config.ScaffoldConfigFileName).
			Err()
	}

	// Create directory if needed
	if err := os.MkdirAll(targetPath, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Load the scaffold configuration from embedded content (don't write to target folder)
	scaffoldConfig, err := config.LoadScaffoldConfigFromContent(scaffoldConfigFile.Content)
	if err != nil {
		if errors.Is(err, errUtils.ErrGeneratorValidation) {
			return errUtils.Build(err).
				WithExplanation("Scaffold configuration validation failed").
				WithHint("Correct the invalid field definition in scaffold.yaml").
				WithExitCode(2).
				Err()
		}
		return fmt.Errorf("failed to load scaffold configuration: %w", err)
	}

	// Run the setup form to collect configuration values.
	mergedValues, _, err := ui.RunSetupForm(scaffoldConfig, targetPath, useDefaults, cmdTemplateValues)
	if err != nil {
		return fmt.Errorf("failed to run setup form: %w", err)
	}

	scaffoldHooks, err := config.DecodeHooks(scaffoldConfig.Spec.Hooks)
	if err != nil {
		return fmt.Errorf("failed to decode scaffold hooks: %w", err)
	}

	// Run pre-generate hooks before any file is written: nothing has run yet
	// (status: success), and a hook failure aborts before any write happens,
	// so no rollback is needed.
	if err := scaffoldhooks.Run(scaffoldHooks, hooks.BeforeScaffoldGenerate, mergedValues, "success", ui.skipHooks); err != nil {
		return fmt.Errorf("pre-generate hook failed: %w", err)
	}

	// Process each file with rich configuration
	var successCount, errorCount int
	var failedFiles []string
	// Resolve once, with the same precedence ProcessFile's own extractDelimiters
	// uses (scaffoldConfig.Spec.Delimiters wins), so this preflight path-skip
	// check and the actual file-body rendering below never disagree.
	activeDelimiters := resolveDelimiters(delimiters, scaffoldConfig)
	whenByPath := fileConditionsByPath(scaffoldConfig)
	for _, file := range embedsConfig.Files {
		// Skip the scaffold.yaml as it's only used for schema definition
		if file.Path == config.ScaffoldConfigFileName {
			continue
		}

		// Skip directory entries. The engine creates parent directories when
		// writing each file, so explicit directory entries are redundant and
		// would otherwise be written as empty files, breaking nested paths.
		if file.IsDirectory {
			continue
		}

		// Declarative spec.files[].when: gates generation before any template
		// rendering happens, keyed by the file's original (undiscovered) path
		// -- distinct from the path-templating sentinel-skip check below,
		// which reacts to a *rendered* path evaluating to empty/false/null.
		if when, ok := whenByPath[file.Path]; ok && !when.Evaluate(condition.Context{Answers: mergedValues}) {
			ui.writeOutput(fileStatusFormat,
				ui.grayStyle.Render(bulletSymbol),
				file.Path,
				ui.grayStyle.Render(skippedText))
			continue
		}

		// Process the file path as a template first to check if it should be skipped
		renderedPath, pathErr := ui.processor.ProcessTemplateWithDelimiters(file.Path, targetPath, scaffoldConfig, mergedValues, activeDelimiters)
		if pathErr != nil {
			// If path processing fails, use original path
			renderedPath = file.Path
		}

		// Check if the rendered path should be skipped
		if ui.processor.ShouldSkipFile(renderedPath) {
			// File was intentionally skipped
			ui.writeOutput(fileStatusFormat,
				ui.grayStyle.Render(bulletSymbol),
				file.Path,
				ui.grayStyle.Render(skippedText))
			continue
		}

		// In dry-run mode, whether the file already exists is the single
		// authoritative signal for "would create" vs "would update": capture
		// it before ProcessFile runs (ProcessFile itself performs no write in
		// dry-run mode, so the file's existence is unaffected either way).
		existedBefore := ui.processor.DryRun && fileExistsAt(targetPath, renderedPath)

		// Use the templating processor to handle file processing.
		err := ui.processor.ProcessFile(toEngineFile(file), targetPath, force, update, scaffoldConfig, mergedValues)

		// Display result using proper UI output
		var skipErr *engine.FileSkippedError
		switch {
		case err == nil:
			successCount++
			if ui.processor.DryRun {
				// The single authoritative signal for this status label is
				// existedBefore, captured above -- not a separate boolean.
				status := dryRunCreateStatus
				if existedBefore {
					status = dryRunUpdateStatus
				}
				ui.writeOutput(fileStatusFormat,
					ui.successStyle.Render(ui.checkmark),
					renderedPath,
					ui.grayStyle.Render(status))
			} else {
				ui.writeOutput("  %s %s\n",
					ui.successStyle.Render(ui.checkmark),
					renderedPath)
			}
		case errors.As(err, &skipErr):
			// File was intentionally skipped
			ui.writeOutput(fileStatusFormat,
				ui.grayStyle.Render(bulletSymbol),
				skipErr.Path,
				ui.grayStyle.Render(skippedText))
		default:
			errorCount++
			failedFiles = append(failedFiles, file.Path)
			ui.writeOutput(fileStatusFormat,
				ui.errorStyle.Render(ui.xMark),
				renderedPath,
				ui.grayStyle.Render(fmt.Sprintf("(error: %v)", err)))
		}
	}

	// Print summary.
	ui.writeOutput(newlineStr)
	if errorCount > 0 {
		ui.writeOutput("Generated %d files. Failed to generate %d files.\n", successCount, errorCount)
		// Don't render README if there were errors - flush output and return error immediately.
		ui.flushOutput()
		// Post-generate hooks still get a chance to run on failure (e.g. a
		// cleanup step declaring when: always/failure); the implicit-success
		// default on an unconditioned hook skips it here, matching hooks
		// elsewhere in Atmos.
		if hookErr := scaffoldhooks.Run(scaffoldHooks, hooks.AfterScaffoldGenerate, mergedValues, "failure", ui.skipHooks); hookErr != nil {
			log.Warn("Post-generate hook failed", "error", hookErr)
		}
		return errUtils.Build(errUtils.ErrScaffoldGeneration).
			WithExplanationf("Failed to generate files: %s", strings.Join(failedFiles, ", ")).
			Err()
	} else {
		ui.writeOutput("Generated %d files.\n", successCount)
	}

	// Write the project record only after all files have been generated
	// successfully so a partial run does not leave the directory looking
	// fully initialised.
	if err := config.SaveProjectRecord(targetPath, scaffoldConfig, embedsConfig.Source, baseRef, mergedValues); err != nil {
		return fmt.Errorf("failed to save project record: %w", err)
	}

	// Run post-generate hooks after the project record is saved, so a hook
	// (e.g. `git add .`) sees the generated .atmos/scaffold.yaml record too.
	if err := scaffoldhooks.Run(scaffoldHooks, hooks.AfterScaffoldGenerate, mergedValues, "success", ui.skipHooks); err != nil {
		return fmt.Errorf("post-generate hook failed: %w", err)
	}

	// Flush all output before rendering README.
	ui.flushOutput()

	// Only render README if all files were successful.
	if embedsConfig.README != "" {
		// Resolve delimiters: use passed-in, or scaffold config, or defaults.
		delimiters = resolveDelimiters(delimiters, scaffoldConfig)

		// Process README template with rich configuration.
		processedContent, err := ui.processor.ProcessTemplateWithDelimiters(embedsConfig.README, targetPath, scaffoldConfig, mergedValues, delimiters)
		if err != nil {
			return fmt.Errorf("failed to process README template: %w", err)
		}

		// Render the processed content as markdown.
		if err := ui.renderMarkdown(processedContent); err != nil {
			return err
		}
	}

	return nil
}

// renderMarkdown renders markdown content to the UI channel (stderr).
// It goes through ui.MarkdownMessagef (backed by pkg/ui/markdown and
// pkg/terminal) rather than building a glamour renderer directly, so it
// honors Atmos's own TTY/color-forcing flags (ATMOS_FORCE_TTY, FORCE_COLOR,
// CLICOLOR_FORCE, NO_COLOR) instead of glamour's own raw os.Stdout.Fd() check,
// which otherwise falls back to a style that leaks literal markdown syntax
// (##, **, *) when stdout isn't a real TTY.
func (ui *InitUI) renderMarkdown(markdownContent string) error {
	atmosui.Writeln("")
	atmosui.MarkdownMessagef("%s", markdownContent)

	return nil
}

// renderREADME renders the README content as markdown.
// The provided delimiters control template rendering so callers that supply
// custom delimiters (e.g. via ExecuteWithDelimiters) are honoured correctly.
// Values are the collected/generated template values (e.g. cmdTemplateValues);
// without them, any `.Config.xxx` reference in the README silently resolves
// to nothing.
func (ui *InitUI) renderREADME(readmeContent string, targetPath string, delimiters []string, values map[string]interface{}) error {
	// Resolve delimiters, falling back to defaults.
	activeDelimiters := resolveDelimiters(delimiters, nil)

	// Process README template with the active delimiters and values.
	processedContent, err := ui.processor.ProcessTemplateWithDelimiters(readmeContent, targetPath, nil, values, activeDelimiters)
	if err != nil {
		return fmt.Errorf("failed to process README template: %w", err)
	}

	// Render the processed content as markdown.
	return ui.renderMarkdown(processedContent)
}

// columnWidths holds the calculated widths for table columns.
type columnWidths struct {
	setting int
	value   int
	source  int
}

// calculateColumnWidths computes the optimal width for each column based on content.
// It ensures minimum widths and adds padding for readability.
func (ui *InitUI) calculateColumnWidths(rows [][]string) columnWidths {
	widths := columnWidths{
		setting: settingColumnMinWidth, // Minimum width for setting names
		value:   valueColumnMinWidth,   // Minimum width for values
		source:  sourceColumnMinWidth,  // Minimum width for sources
	}

	// Find the maximum content width for each column
	for _, row := range rows {
		if len(row[0]) > widths.setting {
			widths.setting = len(row[0])
		}
		if len(row[1]) > widths.value {
			widths.value = len(row[1])
		}
		// For source column, account for colored strings
		coloredSource := ui.colorSource(row[2])
		if len(coloredSource) > widths.source {
			widths.source = len(coloredSource)
		}
	}

	// Add padding to each column
	widths.setting += 2
	widths.value += 2
	widths.source += 2

	return widths
}

// applyTableStyles applies consistent styling to the table including colors and borders.
func applyTableStyles(t *table.Model) {
	s := table.DefaultStyles()
	styles := theme.GetCurrentStyles()
	if styles == nil {
		t.SetStyles(s)
		return
	}
	s.Header = styles.TableHeader.
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.GetBorderColor())).
		BorderBottom(true)

	s.Cell = styles.TableRow
	s.Selected = styles.Selected

	t.SetStyles(s)
}

// prepareTableRows converts raw rows to table.Row format with colored sources.
func (ui *InitUI) prepareTableRows(rows [][]string) []table.Row {
	tableRows := make([]table.Row, 0, len(rows))
	for _, row := range rows {
		coloredSource := ui.colorSource(row[2])
		coloredRow := []string{row[0], row[1], coloredSource}
		tableRows = append(tableRows, table.Row(coloredRow))
	}
	return tableRows
}

// displayConfigurationTable displays configuration data in a formatted table.
func (ui *InitUI) displayConfigurationTable(_ []string, rows [][]string) {
	// Don't display table if there are no rows to show
	if len(rows) == 0 {
		return
	}

	// Get terminal width with fallback
	width := ui.term.Width(terminal.Stdout)
	if width == 0 {
		width = defaultTerminalWidth
	}
	tableWidth := width - tableMargin // Leave margin

	// Prepare table data
	tableRows := ui.prepareTableRows(rows)
	widths := ui.calculateColumnWidths(rows)

	// Calculate total width needed
	totalContentWidth := widths.setting + widths.value + widths.source + tableBorderPadding // for borders
	if totalContentWidth > tableWidth {
		tableWidth = totalContentWidth
	}

	// Create table
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Setting", Width: widths.setting},
			{Title: "Value", Width: widths.value},
			{Title: "Source", Width: widths.source},
		}),
		table.WithRows(tableRows),
		table.WithWidth(tableWidth),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)+1),
	)

	// Apply styling
	applyTableStyles(&t)

	// Print the table
	ui.writeOutput(newlineStr)
	ui.writeOutput("CONFIGURATION SUMMARY\n")
	ui.writeOutput(newlineStr)
	ui.writeOutput("%s\n", t.View())
	ui.writeOutput(newlineStr)
}

// DisplayTemplateTable displays template data in a formatted table.
//
//nolint:revive // complex table rendering with dynamic column widths
func (ui *InitUI) DisplayTemplateTable(header []string, rows [][]string) {
	// Get terminal width
	width := ui.term.Width(terminal.Stdout)
	if width == 0 {
		width = defaultTerminalWidth // fallback width
	}

	// Calculate table width (leave some margin)
	tableWidth := width - tableMargin

	// Convert rows to table.Row format
	var tableRows []table.Row
	for _, row := range rows {
		tableRows = append(tableRows, table.Row(row))
	}

	// Calculate column widths based on content
	nameWidth := nameColumnMinWidth       // Minimum width for template names
	sourceWidth := sourceColumnWidth      // Minimum width for source
	versionWidth := versionColumnMinWidth // Minimum width for version
	descWidth := descColumnMinWidth       // Minimum width for descriptions

	// Find the maximum content width for each column.
	nameWidth, sourceWidth, versionWidth, descWidth = calculateMaxColumnWidths(rows, nameWidth, sourceWidth, versionWidth, descWidth)

	// Add some padding to each column
	nameWidth += 2
	sourceWidth += 2
	versionWidth += 2
	descWidth += 2

	// Calculate total table width needed
	totalContentWidth := nameWidth + sourceWidth + versionWidth + descWidth + tableBorderSpacing // for borders and spacing

	// If content is wider than screen, use content width; otherwise use screen width
	if totalContentWidth > tableWidth {
		tableWidth = totalContentWidth
	}

	// Create table
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Template", Width: nameWidth},
			{Title: "Source", Width: sourceWidth},
			{Title: "Version", Width: versionWidth},
			{Title: "Description", Width: descWidth},
		}),
		table.WithRows(tableRows),
		table.WithWidth(tableWidth),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)+1), // Set explicit height to minimize spacing
	)

	applyTableStyles(&t)

	// Write the table to UI channel.
	atmosui.Writeln("")
	atmosui.Writeln("Available Scaffold Templates")
	atmosui.Writeln("")
	atmosui.Writeln(t.View())
	atmosui.Writeln("")
}

// PromptForTemplate prompts the user to select a template from available options.
// This works for both init (embeds) and scaffold (local/remote) templates.
//
//nolint:revive // complex TUI component with multiple template type handlers
func (ui *InitUI) PromptForTemplate(templateType string, templates interface{}) (string, error) {
	var options []huh.Option[string]

	switch templateType {
	case "embeds":
		// Handle tmpl.Configuration map.
		if configs, ok := templates.(map[string]tmpl.Configuration); ok {
			options = buildEmbedsTemplateOptions(configs)
		}

	case templateTypeScaffold:
		// Handle scaffold templates from atmos.yaml.
		scaffoldOptions, _ := buildScaffoldTemplateOptions(templates)
		options = append(options, scaffoldOptions...)
	}

	if len(options) == 0 {
		return "", errUtils.Build(errUtils.ErrScaffoldTemplatesNotAvailable).
			WithExplanation("No templates available").
			Err()
	}

	var selectedTemplate string

	// Create the selection form
	selectionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Select a %s template", templateType)).
				Description(fmt.Sprintf("Choose from the available %s templates (press 'q' to quit)", templateType)).
				Options(options...).
				Value(&selectedTemplate),
		),
	)

	err := selectionForm.Run()
	if err != nil {
		return "", err
	}

	// Display selected template details.
	atmosui.Writeln("")
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Padding(0, 1)

	atmosui.Writeln(descStyle.Render(fmt.Sprintf("Selected template: %s", selectedTemplate)))
	atmosui.Writeln("")

	return selectedTemplate, nil
}

// PromptForTargetDirectory prompts the user for the target directory with evaluated template values
// This works for both init and scaffold commands.
func (ui *InitUI) PromptForTargetDirectory(templateInfo interface{}, mergedValues map[string]interface{}) (string, error) {
	// Generate suggested directory name based on template and values
	suggestedDir := ui.generateSuggestedDirectoryWithTemplateInfo(templateInfo, mergedValues)
	targetPath := suggestedDir

	// Form to get target directory with smart default
	pathForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target directory").
				Description(fmt.Sprintf("Where should the files be created? (suggested: %s)", suggestedDir)).
				Placeholder(suggestedDir).
				Value(&targetPath).
				Validate(func(s string) error {
					if s == "" {
						return nil // Empty is OK, will use suggested default
					}
					return nil
				}),
		),
	)

	err := pathForm.Run()
	if err != nil {
		return "", err
	}

	// Use suggested directory if empty
	if targetPath == "" {
		targetPath = suggestedDir
	}

	return targetPath, nil
}

// generateSuggestedDirectoryWithTemplateInfo generates a suggested directory name using template info and values.
func (ui *InitUI) generateSuggestedDirectoryWithTemplateInfo(templateInfo interface{}, mergedValues map[string]interface{}) string {
	// If we have merged values, try to use them for a better suggestion
	if mergedValues != nil {
		if name, ok := mergedValues["name"].(string); ok && name != "" {
			return currentDirPrefix + name
		}
		if projectName, ok := mergedValues["project_name"].(string); ok && projectName != "" {
			return currentDirPrefix + projectName
		}
	}

	// Try to extract name from template info
	switch info := templateInfo.(type) {
	case tmpl.Configuration:
		return currentDirPrefix + filepath.Base(info.Name)
	case map[string]interface{}:
		if name, ok := info["name"].(string); ok && name != "" {
			return currentDirPrefix + name
		}
	}

	// Fallback
	return currentDirPrefix + "new-project"
}

// DisplayScaffoldTemplateTable displays scaffold templates in a table format.
func (ui *InitUI) DisplayScaffoldTemplateTable(templatesMap map[string]interface{}) {
	// Collect sorted template names for deterministic row order.
	sortedNames := make([]string, 0, len(templatesMap))
	for templateName := range templatesMap {
		sortedNames = append(sortedNames, templateName)
	}
	sort.Strings(sortedNames)

	// Extract template data for table display in sorted order.
	var rows [][]string
	for _, templateName := range sortedNames {
		templateConfig := templatesMap[templateName]
		templateMap, ok := templateConfig.(map[string]interface{})
		if !ok {
			continue
		}

		// Get template source.
		source, _ := templateMap["source"].(string)
		if source == "" {
			source = "unknown"
		}

		// Get template description (if available).
		description := ""
		if desc, ok := templateMap["description"].(string); ok {
			description = desc
		}

		// Get template ref (if available).
		ref := ""
		if r, ok := templateMap["ref"].(string); ok {
			ref = r
		}

		rows = append(rows, []string{templateName, source, ref, description})
	}

	header := []string{"Template", "Source", "Version", "Description"}
	ui.DisplayTemplateTable(header, rows)
}
