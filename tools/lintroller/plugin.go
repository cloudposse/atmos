package linters

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("lintroller", New)
}

// Analyzer is a standalone analyzer for CLI usage.
var Analyzer = &analysis.Analyzer{
	Name: "lintroller",
	Doc:  "Atmos project-specific linting rules (t.Setenv/os.Setenv checks)",
	Run:  standaloneRun,
}

// standaloneRun runs all rules for the standalone CLI tool.
func standaloneRun(pass *analysis.Pass) (interface{}, error) {
	// Run all rules (both enabled by default).
	rules := []Rule{
		&TSetenvInDeferRule{},
		&OsSetenvInTestRule{},
	}

	for _, file := range pass.Files {
		for _, rule := range rules {
			if err := rule.Check(pass, file); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
}

// Settings for the lintroller plugin.
type Settings struct {
	TSetenvInDefer bool `json:"tsetenv-in-defer" yaml:"tsetenv-in-defer"`
	OsSetenvInTest bool `json:"os-setenv-in-test" yaml:"os-setenv-in-test"`
}

// LintrollerPlugin implements the register.LinterPlugin interface.
type LintrollerPlugin struct {
	settings Settings
}

// New returns a new instance of the lintroller plugin.
func New(settings any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[Settings](settings)
	if err != nil {
		return nil, err
	}

	// Default to enabling all rules if no settings provided.
	if !s.TSetenvInDefer && !s.OsSetenvInTest {
		s.TSetenvInDefer = true
		s.OsSetenvInTest = true
	}

	return &LintrollerPlugin{settings: s}, nil
}

// BuildAnalyzers returns the analyzers for golangci-lint.
func (p *LintrollerPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "lintroller",
			Doc:  "Atmos project-specific linting rules (t.Setenv/os.Setenv checks)",
			Run:  p.run,
		},
	}, nil
}

// GetLoadMode returns the load mode for the analyzer.
func (p *LintrollerPlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

// run executes the lintroller analyzer.
func (p *LintrollerPlugin) run(pass *analysis.Pass) (interface{}, error) {
	// Get enabled rules based on settings.
	var rules []Rule
	if p.settings.TSetenvInDefer {
		rules = append(rules, &TSetenvInDeferRule{})
	}
	if p.settings.OsSetenvInTest {
		rules = append(rules, &OsSetenvInTestRule{})
	}

	// Run all enabled rules.
	for _, file := range pass.Files {
		for _, rule := range rules {
			if err := rule.Check(pass, file); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
}
