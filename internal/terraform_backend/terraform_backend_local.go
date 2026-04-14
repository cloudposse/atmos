package terraform_backend

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
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
//     active apply), use it directly.
//  2. If provision.workdir.enabled is true, derive the canonical workdir path via
//     BuildPath (same formula the provisioner uses).
//  3. Fall back to the static terraform base path for non-JIT components.
//
// BasePath is used (not BasePathAbsolute) for consistency with the source and
// workdir provisioners, which use the same field when calling BuildPath.
func resolveLocalBackendComponentPath(
	atmosConfig *schema.AtmosConfiguration,
	sections *map[string]any,
) string {
	// Fast path: provisioner already stored the concrete workdir path.
	if p, ok := (*sections)[provWorkdir.WorkdirPathKey].(string); ok && p != "" {
		return p
	}
	// Workdir-enabled component: derive the canonical path using the same
	// formula the provisioner uses.
	if provWorkdir.IsWorkdirEnabled(*sections) {
		stack := getAtmosStackFromSections(sections)
		component := getAtmosComponentInstanceFromSections(sections)
		if stack != "" && component != "" {
			return provWorkdir.BuildPath(
				atmosConfig.BasePath, "terraform", component, stack, *sections,
			)
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
