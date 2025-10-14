package main

import (
	"flag"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

var (
	enableTSetenvInDefer = flag.Bool("tsetenv-in-defer", true, "check for t.Setenv in defer/cleanup blocks")
	enableOsSetenvInTest = flag.Bool("os-setenv-in-test", true, "check for os.Setenv in test files")
)

// rules contains all available linting rules.
var rules = []Rule{
	&TSetenvInDeferRule{},
	&OsSetenvInTestRule{},
}

var Analyzer = &analysis.Analyzer{
	Name:  "lintroller",
	Doc:   "Atmos project-specific linting rules",
	Run:   run,
	Flags: *flag.NewFlagSet("lintroller", flag.ExitOnError),
}

func init() {
	Analyzer.Flags.BoolVar(enableTSetenvInDefer, "tsetenv-in-defer", true, "check for t.Setenv in defer/cleanup blocks")
	Analyzer.Flags.BoolVar(enableOsSetenvInTest, "os-setenv-in-test", true, "check for os.Setenv in test files")
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		// Run enabled rules.
		for _, rule := range rules {
			enabled := isRuleEnabled(rule.Name())
			if enabled {
				if err := rule.Check(pass, file); err != nil {
					return nil, err
				}
			}
		}
	}

	return nil, nil
}

// isRuleEnabled checks if a rule is enabled based on flags.
func isRuleEnabled(ruleName string) bool {
	switch ruleName {
	case "tsetenv-in-defer":
		return *enableTSetenvInDefer
	case "os-setenv-in-test":
		return *enableOsSetenvInTest
	default:
		return false
	}
}

func main() {
	singlechecker.Main(Analyzer)
}
