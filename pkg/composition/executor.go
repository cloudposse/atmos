package composition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Seams for testability — overridden in tests.
var (
	initCliConfig        = cfg.InitCliConfig
	describeStacks       = e.ExecuteDescribeStacks
	getComponentProvider = component.GetProvider
	executeProvider      = func(provider component.ComponentProvider, execCtx *component.ExecutionContext) error {
		return provider.Execute(execCtx)
	}
)

const listSeparator = ", "

type member struct {
	Stack         string
	Composition   string
	ComponentType string
	Component     string
	ServiceIndex  int
}

type status struct {
	Name        string
	Description string
	Services    []string
	Members     []member
	Fulfilled   []string
	NotProvided []string
	Unknown     []member
}

type lifecycleRun struct {
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	verb        string
	flags       map[string]any
	targets     []member
}

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

	statuses, err := resolveStatuses(stacksMap, info.Stack, name, atmosConfig.Compositions)
	if err != nil {
		return err
	}
	for i := range statuses {
		renderStatus(&statuses[i])
	}
	return nil
}

// ListRows returns composition inventory rows. Without a stack, every declared
// composition includes the stacks that fulfill at least one of its services.
// With a stack, rows include fulfillment diagnostics for that stack.
func ListRows(_ context.Context, info *schema.ConfigAndStacksInfo) ([]map[string]any, error) {
	defer perf.Track(nil, "composition.ListRows")()

	atmosConfig, err := initCliConfig(*info, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := describeStacks(
		&atmosConfig, info.Stack, nil, nil, nil,
		false, false, false, false, nil, nil,
	)
	if err != nil {
		if info.Stack != "" || !noStacksError(err) {
			return nil, err
		}
		stacksMap = map[string]any{}
	}
	statuses, err := resolveStatuses(stacksMap, info.Stack, "", atmosConfig.Compositions)
	if err != nil {
		return nil, err
	}
	return listRows(statuses, info.Stack != ""), nil
}

func noStacksError(err error) bool {
	return errors.Is(err, errUtils.ErrFailedToFindImport) ||
		errors.Is(err, errUtils.ErrNoStackManifestsFound) ||
		errors.Is(err, errUtils.ErrNoStacksFound)
}

// ExecuteLifecycle runs a provider-backed lifecycle/read command against one
// composition or all compositions fulfilled in the selected stack.
func ExecuteLifecycle(ctx context.Context, info *schema.ConfigAndStacksInfo, verb, name string, flags map[string]any) error {
	defer perf.Track(nil, "composition.ExecuteLifecycle")()

	if strings.TrimSpace(info.Stack) == "" {
		return fmt.Errorf("%w: `atmos composition %s` requires --stack", errUtils.ErrComponentExecutionFailed, verb)
	}

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

	statuses, err := resolveStatuses(stacksMap, info.Stack, name, atmosConfig.Compositions)
	if err != nil {
		return err
	}
	targets, err := lifecycleTargets(statuses, name, verb)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		ui.Info("No fulfilled composition members found")
		return nil
	}

	return runLifecycleTargets(lifecycleRun{
		atmosConfig: &atmosConfig,
		info:        info,
		verb:        verb,
		flags:       flags,
		targets:     targets,
	})
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
		ui.Successf("Fulfilled: %s", strings.Join(report.Fulfilled, listSeparator))
	}
	if len(report.NotProvided) > 0 {
		ui.Infof("Not provided here: %s", strings.Join(report.NotProvided, listSeparator))
	}
	if len(report.Unknown) > 0 {
		ui.Warningf("Unknown members (not declared in services): %s", strings.Join(report.Unknown, listSeparator))
	}
	if len(report.Fulfilled) == 0 && len(report.NotProvided) == 0 {
		ui.Infof("No services declared for %s", report.Composition)
	}
}

func resolveStatuses(stacksMap map[string]any, stack, name string, compositions map[string]schema.Composition) ([]status, error) {
	names, err := selectedCompositionNames(compositions, name)
	if err != nil {
		return nil, err
	}

	statuses := make([]status, 0, len(names))
	for _, compName := range names {
		comp := compositions[compName]
		members := collectMembersDetailed(stacksMap, stack, compName)
		statuses = append(statuses, buildStatus(compName, comp, members))
	}
	return statuses, nil
}

func selectedCompositionNames(compositions map[string]schema.Composition, name string) ([]string, error) {
	if name != "" {
		if _, ok := compositions[name]; !ok {
			return nil, fmt.Errorf("%w: composition %q is not declared", errUtils.ErrUnknownComposition, name)
		}
		return []string{name}, nil
	}
	names := make([]string, 0, len(compositions))
	for compName := range compositions {
		names = append(names, compName)
	}
	sort.Strings(names)
	return names, nil
}

func collectMembersDetailed(stacksMap map[string]any, stack string, compositionName string) []member {
	var members []member
	if stack != "" {
		collectStackMembersDetailed(stacksMap[stack], stack, compositionName, &members)
		return orderMembers(members, false)
	}
	stackNames := make([]string, 0, len(stacksMap))
	for stackName := range stacksMap {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)
	for _, stackName := range stackNames {
		collectStackMembersDetailed(stacksMap[stackName], stackName, compositionName, &members)
	}
	return orderMembers(members, false)
}

func collectStackMembersDetailed(stackData any, stackName string, compositionName string, out *[]member) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return
	}
	componentsMap, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return
	}
	typeNames := make([]string, 0, len(componentsMap))
	for componentType := range componentsMap {
		typeNames = append(typeNames, componentType)
	}
	sort.Strings(typeNames)
	for _, componentType := range typeNames {
		typeMap, ok := componentsMap[componentType].(map[string]any)
		if !ok {
			continue
		}
		componentNames := make([]string, 0, len(typeMap))
		for componentName := range typeMap {
			componentNames = append(componentNames, componentName)
		}
		sort.Strings(componentNames)
		for _, componentName := range componentNames {
			if claimsComposition(typeMap[componentName], compositionName) {
				*out = append(*out, member{
					Stack:         stackName,
					Composition:   compositionName,
					ComponentType: componentType,
					Component:     componentName,
					ServiceIndex:  -1,
				})
			}
		}
	}
}

func buildStatus(name string, comp schema.Composition, members []member) status {
	serviceIndex := make(map[string]int, len(comp.Services))
	for i, service := range comp.Services {
		serviceIndex[service] = i
	}

	memberSet := make(map[string]bool, len(members))
	status := status{
		Name:        name,
		Description: comp.Description,
		Services:    append([]string(nil), comp.Services...),
	}
	for _, m := range members {
		if idx, ok := serviceIndex[m.Component]; ok {
			m.ServiceIndex = idx
			status.Members = append(status.Members, m)
			memberSet[m.Component] = true
			continue
		}
		status.Unknown = append(status.Unknown, m)
	}
	for _, service := range comp.Services {
		if memberSet[service] {
			status.Fulfilled = append(status.Fulfilled, service)
		} else {
			status.NotProvided = append(status.NotProvided, service)
		}
	}
	status.Members = orderMembers(status.Members, false)
	status.Unknown = orderMembers(status.Unknown, false)
	return status
}

func orderMembers(members []member, reverse bool) []member {
	ordered := append([]member(nil), members...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Stack != ordered[j].Stack {
			return ordered[i].Stack < ordered[j].Stack
		}
		if ordered[i].ServiceIndex != ordered[j].ServiceIndex {
			return ordered[i].ServiceIndex < ordered[j].ServiceIndex
		}
		if ordered[i].Component != ordered[j].Component {
			return ordered[i].Component < ordered[j].Component
		}
		return ordered[i].ComponentType < ordered[j].ComponentType
	})
	if reverse {
		slices.Reverse(ordered)
	}
	return ordered
}

func lifecycleTargets(statuses []status, explicitName, verb string) ([]member, error) {
	reverse := isReverseVerb(verb)
	orderedStatuses := append([]status(nil), statuses...)
	sort.SliceStable(orderedStatuses, func(i, j int) bool {
		if reverse {
			return orderedStatuses[i].Name > orderedStatuses[j].Name
		}
		return orderedStatuses[i].Name < orderedStatuses[j].Name
	})

	var targets []member
	for i := range orderedStatuses {
		s := &orderedStatuses[i]
		if len(s.Unknown) > 0 {
			return nil, unknownMembersError(s)
		}
		if explicitName == "" && len(s.Members) == 0 {
			continue
		}
		targets = append(targets, orderMembers(s.Members, reverse)...)
	}
	return targets, nil
}

func unknownMembersError(s *status) error {
	parts := make([]string, len(s.Unknown))
	for i, m := range s.Unknown {
		parts[i] = fmt.Sprintf("%s/%s.%s", m.Stack, m.ComponentType, m.Component)
	}
	return fmt.Errorf("%w: composition %q has members not declared in services: %s",
		errUtils.ErrUnknownCompositionMembership, s.Name, strings.Join(parts, listSeparator))
}

func isReverseVerb(verb string) bool {
	return verb == "down" || verb == "stop" || verb == "rm"
}

func runLifecycleTargets(run lifecycleRun) error {
	var errs []error
	for _, target := range run.targets {
		provider, ok := getComponentProvider(target.ComponentType)
		if !ok {
			errs = append(errs, fmt.Errorf("%w: %s", errUtils.ErrComponentProviderNotFound, target.ComponentType))
			continue
		}
		if !providerSupports(provider, run.verb) {
			errs = append(errs, fmt.Errorf("%w: %s component %q in composition %q does not support %q",
				errUtils.ErrUnsupportedComponentType, target.ComponentType, target.Component, target.Composition, run.verb))
			continue
		}

		itemInfo := *run.info
		itemInfo.Stack = target.Stack
		itemInfo.ComponentType = target.ComponentType
		itemInfo.ComponentFromArg = target.Component
		itemInfo.Component = target.Component
		itemInfo.SubCommand = run.verb
		itemInfo.Command = target.ComponentType
		itemInfo.All = false

		err := executeProvider(provider, &component.ExecutionContext{
			AtmosConfig:         run.atmosConfig,
			ComponentType:       target.ComponentType,
			Component:           target.Component,
			Stack:               target.Stack,
			Command:             target.ComponentType,
			SubCommand:          run.verb,
			ConfigAndStacksInfo: itemInfo,
			Flags:               run.flags,
		})
		if err != nil {
			ui.Errorf("%s/%s.%s: %s failed: %v", target.Stack, target.ComponentType, target.Component, run.verb, err)
			errs = append(errs, fmt.Errorf("%s/%s.%s: %w", target.Stack, target.ComponentType, target.Component, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	ui.Successf("%s: %d composition member(s) succeeded", run.verb, len(run.targets))
	return nil
}

func providerSupports(provider component.ComponentProvider, verb string) bool {
	return slices.Contains(provider.GetAvailableCommands(), verb)
}

func listRows(statuses []status, stackScoped bool) []map[string]any {
	rows := make([]map[string]any, 0, len(statuses))
	for i := range statuses {
		s := &statuses[i]
		row := map[string]any{
			"composition": s.Name,
			"services":    strings.Join(s.Services, listSeparator),
			"description": s.Description,
		}
		if stackScoped {
			row["fulfilled"] = strings.Join(s.Fulfilled, listSeparator)
			row["not_provided"] = strings.Join(s.NotProvided, listSeparator)
			row["unknown"] = memberNames(s.Unknown)
		} else {
			row["stacks"] = strings.Join(memberStacks(s.Members), listSeparator)
		}
		rows = append(rows, row)
	}
	return rows
}

func memberStacks(members []member) []string {
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		seen[member.Stack] = struct{}{}
	}
	stacks := make([]string, 0, len(seen))
	for stack := range seen {
		stacks = append(stacks, stack)
	}
	sort.Strings(stacks)
	return stacks
}

func renderStatus(s *status) {
	renderReport(&Report{
		Composition: s.Name,
		Description: s.Description,
		Fulfilled:   append([]string(nil), s.Fulfilled...),
		NotProvided: append([]string(nil), s.NotProvided...),
		Unknown:     memberComponents(s.Unknown),
	})
}

func memberComponents(members []member) []string {
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = m.Component
	}
	return names
}

func memberNames(members []member) string {
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = fmt.Sprintf("%s.%s", m.ComponentType, m.Component)
	}
	return strings.Join(names, listSeparator)
}
