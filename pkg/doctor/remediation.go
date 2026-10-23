package doctor

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/doctor/monorepo"
)

// message names the declaration a finding is about, including the value that makes it legacy.
func (e *edit) message() string {
	switch e.kind {
	case ruleComponentDependencies:
		return "Legacy component dependency declaration"
	case ruleHookEvent:
		return fmt.Sprintf("Legacy hook-event declaration: %q is now %q", e.from, e.value)
	case ruleCommandProvides:
		return fmt.Sprintf("Legacy command-provides declaration (%s: %s)", e.old, e.from)
	default:
		return "Legacy " + e.kind + " declaration"
	}
}

// manualRemediation tells the author how to make the change by hand, for findings that --fix
// does not apply. It never suggests --fix, because that would not change the outcome.
func (e *edit) manualRemediation() string {
	switch e.kind {
	case ruleLegacyNaming:
		return fmt.Sprintf("Replace %s with %s: %q", e.old, e.target, e.value)
	case ruleHookEvent:
		return fmt.Sprintf("Change the event to %q by hand", e.value)
	case ruleCommandProvides:
		return fmt.Sprintf("Rename %s to provides by hand", e.old)
	case ruleComponentDependencies:
		return "Move settings.depends_on to dependencies.components and resolve any required-dependency errors"
	case ruleTerraformReconfigure:
		return fmt.Sprintf("Replace init_run_reconfigure with init.reconfigure: %s under components.terraform by hand", e.value)
	case ruleComponentMonorepo:
		return "Change the source to " + e.value + " by hand, or follow " + monorepo.MigrationGuideURL
	}
	return "Review the declaration and the rule documentation"
}

// conflictRemediation tells the author what to do when both the legacy key and its replacement exist.
func (e *edit) conflictRemediation() string {
	return fmt.Sprintf("Delete %s by hand after deciding which value to keep", e.old)
}

// fixedReason explains, for a repair that was written, what was verified about it.
func (e *edit) fixedReason() string {
	if e.kind == ruleComponentDependencies {
		return "Migrated to modern required dependencies; comments and unrelated YAML values preserved"
	}
	if e.kind == ruleComponentMonorepo {
		return "Moved to the successor repository at the same version; the files are identical and only the source changed"
	}
	return "Runtime field behavior and unrelated YAML values verified"
}
