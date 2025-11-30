package linters

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "a")
}

func TestPerfTrackRule(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "perftrack")
}
