package utils

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrintAsJson(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled:                true,
					Formatter:              "terminal",
					Theme:                  "dracula",
					HighlightedOutputPager: true,
					LineNumbers:            true,
					Wrap:                   false,
				},
			},
		},
	}

	data := map[string]any{
		"key": "value",
	}

	err := PrintAsJSON(atmosConfig, data)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
