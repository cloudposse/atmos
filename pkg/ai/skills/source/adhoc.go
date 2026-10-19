package source

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AdHocConfig restores locally recorded declarations for installed ad-hoc skills.
func (e *Engine) AdHocConfig(label string) (*schema.AtmosConfiguration, error) {
	config := *e.Config
	config.AI.Skills = map[string]*schema.AISkillConfig{}
	lock := Lock{}
	previous := e.AdHoc
	e.AdHoc = true
	err := readYAML(e.lockPath(), &lock)
	e.AdHoc = previous
	if err != nil {
		return nil, err
	}
	states, err := e.loadStates()
	if err != nil {
		return nil, err
	}
	for _, state := range states {
		for _, r := range state.Records {
			if r.Project != e.Project || !strings.HasPrefix(r.Source, "ad-hoc:") || (label != "" && label != r.Source) {
				continue
			}
			resolution, ok := lock.Tracks[r.Track][r.Source]
			if !ok {
				return nil, fmt.Errorf("%w: missing local resolution for %s", ErrDrift, r.Source)
			}
			d := resolution.Declaration
			config.AI.Skills[r.Source] = &d
		}
	}
	return &config, nil
}

// ParseAdHoc preserves shorthand and local sources while accepting generic Git URLs.
func ParseAdHoc(raw string) (*schema.AISkillConfig, error) {
	if _, ok := marketplace.LookupBundledSkill(raw); ok {
		return &schema.AISkillConfig{Source: "bundled:" + raw}, nil
	}
	remoteURL := (strings.Contains(raw, "://") && !strings.HasPrefix(raw, "file://")) || strings.HasPrefix(raw, "git@")
	if remoteURL {
		if _, err := gitSourceURL(Repository{Source: raw}); err != nil {
			return nil, err
		}
	}
	parsed, err := marketplace.ParseSource(raw)
	if err == nil {
		return &schema.AISkillConfig{Source: parsed.URL, Ref: schema.SkillRef{Literal: parsed.Ref}}, nil
	}
	if remoteURL {
		return &schema.AISkillConfig{Source: raw}, nil
	}
	return nil, err
}
