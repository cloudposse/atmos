// Package sbom builds provenance and software-bill-of-material graphs from
// Atmos lock files and supported dependency adapters.
package sbom

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	toolchainlock "github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	vendorlock "github.com/cloudposse/atmos/pkg/vendoring/lockfile"
	versionmanager "github.com/cloudposse/atmos/pkg/version/manager"
)

const (
	FormatCycloneDXJSON = "cyclonedx-json"
	FormatSPDXJSON      = "spdx-json"
	ModeProvenance      = "provenance"
	ModeNTIA            = "ntia"
	ScopeTerraform      = "terraform"
)

// Subject identifies the software or infrastructure configuration described by
// a graph. NTIA output requires every field; provenance output may omit it.
type Subject struct {
	Name     string
	Version  string
	Supplier string
}

// Options controls graph construction and compliance validation.
type Options struct {
	IncludeFiles bool
	Scope        string
	Mode         string
	Subject      Subject
}

// Component is a normalized node shared by all adapters and renderers.
type Component struct {
	ID         string
	Name       string
	Version    string
	Type       string
	PURL       string
	Source     string
	SHA256     string
	Supplier   string
	Properties map[string]string
}

type Relationship struct {
	From string
	To   string
	Type string
}

// Coverage records whether an adapter had sufficient stable evidence for the
// selected scope. An unavailable adapter is visible in provenance output and
// prevents NTIA-mode output.
type Coverage struct {
	Adapter string
	Status  string // complete, incomplete, unavailable
	Detail  string
}

type Graph struct {
	Subject       Subject
	Components    []Component
	Relationships []Relationship
	Coverage      []Coverage
}

// Build preserves the original package API for callers that want a
// lock-derived provenance graph. The CLI uses BuildWithOptions for scope and
// mode validation.
func Build(config *schema.AtmosConfiguration, includeFiles bool) (*Graph, error) {
	return BuildWithOptions(config, Options{IncludeFiles: includeFiles, Mode: ModeProvenance})
}

func BuildWithOptions(config *schema.AtmosConfiguration, options Options) (*Graph, error) {
	if options.Mode == "" {
		options.Mode = ModeProvenance
	}
	if options.Mode != ModeProvenance && options.Mode != ModeNTIA {
		return nil, fmt.Errorf("unsupported SBOM mode %q", options.Mode)
	}
	if options.Scope != "" && options.Scope != ScopeTerraform {
		return nil, fmt.Errorf("unsupported SBOM scope %q", options.Scope)
	}
	graph := &Graph{Subject: options.Subject}
	if options.Subject.Name != "" {
		graph.Components = append(graph.Components, subjectComponent(options.Subject))
	}
	if err := appendVendor(graph, config, options.IncludeFiles); err != nil {
		return nil, err
	}
	if options.Scope == ScopeTerraform {
		if err := appendTerraform(graph, config); err != nil {
			return nil, err
		}
	} else {
		if err := appendToolchain(graph, config); err != nil {
			return nil, err
		}
		if err := appendVersions(graph, config); err != nil {
			return nil, err
		}
	}
	graph.Components = dedupe(graph.Components)
	sort.Slice(graph.Coverage, func(i, j int) bool { return graph.Coverage[i].Adapter < graph.Coverage[j].Adapter })
	if options.Mode == ModeNTIA {
		if err := validateNTIA(graph, options.Scope); err != nil {
			return nil, err
		}
	}
	return graph, nil
}

func subjectComponent(subject Subject) Component {
	return Component{ID: "atmos:subject", Name: subject.Name, Version: subject.Version, Type: "application", Supplier: subject.Supplier, Properties: map[string]string{"atmos:domain": "subject"}}
}

func appendVendor(graph *Graph, config *schema.AtmosConfiguration, includeFiles bool) error {
	path := vendorlock.Path(config)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// A project may rely solely on JIT sources, whose local receipts are
		// intentionally not represented in the committed vendor lock.
	} else if err != nil {
		return fmt.Errorf("stat vendor lock: %w", err)
	} else {
		lock, err := vendorlock.Load(config)
		if err != nil {
			return err
		}
		for id, artifact := range lock.Artifacts {
			name := artifact.Component
			if name == "" {
				name = id
			}
			source := artifact.Source.Resolved
			if source == "" {
				source = artifact.Source.Declared
			}
			component := Component{ID: "vendor:" + id, Name: name, Version: artifact.Source.Digest, Type: "library", Source: source, SHA256: trimSHA256(artifact.Source.Digest), Properties: map[string]string{"atmos:domain": "vendor", "atmos:lock-file": repositoryPath(config, path), "atmos:kind": artifact.Kind, "atmos:target": repositoryPath(config, artifact.Target)}}
			graph.Components = append(graph.Components, component)
			if !includeFiles {
				continue
			}
			for _, file := range artifact.Files {
				fileID := component.ID + ":file:" + file.Path
				graph.Components = append(graph.Components, Component{ID: fileID, Name: repositoryPath(config, filepath.Join(artifact.Target, file.Path)), Version: file.SHA256, Type: "file", SHA256: file.SHA256, Properties: map[string]string{"atmos:domain": "vendor-file", "atmos:artifact": component.ID}})
				graph.Relationships = append(graph.Relationships, Relationship{From: component.ID, To: fileID, Type: "contains"})
			}
		}
	}
	complete, detail, err := appendJITSources(graph, config)
	if err != nil {
		return err
	}
	graph.Coverage = append(graph.Coverage, Coverage{Adapter: "atmos-sources", Status: coverageStatus(complete), Detail: detail})
	return nil
}

func appendJITSources(graph *Graph, config *schema.AtmosConfiguration) (bool, string, error) {
	if config == nil || config.BasePath == "" {
		return true, "vendor lock parsed; no JIT workdir search path", nil
	}
	complete, receipts := true, 0
	err := filepath.WalkDir(config.BasePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == ".terraform") {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != workdir.MetadataFile || filepath.Base(filepath.Dir(path)) != workdir.AtmosDir {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var receipt workdir.WorkdirMetadata
		if err := json.Unmarshal(data, &receipt); err != nil {
			return fmt.Errorf("parse JIT workdir receipt %q: %w", path, err)
		}
		receipts++
		source := receipt.SourceResolved
		if source == "" {
			source = receipt.SourceURI
		}
		identity := receipt.SourceIdentity
		if identity == "" {
			complete = false
			identity = "NOASSERTION"
		}
		target := filepath.Dir(filepath.Dir(path))
		id := "jit:" + filepath.ToSlash(relativeToBase(config.BasePath, target))
		graph.Components = append(graph.Components, Component{ID: id, Name: receipt.Component, Version: identity, Type: "library", Source: source, SHA256: trimSHA256(identity), Properties: map[string]string{"atmos:domain": "jit-source", "atmos:receipt": repositoryPath(config, path), "atmos:target": repositoryPath(config, target), "atmos:stack": receipt.Stack}})
		return nil
	})
	if err != nil {
		return false, "failed to inspect JIT receipts", err
	}
	if receipts == 0 {
		return true, "vendor lock parsed; no JIT receipts found", nil
	}
	if !complete {
		return false, "one or more JIT receipts lack immutable source identity", nil
	}
	return true, "vendor lock and JIT receipts parsed", nil
}

func relativeToBase(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

func repositoryPath(config *schema.AtmosConfiguration, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(filepath.Clean(path))
	}
	if config == nil || config.BasePath == "" {
		return filepath.ToSlash(filepath.Base(path))
	}
	base := config.BasePath
	if !filepath.IsAbs(base) {
		if workingDirectory, err := os.Getwd(); err == nil {
			base = filepath.Join(workingDirectory, base)
		}
	}
	return filepath.ToSlash(relativeToBase(base, path))
}

func appendToolchain(graph *Graph, config *schema.AtmosConfiguration) error {
	if config == nil {
		return nil
	}
	path := config.Toolchain.LockFile
	if path == "" {
		installPath := config.Toolchain.InstallPath
		if installPath == "" {
			installPath = ".tools"
		}
		path = filepath.Join(installPath, "toolchain.lock.yaml")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(config.BasePath, path)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat toolchain lock: %w", err)
	}
	lock, err := toolchainlock.Load(path)
	if err != nil {
		return err
	}
	for name, tool := range lock.Tools {
		for platform, entry := range tool.Platforms {
			graph.Components = append(graph.Components, Component{ID: "toolchain:" + name + ":" + platform, Name: name, Version: tool.Version, Type: "application", Source: entry.URL, SHA256: trimSHA256(entry.Checksum), Properties: map[string]string{"atmos:domain": "toolchain", "atmos:lock-file": repositoryPath(config, path), "atmos:platform": platform}})
		}
	}
	return nil
}

func appendVersions(graph *Graph, config *schema.AtmosConfiguration) error {
	if config == nil {
		return nil
	}
	path := versionmanager.LockFilePath(config)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat versions lock: %w", err)
	}
	lock, err := versionmanager.LoadLock(config)
	if err != nil {
		return err
	}
	for track, entries := range lock.Tracks {
		for name, entry := range entries {
			graph.Components = append(graph.Components, Component{ID: "version:" + track + ":" + name, Name: name, Version: entry.Version, Type: "library", Source: entry.Provider, SHA256: trimSHA256(entry.Digest), Properties: map[string]string{"atmos:domain": "version-track", "atmos:lock-file": repositoryPath(config, path), "atmos:track": track, "atmos:ecosystem": entry.Ecosystem, "atmos:datasource": entry.Datasource}})
		}
	}
	return nil
}

func validateNTIA(graph *Graph, scope string) error {
	if graph.Subject.Name == "" || graph.Subject.Version == "" || graph.Subject.Supplier == "" {
		return fmt.Errorf("NTIA SBOM requires subject name, version, and supplier")
	}
	if scope != ScopeTerraform {
		return fmt.Errorf("NTIA SBOM requires an explicit supported scope")
	}
	for _, coverage := range graph.Coverage {
		if coverage.Status != "complete" {
			return fmt.Errorf("NTIA SBOM cannot be generated: %s adapter is %s (%s)", coverage.Adapter, coverage.Status, coverage.Detail)
		}
	}
	return nil
}

func dedupe(components []Component) []Component {
	byID := map[string]Component{}
	for _, component := range components {
		byID[component.ID] = component
	}
	result := make([]Component, 0, len(byID))
	for _, component := range byID {
		result = append(result, component)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func trimSHA256(value string) string { return strings.TrimPrefix(value, "sha256:") }
