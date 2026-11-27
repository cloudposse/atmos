package configuration

import (
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// UnknownSectionsBehavior is the edition transition that turns unclaimed sections under
// `components.terraform` from a warning into a configuration error.
const UnknownSectionsBehavior = "components.terraform.unknown_sections"

// warnedSections remembers the sections already reported in this process. The configuration is
// loaded more than once per command, and one warning per section is enough.
var warnedSections sync.Map

// reportUnclaimedSections reports the `components.terraform` sections that are not an Atmos setting
// and that no built-in or enabled plugin provider claims. Editions before UnknownSectionsBehavior keep
// ignoring them and warn once per section; later and unpinned editions reject them.
func reportUnclaimedSections(pin string, sections []string) error {
	if len(sections) == 0 {
		return nil
	}
	strict, err := edition.BehaviorChanged(pin, UnknownSectionsBehavior)
	if err != nil {
		return err
	}
	if strict {
		return unclaimedSectionsError(sections)
	}
	for _, section := range sections {
		if _, warned := warnedSections.LoadOrStore(section, true); warned {
			continue
		}
		log.Warn("Ignoring an unknown section under components.terraform. Correct its name, enable the plugin package that provides it, or remove it; a later edition rejects it.",
			"section", "components.terraform."+section)
	}
	return nil
}

func unclaimedSectionsError(sections []string) error {
	names := make([]string, 0, len(sections))
	for _, section := range sections {
		names = append(names, "`components.terraform."+section+"`")
	}
	builder := errUtils.Build(errUtils.ErrUnknownTerraformSection).
		WithExplanationf("%s in atmos.yaml is not an Atmos setting, and no built-in or enabled plugin configuration provider uses it.", strings.Join(names, ", ")).
		WithHint("If it is a misspelled setting, correct its name.").
		WithHint("If it configures a plugin's configuration provider, enable the plugin package that provides it under `plugins.packages`.").
		WithHint("Otherwise remove it.").
		WithContext("sections", strings.Join(sections, ","))
	if date := transitionDate(); date != "" {
		builder = builder.WithHintf("To keep ignoring it while you migrate, pin `edition` to a date before %s.", date)
	}
	return builder.Err()
}

// transitionDate returns the date the edition transition takes effect, from the journal.
func transitionDate() string {
	for _, entry := range edition.Journal() {
		if entry.Kind == edition.KindBehavior && entry.Key == UnknownSectionsBehavior {
			return entry.Date
		}
	}
	return ""
}
