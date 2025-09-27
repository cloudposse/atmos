## what

- Fixed path duplication bug when using absolute paths in `terraform.base_path` configuration
- Added check for absolute paths before joining in `atmosConfigAbsolutePaths()` function
- Created comprehensive test suite covering 28 edge case scenarios for path handling
- Fixed handling of absolute paths in `metadata.component` field

## why

- GitHub Actions pipelines were failing with duplicated paths: `/home/runner/_work/infrastructure/infrastructure/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform`
- When `terraform.base_path` is configured with an absolute path, `filepath.Join()` on Unix systems doesn't handle two absolute paths correctly, causing path duplication
- This issue manifested after recent changes to path normalization in Atmos

## references

### Bug Details

The issue occurs when:
1. `atmos.base_path` is set to an absolute path (e.g., `/home/runner/_work/infrastructure/infrastructure`)
2. `components.terraform.base_path` is also set to an absolute path (e.g., `/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform`)
3. On Unix systems, `filepath.Join()` with two absolute paths creates unexpected results

### Fix Implementation

#### pkg/config/config.go (Lines 201-244)
```go
// Before joining, check if component base path is absolute
var terraformBasePath string
if filepath.IsAbs(atmosConfig.Components.Terraform.BasePath) {
    terraformBasePath = atmosConfig.Components.Terraform.BasePath
} else {
    terraformBasePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
}
```

#### pkg/utils/component_path_utils.go (Lines 86-105)
```go
// Check if the component itself is an absolute path
if component != "" && filepath.IsAbs(component) {
    // If component is absolute, use it as the base
    if componentFolderPrefix != "" {
        componentPath = filepath.Join(component, componentFolderPrefix)
    } else {
        componentPath = component
    }
}
```

### Test Coverage

Created comprehensive test files:
- `pkg/config/config_path_absolute_test.go` - Tests InitCliConfig absolute path handling
- `pkg/config/config_path_comprehensive_edge_cases_test.go` - 28 edge case scenarios
- `pkg/utils/component_path_absolute_test.go` - Tests GetComponentPath with absolute paths
- `internal/exec/stack_metadata_component_path_test.go` - Tests metadata.component field handling
- `internal/exec/terraform_component_path_utils_test.go` - Tests constructTerraformComponentWorkingDir

All tests pass successfully, confirming the fix resolves the path duplication issue while maintaining backward compatibility with relative path configurations.
