package linters

import (
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestMultilineMarkdownExampleRule(t *testing.T) {
	// Create a dedicated analyzer for this test that only runs the MultilineMarkdownExampleRule.
	testAnalyzer := &analysis.Analyzer{
		Name: "multilinemarkdownexampletest",
		Doc:  "Test analyzer for MultilineMarkdownExampleRule",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			rule := &MultilineMarkdownExampleRule{}
			for _, file := range pass.Files {
				if err := rule.Check(pass, file); err != nil {
					return nil, err
				}
			}
			return nil, nil
		},
	}

	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, testAnalyzer, "multiline_example")
}
