//go:build test
// +build test

package exec

import (
	"path/filepath"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Declare package-level variables mapping to real functions so tests can override them.
var (
	findStacksMapFn               = FindStacksMap
	processTmplFn                 = ProcessTmpl
	processTmplWithDatasourcesFn  = ProcessTmplWithDatasources
	processCustomYamlTagsFn       = ProcessCustomYamlTags
	writeToFileAsJSONFn           = u.WriteToFileAsJSON
	writeTerraformBackendConfigHCLFn = u.WriteTerraformBackendConfigToFileAsHcl
	writeToFileAsHCLFn            = u.WriteToFileAsHcl
	ensureDirFn                   = u.EnsureDir
	absFn                         = filepath.Abs
	replaceContextTokensFn        = cfg.ReplaceContextTokens
)