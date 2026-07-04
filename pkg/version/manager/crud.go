package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

var (
	// ErrEntryExists is returned when adding an entry that is already configured.
	ErrEntryExists = errors.New("version entry already exists")
	// ErrEntryNotFound is returned when editing an entry that is not configured.
	ErrEntryNotFound = errors.New("version entry not found")
)

// Permission for a configuration file written by CRUD edits when its previous
// mode cannot be determined.
const configFilePerm os.FileMode = 0o644

// entryPath returns the dot path to a version entry inside atmos.yaml.
func entryPath(track, name string) string {
	return fmt.Sprintf("version.tracks.%s.versions.%s", track, name)
}

// AddEntry writes a new version entry into the editable atmos.yaml,
// preserving comments and formatting, and returns the file modified. It fails
// with ErrEntryExists when the entry is already configured.
func AddEntry(atmosConfig *schema.AtmosConfiguration, track, name string, entry *schema.VersionEntry) (string, error) {
	defer perf.Track(atmosConfig, "manager.AddEntry")()

	track = EffectiveTrack(atmosConfig, track)
	file, content, err := readEditableConfig(atmosConfig)
	if err != nil {
		return "", err
	}
	path := entryPath(track, name)
	if _, err := atmosyaml.Get(content, path); err == nil {
		return "", fmt.Errorf("%w: %s in track %s (%s)", ErrEntryExists, name, track, file)
	}
	document, err := json.Marshal(entryDocument(entry))
	if err != nil {
		return "", err
	}
	updated, err := atmosyaml.SetRaw(content, path, string(document))
	if err != nil {
		return "", err
	}
	return file, writeConfig(file, updated)
}

// SetEntryFields updates fields of an existing version entry (dot-relative
// paths such as "desired" or "update.pin") and returns the file modified.
func SetEntryFields(atmosConfig *schema.AtmosConfiguration, track, name string, fields map[string]string) (string, error) {
	defer perf.Track(atmosConfig, "manager.SetEntryFields")()

	track = EffectiveTrack(atmosConfig, track)
	file, content, err := readEditableConfig(atmosConfig)
	if err != nil {
		return "", err
	}
	path := entryPath(track, name)
	if _, err := atmosyaml.Get(content, path); err != nil {
		return "", fmt.Errorf("%w: %s in track %s (%s)", ErrEntryNotFound, name, track, file)
	}
	for field, value := range fields {
		content, err = atmosyaml.Set(content, path+"."+field, value)
		if err != nil {
			return "", err
		}
	}
	return file, writeConfig(file, content)
}

// RemoveEntry deletes a version entry and returns the file modified.
func RemoveEntry(atmosConfig *schema.AtmosConfiguration, track, name string) (string, error) {
	defer perf.Track(atmosConfig, "manager.RemoveEntry")()

	track = EffectiveTrack(atmosConfig, track)
	file, content, err := readEditableConfig(atmosConfig)
	if err != nil {
		return "", err
	}
	path := entryPath(track, name)
	if _, err := atmosyaml.Get(content, path); err != nil {
		return "", fmt.Errorf("%w: %s in track %s (%s)", ErrEntryNotFound, name, track, file)
	}
	updated, err := atmosyaml.Delete(content, path)
	if err != nil {
		return "", err
	}
	return file, writeConfig(file, updated)
}

// InferEcosystem guesses the ecosystem for a package coordinate when it is
// not set explicitly: GitHub Actions for actions/*, OCI for registry-hosted
// images (first segment carries a dot), toolchain for bare tool names, and
// github for everything else in owner/repo form.
func InferEcosystem(pkg string) string {
	defer perf.Track(nil, "manager.InferEcosystem")()

	switch {
	case strings.HasPrefix(pkg, "actions/"):
		return "github-actions"
	case strings.Contains(strings.SplitN(pkg, "/", 2)[0], "."): //nolint:mnd // Split into host and remainder.
		return "oci"
	case strings.Contains(pkg, "/"):
		return "github"
	default:
		return "toolchain"
	}
}

// entryDocument builds the minimal YAML document for a new entry, leaving out
// empty fields so the configuration stays clean.
func entryDocument(entry *schema.VersionEntry) map[string]any {
	document := map[string]any{}
	setIfPresent := func(key, value string) {
		if value != "" {
			document[key] = value
		}
	}
	setIfPresent("ecosystem", entry.Ecosystem)
	setIfPresent("datasource", entry.Datasource)
	setIfPresent("provider", entry.Provider)
	setIfPresent("package", entry.Package)
	setIfPresent("desired", entry.Desired)
	setIfPresent("group", entry.Group)
	if entry.Update.Pin != "" {
		document["update"] = map[string]any{"pin": entry.Update.Pin}
	}
	return document
}

// readEditableConfig resolves and reads the atmos.yaml file CRUD edits target.
func readEditableConfig(atmosConfig *schema.AtmosConfiguration) (string, []byte, error) {
	file, err := cfg.ResolveEditableConfigFile(atmosConfig, "")
	if err != nil {
		return "", nil, err
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return "", nil, err
	}
	return file, content, nil
}

// writeConfig writes the updated configuration preserving the file's mode.
func writeConfig(file string, content []byte) error {
	perm := configFilePerm
	if info, err := os.Stat(file); err == nil {
		perm = info.Mode().Perm()
	}
	return os.WriteFile(file, content, perm) // #nosec G306 -- atmos.yaml is a non-sensitive project file.
}
