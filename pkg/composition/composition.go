// Package composition validates component membership in named compositions.
//
// A composition groups multiple services into one system. Components join a
// composition via their first-class `composition:` field. Membership is a closed
// contract (a component may only claim a service the composition declares) but
// fulfillment is open (a declared service with no component in a given stack is
// allowed).
package composition

import (
	"fmt"
	"slices"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValidateMembership checks that a component claiming membership in a composition
// is allowed: the composition must exist and must declare the component's name in
// its services list. Returns a hard error otherwise. An empty composition name is
// valid (no membership).
func ValidateMembership(componentName, compositionName string, compositions map[string]schema.Composition) error {
	defer perf.Track(nil, "composition.ValidateMembership")()

	if compositionName == "" {
		return nil
	}
	comp, ok := compositions[compositionName]
	if !ok {
		return fmt.Errorf("%w: component %q references composition %q, which is not declared under `compositions`",
			errUtils.ErrUnknownComposition, componentName, compositionName)
	}
	if !slices.Contains(comp.Services, componentName) {
		return fmt.Errorf("%w: component %q is not listed in `compositions.%s.services` %v",
			errUtils.ErrUnknownCompositionMembership, componentName, compositionName, comp.Services)
	}
	return nil
}

// Report describes which declared services of a composition are fulfilled by
// components in a stack and which are not provided there.
type Report struct {
	Composition string
	Description string
	Fulfilled   []string // declared services that a component provides in this stack
	NotProvided []string // declared services with no component in this stack
	Unknown     []string // members claimed by components but not declared (hard errors)
}

// Validate builds a soft Report for a composition given the set of component
// names that claim membership in it within a stack. Members not declared by the
// composition are surfaced in Unknown (these are hard errors elsewhere).
func Validate(compositionName string, members []string, compositions map[string]schema.Composition) (Report, error) {
	defer perf.Track(nil, "composition.Validate")()

	comp, ok := compositions[compositionName]
	if !ok {
		return Report{}, fmt.Errorf("%w: composition %q is not declared", errUtils.ErrUnknownComposition, compositionName)
	}

	memberSet := make(map[string]bool, len(members))
	for _, m := range members {
		memberSet[m] = true
	}

	report := Report{Composition: compositionName, Description: comp.Description}
	for _, service := range comp.Services {
		if memberSet[service] {
			report.Fulfilled = append(report.Fulfilled, service)
		} else {
			report.NotProvided = append(report.NotProvided, service)
		}
	}
	for _, member := range members {
		if !slices.Contains(comp.Services, member) {
			report.Unknown = append(report.Unknown, member)
		}
	}

	sort.Strings(report.Fulfilled)
	sort.Strings(report.NotProvided)
	sort.Strings(report.Unknown)
	return report, nil
}
