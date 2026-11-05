// Package inventory defines the committed package inventory shared by release planning and catalog reconciliation.
package inventory

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// File declares package locations and optional CI runner assignments.
type File struct {
	Packages map[string]Package `yaml:"packages"`
	Runners  map[string]string  `yaml:"runners"`
}

// Package identifies a package and its additional repository-relative build inputs.
type Package struct {
	Path       string   `yaml:"path"`
	Repository string   `yaml:"repository"`
	Inputs     []string `yaml:"inputs"`
}

// Parse decodes the shared inventory schema without requiring build-specific metadata.
func Parse(raw []byte) (*File, error) {
	defer perf.Track(nil, "inventory.Parse")()

	var file File
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("%w: inventory: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	if len(file.Packages) == 0 {
		return nil, fmt.Errorf("%w: inventory has no packages", errUtils.ErrPluginInvalidConfig)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: inventory must contain one YAML document", errUtils.ErrPluginInvalidConfig)
	}
	return &file, nil
}

// SafePath reports whether a path is canonical and relative to the repository root.
//
//nolint:lintroller // Pure path validation.
func SafePath(value string) bool {
	if value == "" || path.IsAbs(value) || path.Clean(value) != value || strings.ContainsAny(value, "\\:\x00\n\r") {
		return false
	}
	return value != "." && value != ".." && !strings.HasPrefix(value, "../")
}
