// Package terraform provides the CI provider implementation for Terraform.
package terraform

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Regular expressions for parsing terraform stdout (errors/warnings only).
// These are compiled at package initialization and are safe for concurrent use
// as regexp.Regexp is immutable after compilation.
var (
	// Matches error messages in terraform output.
	errorRe = regexp.MustCompile(`(?m)^(?:[│|] )?Error:\s*(.+)$`)

	// Matches warning messages in terraform output.
	warningRe = regexp.MustCompile(`(?m)^(?:[│|] )?Warning:\s*(.+)$`)

	// Matches apply summary: "Apply complete! Resources: X added, Y changed, Z destroyed."
	// Used as fallback when JSON is not available.
	applySummaryRe = regexp.MustCompile(`Apply complete!\s*Resources:\s*(\d+)\s+added,\s*(\d+)\s+changed,\s*(\d+)\s+destroyed`)

	// Matches "Plan: X to add, Y to change, Z to destroy."
	// Used as fallback when JSON is not available.
	planSummaryRe = regexp.MustCompile(`Plan:\s*(\d+)\s+to add,\s*(\d+)\s+to change,\s*(\d+)\s+to destroy`)

	// NoChangesRe matches "No changes. Your infrastructure matches the configuration.".
	noChangesRe = regexp.MustCompile(`No changes\.|Your infrastructure matches the configuration`)
)

// ParsePlanJSON parses terraform plan JSON from `terraform show -json <planfile>`.
// This is the preferred method for parsing plan data as it provides structured data.
func ParsePlanJSON(jsonData []byte) (*plugin.OutputResult, error) {
	defer perf.Track(nil, "terraform.ParsePlanJSON")()

	var plan tfjson.Plan
	if err := json.Unmarshal(jsonData, &plan); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal plan JSON: %w", errUtils.ErrParseFile, err)
	}

	result := newEmptyResult()
	data := result.Data.(*plugin.TerraformOutputData)

	processResourceChanges(plan.ResourceChanges, data)
	processOutputChanges(plan.OutputChanges, data)

	result.HasChanges = hasResourceChanges(data.ResourceCounts)
	if result.HasChanges {
		data.ChangedResult = buildChangeSummary(data.ResourceCounts)
	}

	return result, nil
}

// newEmptyResult creates an empty OutputResult with initialized data.
func newEmptyResult() *plugin.OutputResult {
	return &plugin.OutputResult{
		ExitCode:   0,
		HasChanges: false,
		HasErrors:  false,
		Errors:     nil,
		Data: &plugin.TerraformOutputData{
			ResourceCounts:    plugin.ResourceCounts{},
			CreatedResources:  []string{},
			UpdatedResources:  []string{},
			ReplacedResources: []string{},
			DeletedResources:  []string{},
			MovedResources:    []plugin.MovedResource{},
			ImportedResources: []string{},
			Outputs:           make(map[string]plugin.TerraformOutput),
		},
	}
}

// processResourceChanges processes plan resource changes and updates the data.
func processResourceChanges(changes []*tfjson.ResourceChange, data *plugin.TerraformOutputData) {
	for _, rc := range changes {
		if rc == nil || rc.Change == nil {
			continue
		}
		processResourceChange(rc, data)
	}
}

// processResourceChange processes a single resource change.
func processResourceChange(rc *tfjson.ResourceChange, data *plugin.TerraformOutputData) {
	addr := rc.Address
	actions := rc.Change.Actions

	switch {
	case actions.Create():
		data.ResourceCounts.Create++
		data.CreatedResources = append(data.CreatedResources, addr)
	case actions.Delete():
		data.ResourceCounts.Destroy++
		data.DeletedResources = append(data.DeletedResources, addr)
	case actions.Update():
		data.ResourceCounts.Change++
		data.UpdatedResources = append(data.UpdatedResources, addr)
	case actions.Replace():
		data.ResourceCounts.Replace++
		data.ReplacedResources = append(data.ReplacedResources, addr)
	}

	if rc.Change.Importing != nil {
		data.ImportedResources = append(data.ImportedResources, addr)
	}
}

// processOutputChanges processes plan output changes.
func processOutputChanges(changes map[string]*tfjson.Change, data *plugin.TerraformOutputData) {
	for name, output := range changes {
		if output == nil {
			continue
		}
		data.Outputs[name] = plugin.TerraformOutput{
			Value:     output.After,
			Sensitive: output.AfterSensitive != nil,
		}
	}
}

// hasResourceChanges checks if there are any resource changes.
func hasResourceChanges(counts plugin.ResourceCounts) bool {
	return counts.Create > 0 || counts.Change > 0 || counts.Replace > 0 || counts.Destroy > 0
}

// OutputJSON represents the structure of `terraform output -json`.
type OutputJSON map[string]OutputValue

// OutputValue represents a single output value from `terraform output -json`.
type OutputValue struct {
	Sensitive bool `json:"sensitive"`
	Type      any  `json:"type"`
	Value     any  `json:"value"`
}

// ParseOutputJSON parses terraform output JSON from `terraform output -json`.
// Returns a map of output name to TerraformOutput.
func ParseOutputJSON(jsonData []byte) (map[string]plugin.TerraformOutput, error) {
	defer perf.Track(nil, "terraform.ParseOutputJSON")()

	var outputs OutputJSON
	if err := json.Unmarshal(jsonData, &outputs); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal output JSON: %w", errUtils.ErrParseFile, err)
	}

	result := make(map[string]plugin.TerraformOutput)
	for name, out := range outputs {
		result[name] = plugin.TerraformOutput{
			Value:     out.Value,
			Type:      formatType(out.Type),
			Sensitive: out.Sensitive,
		}
	}

	return result, nil
}

// formatType converts terraform type to a string representation.
func formatType(t any) string {
	if t == nil {
		return ""
	}
	switch v := t.(type) {
	case string:
		return v
	case []any:
		// Complex types like ["object", {"key": "type"}].
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
		return "complex"
	default:
		return "unknown"
	}
}

// ExtractErrors extracts error messages from terraform stdout.
// This is used when JSON parsing is not available or for runtime errors.
func ExtractErrors(stdout string) []string {
	defer perf.Track(nil, "terraform.ExtractErrors")()

	var errors []string
	matches := errorRe.FindAllStringSubmatch(stdout, -1)
	for _, match := range matches {
		if len(match) > 1 {
			errors = append(errors, strings.TrimSpace(match[1]))
		}
	}
	return errors
}

// ExtractWarnings extracts warning messages from terraform stdout.
func ExtractWarnings(stdout string) []string {
	defer perf.Track(nil, "terraform.ExtractWarnings")()

	var warnings []string
	matches := warningRe.FindAllStringSubmatch(stdout, -1)
	for _, match := range matches {
		if len(match) > 1 {
			warnings = append(warnings, strings.TrimSpace(match[1]))
		}
	}
	return warnings
}

// ExtractWarningBlocks extracts full warning blocks from terraform stdout.
// Unlike ExtractWarnings which returns only summary lines, this returns the entire
// warning block text (with box-drawing characters stripped) for display in CI summaries.
func ExtractWarningBlocks(stdout string) []string {
	defer perf.Track(nil, "terraform.ExtractWarningBlocks")()

	var blocks []string
	lines := strings.Split(stdout, "\n")
	var current []string
	inWarningBlock := false

	for _, line := range lines {
		// Strip box-drawing prefix (│ or |) from the line for content checking.
		stripped := strings.TrimPrefix(line, "│ ")
		if stripped == line {
			stripped = strings.TrimPrefix(line, "| ")
		}

		// Detect warning start.
		if strings.HasPrefix(stripped, "Warning: ") && !inWarningBlock {
			inWarningBlock = true
			current = []string{stripped}
			continue
		}

		if inWarningBlock {
			// End of block: empty line without box prefix, or a new section marker.
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || trimmed == "╵" {
				if len(current) > 0 {
					blocks = append(blocks, strings.TrimRight(strings.Join(current, "\n"), "\n"))
				}
				current = nil
				inWarningBlock = false
				continue
			}
			// Detect start of a new block (error or another warning) without closing the previous one.
			if strings.HasPrefix(stripped, "Error: ") || (strings.HasPrefix(stripped, "Warning: ") && len(current) > 0) {
				blocks = append(blocks, strings.TrimRight(strings.Join(current, "\n"), "\n"))
				current = []string{stripped}
				continue
			}
			current = append(current, stripped)
		}
	}

	// Handle block that extends to end of output.
	if inWarningBlock && len(current) > 0 {
		blocks = append(blocks, strings.TrimRight(strings.Join(current, "\n"), "\n"))
	}

	return blocks
}

// ParsePlanOutput parses terraform plan stdout (fallback when JSON not available).
// Prefer ParsePlanJSON when a planfile is available.
func ParsePlanOutput(output string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParsePlanOutput")()

	result := &plugin.OutputResult{
		ExitCode:   0,
		HasChanges: false,
		HasErrors:  false,
		Errors:     nil,
		Data: &plugin.TerraformOutputData{
			ResourceCounts:    plugin.ResourceCounts{},
			CreatedResources:  []string{},
			UpdatedResources:  []string{},
			ReplacedResources: []string{},
			DeletedResources:  []string{},
			MovedResources:    []plugin.MovedResource{},
			ImportedResources: []string{},
			Outputs:           make(map[string]plugin.TerraformOutput),
		},
	}

	data := result.Data.(*plugin.TerraformOutputData)

	// Check for errors.
	if errors := ExtractErrors(output); len(errors) > 0 {
		result.HasErrors = true
		result.Errors = errors
	}

	// Check for no changes.
	if noChangesRe.MatchString(output) {
		result.HasChanges = false
		return result
	}

	// Try to parse plan summary (fallback regex parsing).
	if matches := planSummaryRe.FindStringSubmatch(output); len(matches) == 4 {
		data.ResourceCounts.Create = parseIntOrZero(matches[1])
		data.ResourceCounts.Change = parseIntOrZero(matches[2])
		data.ResourceCounts.Destroy = parseIntOrZero(matches[3])
		result.HasChanges = data.ResourceCounts.Create > 0 ||
			data.ResourceCounts.Change > 0 ||
			data.ResourceCounts.Destroy > 0

		if result.HasChanges {
			data.ChangedResult = buildChangeSummary(data.ResourceCounts)
		}
	}

	// Extract full warning blocks for CI summary display.
	data.Warnings = ExtractWarningBlocks(output)

	return result
}

// ParseApplyOutput parses terraform apply stdout.
func ParseApplyOutput(output string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParseApplyOutput")()

	result := &plugin.OutputResult{
		ExitCode:   0,
		HasChanges: false,
		HasErrors:  false,
		Errors:     nil,
		Data: &plugin.TerraformOutputData{
			ResourceCounts:    plugin.ResourceCounts{},
			CreatedResources:  []string{},
			UpdatedResources:  []string{},
			ReplacedResources: []string{},
			DeletedResources:  []string{},
			MovedResources:    []plugin.MovedResource{},
			ImportedResources: []string{},
			Outputs:           make(map[string]plugin.TerraformOutput),
		},
	}

	data := result.Data.(*plugin.TerraformOutputData)

	// Check for errors.
	if errors := ExtractErrors(output); len(errors) > 0 {
		result.HasErrors = true
		result.Errors = errors
	}

	// Parse apply summary.
	if matches := applySummaryRe.FindStringSubmatch(output); len(matches) == 4 {
		data.ResourceCounts.Create = parseIntOrZero(matches[1])
		data.ResourceCounts.Change = parseIntOrZero(matches[2])
		data.ResourceCounts.Destroy = parseIntOrZero(matches[3])
		result.HasChanges = data.ResourceCounts.Create > 0 ||
			data.ResourceCounts.Change > 0 ||
			data.ResourceCounts.Destroy > 0
	}

	return result
}

// ParseOutput parses terraform output for a given command (fallback when JSON not available).
// Prefer ParsePlanJSON + ParseOutputJSON for structured data.
func ParseOutput(output string, command string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParseOutput")()

	switch command {
	case "plan":
		return ParsePlanOutput(output)
	case "apply":
		return ParseApplyOutput(output)
	default:
		// For unknown commands, return minimal result.
		return &plugin.OutputResult{
			ExitCode:   0,
			HasChanges: false,
			HasErrors:  false,
			Errors:     nil,
			Data: &plugin.TerraformOutputData{
				ResourceCounts:    plugin.ResourceCounts{},
				CreatedResources:  []string{},
				UpdatedResources:  []string{},
				ReplacedResources: []string{},
				DeletedResources:  []string{},
				MovedResources:    []plugin.MovedResource{},
				ImportedResources: []string{},
				Outputs:           make(map[string]plugin.TerraformOutput),
			},
		}
	}
}

// buildChangeSummary builds a human-readable change summary string.
func buildChangeSummary(counts plugin.ResourceCounts) string {
	var parts []string
	if counts.Create > 0 {
		parts = append(parts, resourceCount(counts.Create)+" to add")
	}
	if counts.Change > 0 {
		parts = append(parts, resourceCount(counts.Change)+" to change")
	}
	if counts.Replace > 0 {
		parts = append(parts, resourceCount(counts.Replace)+" to replace")
	}
	if counts.Destroy > 0 {
		parts = append(parts, resourceCount(counts.Destroy)+" to destroy")
	}
	if len(parts) == 0 {
		return "No changes"
	}
	return strings.Join(parts, ", ")
}

// resourceCount formats a count of resources with proper pluralization.
func resourceCount(count int) string {
	if count == 1 {
		return "1 resource"
	}
	return strconv.Itoa(count) + " resources"
}

// parseIntOrZero parses a string to int, returning 0 on error.
func parseIntOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
