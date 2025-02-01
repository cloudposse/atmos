package templates

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/fatih/color"
)

// ProcessWithGoTemplate takes input data and applies a Go template to it
func ProcessWithGoTemplate(input interface{}, templateStr string) (string, error) {
	// If no template is provided, return empty string
	if templateStr == "" {
		return "", nil
	}

	// Create a new template with Sprig functions
	tmpl := template.New("output").Funcs(sprig.FuncMap()).Funcs(template.FuncMap{
		// Add custom functions similar to gh CLI
		"autocolor": func(text string) string {
			if color.NoColor {
				return text
			}
			return theme.Colors.Success.Sprint(text)
		},
		"color": func(style string, text string) string {
			switch style {
			case "success":
				return theme.Colors.Success.Sprint(text)
			case "error":
				return theme.Colors.Error.Sprint(text)
			case "warning":
				return theme.Colors.Warning.Sprint(text)
			case "info":
				return theme.Colors.Info.Sprint(text)
			default:
				return text
			}
		},
		"tablerow": func(fields ...interface{}) string {
			var result bytes.Buffer
			for i, field := range fields {
				if i > 0 {
					result.WriteString("  ")
				}
				result.WriteString(fmt.Sprintf("%v", field))
			}
			result.WriteString("\n")
			return result.String()
		},
		"tablerender": func() string {
			return "" // Placeholder for table rendering
		},
	})

	// Parse the template
	tmpl, err := tmpl.Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %w", err)
	}

	// Execute the template
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, input)
	if err != nil {
		return "", fmt.Errorf("error executing template: %w", err)
	}

	return buf.String(), nil
}
