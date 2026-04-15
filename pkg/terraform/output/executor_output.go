package output

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// handleDisabledComponent returns empty outputs for disabled or abstract components.
func handleDisabledComponent(component, stack string, _, abstract bool) map[string]any {
	status := "disabled"
	if abstract {
		status = "abstract"
	}
	log.Debug("Skipping terraform output due to component status", "component", component, "stack", stack, "status", status)
	return map[string]any{}
}

// processOutputs converts tfexec.OutputMeta to map[string]any.
func processOutputs(outputMeta map[string]tfexec.OutputMeta, atmosConfig *schema.AtmosConfiguration) map[string]any {
	defer perf.Track(atmosConfig, "output.processOutputs")()

	return lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
		s := string(v.Value)

		// Log summary to avoid multiline value formatting issues.
		valueSummary := summarizeValue(s)
		log.Debug("Converting output from JSON to Go data type", "key", k, "value_summary", valueSummary)

		d, err := u.ConvertFromJSON(s)
		if err != nil {
			log.Error("Failed to convert output", "key", k, "error", err)
			return k, nil
		}

		return k, d
	})
}

// summarizeValue creates a summary for logging long or multiline values.
func summarizeValue(s string) string {
	if strings.Contains(s, "\n") {
		lineCount := strings.Count(s, "\n") + 1
		return fmt.Sprintf("<multiline: %d lines, %d bytes>", lineCount, len(s))
	}
	if len(s) > maxLogValueLen {
		return s[:maxLogValueLen] + "..."
	}
	return s
}

// extractYqValue extracts a value from a map using yq expression.
// It returns the extracted value, whether the key exists, and any error.
func extractYqValue(
	atmosConfig *schema.AtmosConfiguration,
	data map[string]any,
	output string,
	errContext string,
) (any, bool, error) {
	// Use yq to extract the value (handles nested paths, alternative operators, etc.).
	val := output
	if !strings.HasPrefix(output, dotSeparator) {
		val = dotSeparator + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, data, val)
	if err != nil {
		return nil, false, fmt.Errorf("failed to evaluate %s: %w", errContext, err)
	}

	// Check if this is a simple key lookup (no yq operators).
	hasYqOperators := strings.Contains(output, "//") ||
		strings.Contains(output, "|") ||
		strings.Contains(output, "=") ||
		strings.Contains(output, "[") ||
		strings.Contains(output, "]")

	if !hasYqOperators {
		outputKey := strings.TrimPrefix(output, dotSeparator)
		if !strings.Contains(outputKey, dotSeparator) {
			_, exists := data[outputKey]
			if !exists {
				return nil, false, nil
			}
		}
	}

	return res, true, nil
}

// getOutputVariable extracts a specific output variable using yq expression.
func getOutputVariable(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	outputs map[string]any,
	output string,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "output.getOutputVariable")()

	errContext := fmt.Sprintf("terraform output for component %s in stack %s", component, stack)
	return extractYqValue(atmosConfig, outputs, output, errContext)
}

// GetStaticRemoteStateOutput extracts a specific output from static remote state.
// This is exported for use by terraform_state_utils.go and other callers that need
// to extract values from static remote state sections.
func GetStaticRemoteStateOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) (any, bool, error) {
	defer perf.Track(atmosConfig, "output.GetStaticRemoteStateOutput")()

	errContext := fmt.Sprintf("static remote state for component %s in stack %s", component, stack)
	return extractYqValue(atmosConfig, remoteStateSection, output, errContext)
}
