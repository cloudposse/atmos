package marketplace

import (
	"fmt"
	"io/fs"
	"sort"

	log "github.com/charmbracelet/log"

	agentskills "github.com/cloudposse/atmos/agent-skills"
	"github.com/cloudposse/atmos/pkg/perf"
)

// bundledSkillsRoot is the directory within the embedded FS that holds the
// official skills (skills/<name>/SKILL.md).
const bundledSkillsRoot = "skills"

// bundledSourceFmt is the canonical install source recorded for a bundled skill,
// pointing at the skill's path in the cloudposse/atmos repository so an installed
// bundled skill is traceable to its upstream source.
const bundledSourceFmt = "github.com/cloudposse/atmos//agent-skills/skills/%s"

// AvailableSkill describes a skill in the bundled catalog (whether or not it is
// installed). It is the "available" half of the available-vs-installed view in
// `atmos ai skill list`.
type AvailableSkill struct {
	Name        string
	DisplayName string
	Description string
	Version     string
	Source      string
}

// Catalog returns the official skills bundled into the binary, sorted by name.
// It works fully offline — no network or Git access required.
func Catalog() ([]AvailableSkill, error) {
	defer perf.Track(nil, "marketplace.Catalog")()

	entries, err := fs.ReadDir(agentskills.Skills, bundledSkillsRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundled skills: %w", err)
	}

	catalog := make([]AvailableSkill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		available, ok := LookupBundledSkill(entry.Name())
		if !ok {
			// A directory without a valid SKILL.md is not a usable skill; skip it
			// rather than failing the whole catalog.
			continue
		}

		catalog = append(catalog, available)
	}

	sort.Slice(catalog, func(i, j int) bool {
		return catalog[i].Name < catalog[j].Name
	})

	return catalog, nil
}

// LookupBundledSkill returns the catalog entry for a bundled skill by name and
// whether it exists. The name maps directly to a directory in the embedded FS,
// which is how `install <name>` resolves to the offline source.
//
// The second return value is false only when the skill directory is absent.
// A parse failure returns (zero, false) and also logs an error so that a
// broken bundled SKILL.md is surfaced loudly rather than silently dropped.
func LookupBundledSkill(name string) (AvailableSkill, bool) {
	defer perf.Track(nil, "marketplace.LookupBundledSkill")()

	// Probe the directory first so we can distinguish "not present" from
	// "present but invalid".
	skillMDPath := fmt.Sprintf("%s/%s/%s", bundledSkillsRoot, name, skillFileName)
	_, statErr := fs.Stat(agentskills.Skills, skillMDPath)
	if statErr != nil {
		// Skill directory / SKILL.md does not exist — genuinely absent.
		return AvailableSkill{}, false
	}

	metadata, err := readBundledMetadata(name)
	if err != nil {
		// SKILL.md exists but is malformed — surface the error loudly so a
		// broken release artifact is not silently treated as "not found".
		log.Error("bundled skill has an invalid SKILL.md and will be unavailable", "skill", name, "error", err)
		return AvailableSkill{}, false
	}

	// Use the embedded directory name (not frontmatter metadata.Name) as the
	// stable skill ID so that list/install/Source are always consistent.
	return AvailableSkill{
		Name:        name,
		DisplayName: metadata.GetDisplayName(),
		Description: metadata.Description,
		Version:     metadata.GetVersion(),
		Source:      fmt.Sprintf(bundledSourceFmt, name),
	}, true
}

// readBundledMetadata parses the SKILL.md frontmatter for a bundled skill from
// the embedded FS. Embedded (fs.FS) paths always use forward slashes, so they
// are built with explicit "/" rather than filepath.Join.
func readBundledMetadata(name string) (*SkillMetadata, error) {
	skillMDPath := fmt.Sprintf("%s/%s/%s", bundledSkillsRoot, name, skillFileName)
	data, err := fs.ReadFile(agentskills.Skills, skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", skillFileName, err)
	}

	return ParseSkillMetadataBytes(data)
}

// bundledSkillFS returns an fs.FS rooted at the bundled skill's directory, used
// to copy the complete skill (SKILL.md plus references) to the install path.
func bundledSkillFS(name string) (fs.FS, error) {
	root := fmt.Sprintf("%s/%s", bundledSkillsRoot, name)
	sub, err := fs.Sub(agentskills.Skills, root)
	if err != nil {
		return nil, fmt.Errorf("failed to open bundled skill %q: %w", name, err)
	}

	return sub, nil
}
