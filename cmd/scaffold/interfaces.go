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
	SetDryRun(dryRun bool)
	SetSkipHooks(skip func(string) bool)
	PromptForTemplate(templateType string, templates interface{}) (string, error)
	DisplayTemplateTable(header []string, rows [][]string)
	ExecuteWithBaseRef(embedsConfig *templates.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}) error
	ExecuteWithInteractiveFlowAndBaseRefResult(embedsConfig *templates.Configuration, targetPath string, force, update, useDefaults bool, baseRef string, cmdTemplateValues map[string]interface{}) (string, error)
	ConfirmUpdateInstead(targetPath string) (bool, error)
}

// Compile-time check that *generatorUI.InitUI satisfies ScaffoldUI.
var _ ScaffoldUI = (*generatorUI.InitUI)(nil)
