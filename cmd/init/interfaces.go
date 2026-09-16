package initcmd

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/generator/merge"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
)

// InitUI is the subset of *generatorUI.InitUI's behavior the init command
// depends on, extracted so tests can substitute a mock instead of driving
// the real interactive TUI (prompts, huh forms) end to end. Mirrors
// cmd/scaffold's ScaffoldUI, which solves the same problem for the sibling
// command.
type InitUI interface {
	SetConflictStrategy(strategy merge.ConflictStrategy)
	SetMergeDriver(driver merge.Driver)
	SetSkipHooks(skip func(string) bool)
	PromptForTemplate(templateType string, templates interface{}) (string, error)
	ExecuteWithBaseRef(embedsConfig *templates.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}) error
	ExecuteWithInteractiveFlowAndBaseRefResult(embedsConfig *templates.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}) (string, error)
	ResolveTargetPath(embedsConfig *templates.Configuration, targetPath string, update, useDefaults bool, cmdTemplateValues map[string]interface{}) (string, map[string]interface{}, bool, error)
	ConfirmUpdateInstead(targetPath string) (bool, error)
}

// Compile-time check that *generatorUI.InitUI satisfies InitUI.
var _ InitUI = (*generatorUI.InitUI)(nil)
