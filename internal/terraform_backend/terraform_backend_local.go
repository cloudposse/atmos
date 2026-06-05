package terraform_backend

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ReadTerraformBackendLocal reads the Terraform state file from the local backend.
// If the state file does not exist, the function returns `nil`.
//
// According to Terraform local backend behavior:
// - For the default workspace: state is stored at `terraform.tfstate`
// - For named workspaces: state is stored at `terraform.tfstate.d/<workspace>/terraform.tfstate`
//
// See: https://github.com/cloudposse/atmos/issues/1920
func ReadTerraformBackendLocal(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	_ *schema.AuthContext, // Auth context not used for local backend.
) ([]byte, error) {
	defer perf.Track(atmosConfig, "terraform_backend.ReadTerraformBackendLocal")()

	workspace := GetTerraformWorkspace(componentSections)
	componentPath := resolveLocalBackendComponentPath(atmosConfig, componentSections)

	var tfStateFilePath string
	if workspace == "" || workspace == "default" {
		// Default workspace: state is stored directly at terraform.tfstate.
		tfStateFilePath = filepath.Join(componentPath, "terraform.tfstate")
	} else {
		// Named workspace: state is stored at terraform.tfstate.d/<workspace>/terraform.tfstate.
		tfStateFilePath = filepath.Join(componentPath, "terraform.tfstate.d", workspace, "terraform.tfstate")
	}

	// If the state file does not exist (the component in the stack has not been provisioned yet), return a `nil` result and no error.
	// On Windows, recently-written files may not be immediately visible due to filesystem latency.
	if !u.FileExists(tfStateFilePath) {
		if runtime.GOOS != "windows" {
			return nil, nil
		}

		time.Sleep(200 * time.Millisecond)
		if !u.FileExists(tfStateFilePath) {
			return nil, nil
		}
	}

	content, err := os.ReadFile(tfStateFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w.\npath: `%s`\nerror: %v", errUtils.ErrReadFile, tfStateFilePath, err)
	}

	return content, nil
}

// resolveLocalBackendComponentPath returns the directory that contains the local
// state file for a component. Resolution order:
//
//  1. If the provisioner already set _workdir_path in sections (e.g. during an
//     active apply), validate it is contained within BasePath and use it.
//     If it escapes BasePath, fall through to derivation (path traversal guard).
//  2. If provision.workdir.enabled is true, derive the canonical workdir path via
//     BuildPath (same formula the provisioner uses), then absolutize.
//  3. Fall back to the static terraform base path for non-JIT components.
func resolveLocalBackendComponentPath(
	atmosConfig *schema.AtmosConfiguration,
	sections *map[string]any,
) string {
	// Fast path: provisioner already stored the concrete workdir path.
	// Validate that the path stays within the project directory (path traversal guard).
	if p, ok := (*sections)[provWorkdir.WorkdirPathKey].(string); ok && p != "" {
		absP, errP := filepath.Abs(p)
		absBase, errBase := filepath.Abs(atmosConfig.BasePath)
		if errP != nil || errBase != nil {
			log.Debug("Could not absolutize _workdir_path or BasePath; falling through to derived path",
				"workdir_path", p, "base_path", atmosConfig.BasePath)
		} else {
			// Note: symlinks are not resolved; this is a best-effort guard against
			// literal path traversal in _workdir_path values from stack config.
			sep := string(filepath.Separator)
			if strings.HasPrefix(absP, absBase+sep) || absP == absBase {
				return absP
			}
			// Path escapes project directory — fall through to derived path.
		}
	}
	// Workdir-enabled component: derive the canonical path using the same
	// formula the provisioner uses. Absolutize for CWD-independence (mirrors
	// config.go:extractComponentPath lines 176-180).
	if provWorkdir.IsWorkdirEnabled(*sections) {
		stack := getAtmosStackFromSections(sections)
		component := getAtmosComponentInstanceFromSections(sections)
		if stack != "" && component != "" {
			workdirPath := provWorkdir.BuildPath(
				atmosConfig.BasePath, "terraform", component, stack, *sections,
			)
			if !filepath.IsAbs(workdirPath) {
				if abs, absErr := filepath.Abs(workdirPath); absErr == nil {
					workdirPath = abs
				}
			}
			// Containment guard: reject derived paths that escape the project directory.
			// atmos_component and atmos_stack come from user-controlled YAML; a value
			// containing ../ sequences (e.g. "../../../../etc/evil") could otherwise
			// escape BasePath via filepath.Join resolution.
			// Note: symlinks are not resolved — same best-effort scope as the fast path above.
			absBase, errBase := filepath.Abs(atmosConfig.BasePath)
			if errBase == nil {
				sep := string(filepath.Separator)
				if strings.HasPrefix(workdirPath, absBase+sep) || workdirPath == absBase {
					return workdirPath
				}
				log.Debug("Derived workdir path escapes project directory; falling through to static path",
					"derived_path", workdirPath, "base_path", atmosConfig.BasePath)
			} else {
				// Cannot absolutize BasePath for containment check; fall through to the
				// static path rather than returning an unverified derived path.
				log.Debug("Could not absolutize BasePath; falling through to static path",
					"derived_path", workdirPath, "base_path", atmosConfig.BasePath)
			}
		}
	}
	// Default: static components/terraform/<component> path.
	return filepath.Join(
		atmosConfig.TerraformDirAbsolutePath,
		GetTerraformComponent(sections),
	)
}

func getAtmosStackFromSections(sections *map[string]any) string {
	s, _ := (*sections)["atmos_stack"].(string)
	return s
}

func getAtmosComponentInstanceFromSections(sections *map[string]any) string {
	if ac, ok := (*sections)["atmos_component"].(string); ok && ac != "" {
		return ac
	}
	return GetTerraformComponent(sections)
}
