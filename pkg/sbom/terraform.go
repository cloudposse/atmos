//nolint:gocognit,gocritic,revive // Terraform graph construction intentionally keeps lock and module evidence together.
package sbom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
	terraformlock "github.com/cloudposse/atmos/pkg/terraform/lockfile"
)

var errTerraformConfigurationRequired = errors.New("terraform SBOM scope requires configuration")

type terraformModuleDocument struct {
	FormatVersion string            `json:"format_version"`
	Modules       []terraformModule `json:"modules"`
}

type terraformModule struct {
	Key     string `json:"key"`
	Source  string `json:"source"`
	Version string `json:"version"`
}

var runTerraformModules = func(ctx context.Context, executable, directory string) ([]byte, error) {
	command := osExec.CommandContext(ctx, executable, "modules", "-json") // #nosec G204 -- executable is the project's configured Terraform command.
	command.Dir = directory
	return command.Output()
}

func appendTerraform(ctx context.Context, graph *Graph, config *schema.AtmosConfiguration) error {
	if config == nil {
		return errTerraformConfigurationRequired
	}
	base := config.TerraformDirAbsolutePath
	if base == "" {
		base = filepath.Join(config.BasePath, config.Components.Terraform.BasePath)
	}
	locks, err := findProviderLocks(base)
	if err != nil {
		return err
	}
	providerComplete, moduleComplete := true, true
	providerDetail, moduleDetail := "provider locks parsed", "module graphs parsed"
	for _, lockPath := range locks {
		directory := filepath.Dir(lockPath)
		configurationID := "terraform:" + relativeID(base, directory)
		componentPath := repositoryPath(config, directory)
		graph.Components = append(graph.Components, Component{ID: configurationID, Name: relativeID(base, directory), Version: "NOASSERTION", Type: "application", Properties: map[string]string{"atmos:domain": "terraform-configuration", "atmos:lock-file": repositoryPath(config, lockPath), "atmos:component-path": componentPath}})
		linkConfigurationSource(graph, configurationID, componentPath)
		providers, parseErr := terraformlock.ParseFile(lockPath)
		if parseErr != nil {
			return parseErr
		}
		for _, provider := range providers {
			component := Component{ID: "terraform-provider:" + provider.Source + "@" + provider.Version, Name: provider.Source, Version: provider.Version, Type: "library", Source: "https://" + provider.Source, Properties: map[string]string{"atmos:domain": "terraform-provider", "atmos:lock-file": repositoryPath(config, lockPath), "atmos:constraints": provider.Constraints, "atmos:lock-hashes": strings.Join(provider.Hashes, ",")}}
			for _, checksum := range provider.Hashes {
				if strings.HasPrefix(checksum, "zh:") {
					component.SHA256 = strings.TrimPrefix(checksum, "zh:")
					break
				}
			}
			if component.SHA256 == "" {
				providerComplete, providerDetail = false, "one or more providers lack a SHA-256 archive checksum"
			}
			graph.Components = append(graph.Components, component)
			graph.Relationships = append(graph.Relationships, Relationship{From: configurationID, To: component.ID, Type: "depends_on"})
		}
		complete, detail := appendModulesForDirectory(ctx, graph, config, configurationID, directory)
		if !complete {
			moduleComplete, moduleDetail = false, detail
		}
	}
	if len(locks) == 0 {
		providerDetail, moduleDetail = "no Terraform lock files discovered", "no initialized Terraform configurations discovered"
	}
	graph.Coverage = append(graph.Coverage, Coverage{Adapter: "terraform-providers", Status: coverageStatus(providerComplete), Detail: providerDetail}, Coverage{Adapter: "terraform-modules", Status: coverageStatus(moduleComplete), Detail: moduleDetail}, Coverage{Adapter: "oci-artifacts", Status: "complete", Detail: ociArtifactsDetail(graph)})
	return nil
}

// ociArtifactsDetail reports whether any vendor-lock artifact is actually OCI-sourced (its
// "atmos:kind" property, set by appendVendor -- which always runs before appendTerraform -- equals
// "oci"), instead of unconditionally claiming OCI artifacts were represented regardless of whether
// any exist in this project.
func ociArtifactsDetail(graph *Graph) string {
	count := 0
	for _, component := range graph.Components {
		if component.Properties["atmos:domain"] == "vendor" && component.Properties["atmos:kind"] == "oci" {
			count++
		}
	}
	if count == 0 {
		return "no OCI artifacts found in the vendor lock"
	}
	return fmt.Sprintf("%d OCI artifact(s) represented by Atmos source receipts", count)
}

// linkConfigurationSource connects a local Terraform component to the vendor
// or JIT artifact that materialized it. The artifact holds the external source
// URL and immutable identity; the component path remains repository-relative
// metadata rather than a false distribution URL.
func linkConfigurationSource(graph *Graph, configurationID, componentPath string) {
	for _, component := range graph.Components {
		if component.Properties["atmos:target"] != componentPath {
			continue
		}
		if component.Properties["atmos:domain"] != "vendor" && component.Properties["atmos:domain"] != "jit-source" {
			continue
		}
		graph.Relationships = append(graph.Relationships, Relationship{From: configurationID, To: component.ID, Type: "depends_on"})
	}
}

func appendModulesForDirectory(ctx context.Context, graph *Graph, config *schema.AtmosConfiguration, configurationID, directory string) (bool, string) {
	executable := config.Components.Terraform.Command
	if executable == "" {
		executable = "terraform"
	}
	// The 30-second cap applies only to the local `terraform modules -json` subprocess; module
	// artifact resolution below is network-bound and governed by the caller's ctx instead.
	subprocessCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := runTerraformModules(subprocessCtx, executable, directory)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "executable file not found") {
			return false, "Terraform >= 1.10 with modules -json is unavailable"
		}
		return false, fmt.Sprintf("modules -json unavailable for %s", directory)
	}
	var document terraformModuleDocument
	if err := json.Unmarshal(output, &document); err != nil {
		return false, fmt.Sprintf("invalid modules -json output for %s", directory)
	}
	if document.FormatVersion == "" {
		return false, fmt.Sprintf("modules -json did not return format_version for %s", directory)
	}
	complete := true
	detail := "module graphs parsed"
	for _, module := range document.Modules {
		if module.Key == "" || module.Source == "" {
			continue
		}
		artifact, artifactErr := resolveModuleArtifact(ctx, config, directory, module.Source)
		if artifactErr != nil || artifact.Identity == "" {
			complete, detail = false, "one or more modules lack immutable resolution evidence"
		}
		version := module.Version
		if artifact.Identity != "" {
			version = artifact.Identity
		}
		if version == "" {
			version = "NOASSERTION"
		}
		moduleID := "terraform-module:" + configurationID + ":" + module.Key
		graph.Components = append(graph.Components, Component{ID: moduleID, Name: module.Key, Version: version, Type: "library", Source: downloader.RedactSource(module.Source), SHA256: trimSHA256(artifact.Identity), Properties: map[string]string{"atmos:domain": "terraform-module", "atmos:declared-source": downloader.RedactSource(module.Source), "atmos:identity-kind": artifact.Kind}})
		graph.Relationships = append(graph.Relationships, Relationship{From: configurationID, To: moduleID, Type: "depends_on"})
	}
	return complete, detail
}

func resolveModuleArtifact(ctx context.Context, config *schema.AtmosConfiguration, directory, source string) (downloader.ResolvedArtifact, error) {
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || filepath.IsAbs(source) {
		path := source
		if !filepath.IsAbs(path) {
			path = filepath.Join(directory, source)
		}
		return downloader.ResolveArtifact(ctx, config, path, path)
	}
	if strings.HasPrefix(source, "oci://") || strings.Contains(source, "@sha256:") {
		return downloader.ResolveArtifact(ctx, config, source, "")
	}
	if err := ctx.Err(); err != nil {
		return downloader.ResolvedArtifact{}, err
	}
	staging, err := os.MkdirTemp("", "atmos-sbom-module-*")
	if err != nil {
		return downloader.ResolvedArtifact{}, err
	}
	defer os.RemoveAll(staging)
	if err := downloader.NewGoGetterDownloader(config).Fetch(source, staging, downloader.ClientModeAny, moduleFetchTimeout(ctx)); err != nil {
		return downloader.ResolvedArtifact{}, err
	}
	return downloader.ResolveArtifact(ctx, config, source, staging)
}

// moduleFetchTimeout bounds a single module download at 10 minutes, tightened to the caller's
// remaining ctx deadline when one is set so `sbom generate` never outlives its own context.
// (Fetch builds its own timeout context internally, so the deadline must be passed as a duration.)
func moduleFetchTimeout(ctx context.Context) time.Duration {
	timeout := 10 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	return timeout
}

func findProviderLocks(base string) ([]string, error) {
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var locks []string
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".terraform" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == terraformlock.Name {
			locks = append(locks, path)
		}
		return nil
	})
	sort.Strings(locks)
	return locks, err
}

func relativeID(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return "root"
	}
	return filepath.ToSlash(rel)
}

func coverageStatus(complete bool) string {
	if complete {
		return "complete"
	}
	return "incomplete"
}
