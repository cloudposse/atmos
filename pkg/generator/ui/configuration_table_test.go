package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"

	atmosui "github.com/cloudposse/atmos/pkg/ui"
)

func TestConfigurationSummaryFitsTerminalWithoutLosingValues(t *testing.T) {
	values := []struct{ key, value string }{
		{"description", "OIDC login with a downstream credential broker and reusable authentication sessions"},
		{"sdk_path", "/workspace/a-very-long-checkout-directory/.conductor/plugin-manual/sdk"},
		{"description", "Custom 認証 provider with Unicode metadata and a downstream exchange"},
	}
	for _, width := range []int{40, 80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Setenv("COLUMNS", strconv.Itoa(width))
			t.Setenv("NO_COLOR", "1")
			previous := atmosui.GetColorProfile()
			atmosui.SetColorProfile(termenv.Ascii)
			t.Cleanup(func() { atmosui.SetColorProfile(previous) })
			generator := createTestUI(t)
			for _, value := range values {
				generator.output.Reset()
				generator.displayConfigurationTable([]string{"Setting", "Value", "Source"}, [][]string{{value.key, value.value, "flag"}})
				output := generator.output.String()
				require.NotContains(t, output, "\x1b")
				lines := strings.Split(output, "\n")
				valueColumn, sourceColumn, bodyLine := 0, 0, 0
				for index, line := range lines {
					require.LessOrEqual(t, ansi.StringWidth(line), width, "line %q", line)
					valueIndex, sourceIndex := strings.Index(line, "Value"), strings.Index(line, "Sour")
					if valueIndex >= 0 && sourceIndex >= 0 {
						valueColumn = ansi.StringWidth(line[:valueIndex])
						sourceColumn = ansi.StringWidth(line[:sourceIndex])
					}
					if strings.HasPrefix(line, "──") {
						bodyLine = index + 1
					}
				}
				require.Positive(t, valueColumn)
				require.Greater(t, sourceColumn, valueColumn)
				var displayed strings.Builder
				for _, line := range lines[bodyLine:] {
					displayed.WriteString(ansi.Cut(line, valueColumn, sourceColumn-1))
				}
				require.Equal(t, strings.Join(strings.Fields(value.value), ""), strings.Join(strings.Fields(displayed.String()), ""), "summary must wrap complete values")
			}
		})
	}
}

func TestConfigurationSummaryEmpty(t *testing.T) {
	generator := createTestUI(t)
	generator.displayConfigurationTable(nil, nil)
	require.Empty(t, generator.output.String())
}
