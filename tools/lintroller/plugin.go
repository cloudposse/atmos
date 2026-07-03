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
	Doc:  "Atmos project-specific linting rules (t.Setenv/os.Setenv/t.TempDir/t.Chdir/os.Args/perf.Track/TestKit checks)",
	Run:  standaloneRun,
}

// standaloneRun runs all rules for the standalone CLI tool.
func standaloneRun(pass *analysis.Pass) (interface{}, error) {
	// Run all rules (all enabled by default).
	rules := []Rule{
		&TSetenvInDeferRule{},
		&OsSetenvInTestRule{},
		&OsMkdirTempInTestRule{},
		&OsChdirInTestRule{},
		&OsArgsInTestRule{},
		&PerfTrackRule{},
		&TestKitRule{},
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
	TSetenvInDefer    bool `json:"tsetenv-in-defer" yaml:"tsetenv-in-defer"`
	OsSetenvInTest    bool `json:"os-setenv-in-test" yaml:"os-setenv-in-test"`
	OsMkdirTempInTest bool `json:"os-mkdirtemp-in-test" yaml:"os-mkdirtemp-in-test"`
	OsChdirInTest     bool `json:"os-chdir-in-test" yaml:"os-chdir-in-test"`
	OsArgsInTest      bool `json:"os-args-in-test" yaml:"os-args-in-test"`
	PerfTrack         bool `json:"perf-track" yaml:"perf-track"`
	TestKitRequired   bool `json:"testkit-required" yaml:"testkit-required"`
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
	if !s.TSetenvInDefer && !s.OsSetenvInTest && !s.OsMkdirTempInTest && !s.OsChdirInTest && !s.OsArgsInTest && !s.PerfTrack && !s.TestKitRequired {
		s.TSetenvInDefer = true
		s.OsSetenvInTest = true
		s.OsMkdirTempInTest = true
		s.OsChdirInTest = true
		s.OsArgsInTest = true
		s.PerfTrack = true
		s.TestKitRequired = true
	}

	return &LintrollerPlugin{settings: s}, nil
}

// BuildAnalyzers returns the analyzers for golangci-lint.
func (p *LintrollerPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "lintroller",
			Doc:  "Atmos project-specific linting rules (t.Setenv/os.Setenv/t.TempDir/t.Chdir/os.Args/perf.Track/TestKit checks)",
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
	if p.settings.OsMkdirTempInTest {
		rules = append(rules, &OsMkdirTempInTestRule{})
	}
	if p.settings.OsChdirInTest {
		rules = append(rules, &OsChdirInTestRule{})
	}
	if p.settings.OsArgsInTest {
		rules = append(rules, &OsArgsInTestRule{})
	}
	if p.settings.PerfTrack {
		rules = append(rules, &PerfTrackRule{})
	}
	if p.settings.TestKitRequired {
		rules = append(rules, &TestKitRule{})
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
