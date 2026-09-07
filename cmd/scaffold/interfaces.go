package scaffold

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
)

// ScaffoldUI is the subset of *generatorUI.InitUI's behavior the scaffold
// command depends on, extracted so tests can substitute a mock instead of
// driving the real interactive TUI (prompts, huh forms) end to end.
type ScaffoldUI interface {
	SetConflictStrategy(strategy merge.ConflictStrategy)
	// SetMergeDriver selects the merger used by scaffold updates (YAML-aware
	// auto-detection vs. forcing the line-oriented text merger).
	SetMergeDriver(driver merge.Driver)
	SetDryRun(dryRun bool)
	SetSkipHooks(skip func(string) bool)
	PromptForTemplate(templateType string, templates interface{}) (string, error)
	DisplayTemplateTable(header []string, rows [][]string)
	ExecuteWithBaseRef(embedsConfig *templates.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}) error
	ExecuteWithInteractiveFlowAndBaseRefResult(embedsConfig *templates.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}) (string, error)
	// ResolveTargetPath resolves (prompting interactively when targetPath is
	// "") the final target directory generation will use, without running
	// generation itself. Callers that need the real target directory before
	// resolving other per-target inputs (e.g. the --update base ref) can call
	// this first, then pass the result back as targetPath to one of the
	// Execute* methods to skip prompting a second time.
	ResolveTargetPath(embedsConfig *templates.Configuration, targetPath string, update, useDefaults bool, cmdTemplateValues map[string]interface{}) (string, map[string]interface{}, bool, error)
	ConfirmUpdateInstead(targetPath string) (bool, error)
}

// Compile-time check that *generatorUI.InitUI satisfies ScaffoldUI.
var _ ScaffoldUI = (*generatorUI.InitUI)(nil)
