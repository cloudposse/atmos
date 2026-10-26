package monorepo

import (
	_ "embed"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

//go:embed components.yaml
var componentsYAML []byte

// componentMap is the decoded form of components.yaml.
type componentMap struct {
	RepoSettings struct {
		Prefix string `yaml:"prefix"`
	} `yaml:"repo_settings"`
	// Components maps a monorepo module path to the suffix of its successor repository.
	Components map[string]string `yaml:"component_map"`
}

var (
	loadOnce sync.Once
	loaded   componentMap
)

// components returns the embedded map, decoded once. The file ships with the binary, so a
// decoding failure is a build defect that the package tests catch.
func components() *componentMap {
	loadOnce.Do(func() {
		_ = yaml.Unmarshal(componentsYAML, &loaded)
	})
	return &loaded
}

// Repository returns the name of the repository that replaced the monorepo module, for example
// "aws-identity-center" for "aws-sso". It reports false for a module without a successor.
func Repository(module string) (string, bool) {
	defer perf.Track(nil, "monorepo.Repository")()

	m := components()
	suffix, ok := m.Components[module]
	if !ok {
		return "", false
	}
	return m.RepoSettings.Prefix + "-" + strings.ReplaceAll(suffix, "/", "-"), true
}

// Successors lists the repositories that replaced the subcomponents of a module that was split,
// for example the repositories of "tgw/hub", "tgw/spoke", and "tgw/cross-region-hub-connector"
// for "tgw". It returns nil when no mapped module lives below the given path.
func Successors(parent string) []string {
	defer perf.Track(nil, "monorepo.Successors")()

	prefix := parent + "/"
	var repositories []string
	for module := range components().Components {
		if !strings.HasPrefix(module, prefix) {
			continue
		}
		if name, ok := Repository(module); ok {
			repositories = append(repositories, name)
		}
	}
	sort.Strings(repositories)
	return repositories
}

// ParentModule finds the longest mapped module that contains a deeper path, such as "eks/cluster"
// for "eks/cluster/modules/foo", and returns it with the remaining path. It compares whole path
// segments, so "eks/karpenter" never matches "eks/karpenter-node-pool/x". It reports false when no
// proper parent is mapped or the path has empty, "." or ".." segments.
func ParentModule(module string) (parent, subpath string, ok bool) {
	defer perf.Track(nil, "monorepo.ParentModule")()

	segments := strings.Split(module, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", "", false
		}
	}
	for n := len(segments) - 1; n > 0; n-- {
		candidate := strings.Join(segments[:n], "/")
		if _, mapped := Repository(candidate); mapped {
			return candidate, strings.Join(segments[n:], "/"), true
		}
	}
	return "", "", false
}
