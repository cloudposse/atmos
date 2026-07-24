// Package sbom builds provenance and software-bill-of-material graphs from
// Atmos lock files and supported dependency adapters.
//
//nolint:gocritic,lintroller,nestif // Graph assembly keeps adapter ordering and coverage validation explicit.
package sbom

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
	// ScopeTerraform selects Terraform provider and module evidence (the CLI's default scope).
	ScopeTerraform = "terraform"
	// ScopeDependencies selects toolchain and version-track lock evidence instead of Terraform.
	ScopeDependencies = "dependencies"
)

var (
	errUnsupportedMode        = errors.New("unsupported SBOM mode")
	errUnsupportedScope       = errors.New("unsupported SBOM scope")
	errNTIASubjectRequired    = errors.New("NTIA SBOM requires subject name, version, and supplier")
	errNTIAScopeRequired      = errors.New("NTIA SBOM requires an explicit supported scope")
	errNTIACoverageIncomplete = errors.New("NTIA SBOM cannot be generated")
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

func BuildWithOptions(config *schema.AtmosConfiguration, options Options) (*Graph, error) {
	if err := normalizeOptions(&options); err != nil {
		return nil, err
	}
	graph := &Graph{Subject: options.Subject}
	if options.Subject.Name != "" {
		graph.Components = append(graph.Components, subjectComponent(options.Subject))
	}
	if err := appendVendor(graph, config, options.IncludeFiles); err != nil {
		return nil, err
	}
	if err := appendScopeArtifacts(graph, config, options.Scope); err != nil {
		return nil, err
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

func normalizeOptions(options *Options) error {
	if options.Mode == "" {
		options.Mode = ModeProvenance
	}
	if options.Mode != ModeProvenance && options.Mode != ModeNTIA {
		return fmt.Errorf("%w: %s", errUnsupportedMode, options.Mode)
	}
	if options.Scope == "" {
		options.Scope = ScopeTerraform
	}
	if options.Scope != ScopeTerraform && options.Scope != ScopeDependencies {
		return fmt.Errorf("%w: %s", errUnsupportedScope, options.Scope)
	}
	return nil
}

func appendScopeArtifacts(graph *Graph, config *schema.AtmosConfiguration, scope string) error {
	if scope == ScopeDependencies {
		if err := appendToolchain(graph, config); err != nil {
			return err
		}
		return appendVersions(graph, config)
	}
	return appendTerraform(graph, config)
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
			name := artifact.Name
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
	state := jitSourceState{complete: true}
	err := filepath.WalkDir(config.BasePath, jitSourceWalkFunc(graph, config, &state))
	if err != nil {
		return false, "failed to inspect JIT receipts", err
	}
	if state.receipts == 0 {
		return true, "vendor lock parsed; no JIT receipts found", nil
	}
	if !state.complete {
		return false, "one or more JIT receipts lack immutable source identity", nil
	}
	return true, "vendor lock and JIT receipts parsed", nil
}

// jitSourceWalkFunc builds the filepath.WalkDirFunc used to inspect JIT
// workdir receipts, keeping the walk logic separate from appendJITSources'
// overall result assembly.
func jitSourceWalkFunc(graph *Graph, config *schema.AtmosConfiguration, state *jitSourceState) fs.WalkDirFunc {
	return func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == ".terraform") {
			return filepath.SkipDir
		}
		if !isJITReceipt(entry, path) {
			return nil
		}
		return state.append(graph, config, path)
	}
}

type jitSourceState struct {
	complete bool
	receipts int
}

func isJITReceipt(entry os.DirEntry, path string) bool {
	return !entry.IsDir() && entry.Name() == workdir.MetadataFile && filepath.Base(filepath.Dir(path)) == workdir.AtmosDir
}

func (state *jitSourceState) append(graph *Graph, config *schema.AtmosConfiguration, path string) error {
	receipt, err := loadJITReceipt(path)
	if err != nil {
		return err
	}
	state.receipts++
	identity := receipt.SourceIdentity
	if identity == "" {
		state.complete = false
		identity = "NOASSERTION"
	}
	target := filepath.Dir(filepath.Dir(path))
	graph.Components = append(graph.Components, Component{
		ID:      "jit:" + filepath.ToSlash(relativeToBase(config.BasePath, target)),
		Name:    receipt.Component,
		Version: identity,
		Type:    "library",
		Source:  firstNonEmpty(receipt.SourceResolved, receipt.SourceURI),
		SHA256:  trimSHA256(identity),
		Properties: map[string]string{
			"atmos:domain":  "jit-source",
			"atmos:receipt": repositoryPath(config, path),
			"atmos:target":  repositoryPath(config, target),
			"atmos:stack":   receipt.Stack,
		},
	})
	return nil
}

func loadJITReceipt(path string) (workdir.WorkdirMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return workdir.WorkdirMetadata{}, err
	}
	var receipt workdir.WorkdirMetadata
	if err := json.Unmarshal(data, &receipt); err != nil {
		return workdir.WorkdirMetadata{}, fmt.Errorf("parse JIT workdir receipt %q: %w", path, err)
	}
	return receipt, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
		return errNTIASubjectRequired
	}
	if scope != ScopeTerraform {
		return errNTIAScopeRequired
	}
	for _, coverage := range graph.Coverage {
		if coverage.Status != "complete" {
			return fmt.Errorf("%w: %s adapter is %s (%s)", errNTIACoverageIncomplete, coverage.Adapter, coverage.Status, coverage.Detail)
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
