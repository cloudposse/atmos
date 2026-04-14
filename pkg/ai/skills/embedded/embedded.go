// Package embedded ships the built-in Atmos agent skills inside the Atmos binary.
//
// These skills are the canonical source at agent-skills/skills/ embedded at build
// time. Users get them immediately after installing Atmos — no marketplace
// install step is required — so `atmos ai ask --skill atmos-pro` (and any other
// built-in skill) works out of the box.
//
// Marketplace-installed skills (under ~/.atmos/skills/) override built-in skills
// of the same name. This lets advanced users ship updated versions without
// rebuilding Atmos.
package embedded

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	agentskills "github.com/cloudposse/atmos/agent-skills"
	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrNoClosingFrontmatter is returned when a SKILL.md opens with "---" but
// never closes the YAML frontmatter block.
var ErrNoClosingFrontmatter = errors.New("no closing frontmatter delimiter")

// skillsFS is the embedded skills filesystem supplied by the agent-skills
// package. Re-exposed here so other packages don't need to know the source.
var skillsFS = agentskills.SkillsFS

const (
	// Path within skillsFS that contains the per-skill directories.
	skillsRoot = "skills"

	// Manifest file every skill must provide.
	skillFileName = "SKILL.md"
)

// FS returns the embedded skills filesystem rooted at the per-skill directories.
// Callers can use fs.Sub to narrow to a specific skill's subtree.
func FS() fs.FS {
	sub, _ := fs.Sub(skillsFS, skillsRoot)
	return sub
}

// ListNames returns the names of every embedded skill in alphabetical order.
func ListNames() ([]string, error) {
	defer perf.Track(nil, "embedded.ListNames")()

	entries, err := fs.ReadDir(skillsFS, skillsRoot)
	if err != nil {
		return nil, fmt.Errorf("read embedded skills dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

// Load reads a single embedded skill by name and returns it ready for
// registration. The skill is marked IsBuiltIn: true so callers can
// distinguish it from marketplace-installed skills.
func Load(name string) (*skills.Skill, error) {
	defer perf.Track(nil, "embedded.Load")()

	skillDir := path.Join(skillsRoot, name)           //nolint:forbidigo // fs.FS requires forward slashes.
	skillMDPath := path.Join(skillDir, skillFileName) //nolint:forbidigo // fs.FS requires forward slashes.

	data, err := fs.ReadFile(skillsFS, skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", skillMDPath, err)
	}

	metadata, err := marketplace.ParseSkillMetadataBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s frontmatter: %w", skillMDPath, err)
	}

	body, err := readBody(data)
	if err != nil {
		return nil, fmt.Errorf("read %s body: %w", skillMDPath, err)
	}

	// Append referenced files if any (same semantics as marketplace loader).
	prompt := body
	for _, ref := range metadata.References {
		refPath := path.Join(skillDir, ref) //nolint:forbidigo // fs.FS requires forward slashes.
		refContent, err := fs.ReadFile(skillsFS, refPath)
		if err != nil {
			log.Warnf("embedded skill %q: reference %q not readable: %v", name, ref, err)
			continue
		}
		prompt += "\n\n---\n\n## Reference: " + path.Base(ref) + "\n\n" + strings.TrimSpace(string(refContent))
	}

	return &skills.Skill{
		Name:            metadata.Name,
		DisplayName:     metadata.GetDisplayName(),
		Description:     metadata.Description,
		SystemPrompt:    prompt,
		Category:        metadata.GetCategory(),
		IsBuiltIn:       true,
		AllowedTools:    metadata.AllowedTools,
		RestrictedTools: metadata.RestrictedTools,
	}, nil
}

// Loader implements skills.SkillLoader by registering every embedded skill.
// Marketplace-installed skills already present in the registry are preserved
// (first writer wins) so user overrides take precedence.
type Loader struct{}

// LoadInstalledSkills satisfies skills.SkillLoader. The name is misleading for
// the embedded case — these skills are compiled in, not installed — but keeping
// the interface shared lets LoadSkills stay polymorphic.
func (Loader) LoadInstalledSkills(registry *skills.Registry) error {
	return LoadAll(registry)
}

// LoadAll reads every embedded skill and registers it with the given registry.
// A skill that fails to load is logged and skipped — one broken skill should
// not break the rest. Skills already present in the registry (e.g., marketplace
// installs loaded earlier) are left alone so user overrides win.
func LoadAll(registry *skills.Registry) error {
	defer perf.Track(nil, "embedded.LoadAll")()

	names, err := ListNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		if registry.Has(name) {
			// Marketplace-installed skill already registered; user override wins.
			log.Debugf("embedded skill %q skipped — marketplace-installed version is present", name)
			continue
		}
		skill, err := Load(name)
		if err != nil {
			log.Warnf("embedded skill %q: %v", name, err)
			continue
		}
		if err := registry.Register(skill); err != nil {
			log.Warnf("embedded skill %q: register: %v", name, err)
			continue
		}
	}
	return nil
}

// readBody returns everything after the YAML frontmatter in a SKILL.md file.
// If there is no frontmatter, the entire content is treated as body.
func readBody(data []byte) (string, error) {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return s, nil
	}
	// Skip the opening ---.
	rest := strings.TrimPrefix(s, "---\n")
	rest = strings.TrimPrefix(rest, "---\r\n")
	// Find the closing ---.
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", ErrNoClosingFrontmatter
	}
	body := rest[idx+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	body = strings.TrimPrefix(body, "\r\n")
	return body, nil
}
