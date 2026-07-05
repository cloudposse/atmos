package composition

import (
	"context"
	"sort"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Seams for testability — overridden in tests.
var (
	initCliConfig  = cfg.InitCliConfig
	describeStacks = e.ExecuteDescribeStacks
)

// ExecuteValidate produces a soft report of which composition services are
// fulfilled and which are not provided in a stack, and surfaces any unknown
// members (which are hard errors enforced during component execution).
func ExecuteValidate(_ context.Context, info *schema.ConfigAndStacksInfo, name string) error {
	defer perf.Track(nil, "composition.ExecuteValidate")()

	atmosConfig, err := initCliConfig(*info, true)
	if err != nil {
		return err
	}

	stacksMap, err := describeStacks(
		&atmosConfig, info.Stack, nil, nil, nil,
		false, false, false, false, nil, nil,
	)
	if err != nil {
		return err
	}

	report, err := reportForStacks(stacksMap, info.Stack, name, atmosConfig.Compositions)
	if err != nil {
		return err
	}
	renderReport(&report)
	return nil
}

// reportForStacks collects the components that claim membership in the named
// composition across all stacks and builds the soft fulfillment Report. Split
// out from ExecuteValidate so the collect→validate core is unit-testable without
// config init or stack describe.
func reportForStacks(stacksMap map[string]any, stack string, name string, compositions map[string]schema.Composition) (Report, error) {
	return Validate(name, collectMembers(stacksMap, stack, name), compositions)
}

// collectMembers returns the sorted, de-duplicated set of component names across
// all stacks that declare membership in the named composition.
func collectMembers(stacksMap map[string]any, stack string, name string) []string {
	seen := map[string]bool{}
	if stack != "" {
		collectStackMembers(stacksMap[stack], name, seen)
		return sortedMembers(seen)
	}
	for _, stackData := range stacksMap {
		collectStackMembers(stackData, name, seen)
	}
	return sortedMembers(seen)
}

func sortedMembers(seen map[string]bool) []string {
	members := make([]string, 0, len(seen))
	for m := range seen {
		members = append(members, m)
	}
	sort.Strings(members)
	return members
}

// collectStackMembers records component names in one stack that claim membership
// in the named composition.
func collectStackMembers(stackData any, name string, seen map[string]bool) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return
	}
	componentsMap, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return
	}
	for _, typeSection := range componentsMap {
		typeMap, ok := typeSection.(map[string]any)
		if !ok {
			continue
		}
		for component, compData := range typeMap {
			if claimsComposition(compData, name) {
				seen[component] = true
			}
		}
	}
}

// claimsComposition reports whether a component section declares membership in
// the named composition.
func claimsComposition(compData any, name string) bool {
	compMap, ok := compData.(map[string]any)
	if !ok {
		return false
	}
	composition, _ := compMap["composition"].(string)
	return composition == name
}

// renderReport prints the composition report to the UI channel.
func renderReport(report *Report) {
	ui.Infof("Composition: %s", report.Composition)
	if report.Description != "" {
		ui.Writef("  %s\n", report.Description)
	}
	if len(report.Fulfilled) > 0 {
		ui.Successf("Fulfilled: %s", strings.Join(report.Fulfilled, ", "))
	}
	if len(report.NotProvided) > 0 {
		ui.Infof("Not provided here: %s", strings.Join(report.NotProvided, ", "))
	}
	if len(report.Unknown) > 0 {
		ui.Warningf("Unknown members (not declared in services): %s", strings.Join(report.Unknown, ", "))
	}
	if len(report.Fulfilled) == 0 && len(report.NotProvided) == 0 {
		ui.Infof("No services declared for %s", report.Composition)
	}
}
