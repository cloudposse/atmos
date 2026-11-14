package utils

import (
	"os"

	"github.com/hashicorp/hcl/v2/hclwrite"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WriteTerraformBackendConfigToFileAsHcl writes the provided Terraform backend config to the specified file.
// https://dev.to/pdcommunity/write-terraform-files-in-go-with-hclwrite-2e1j
// https://pkg.go.dev/github.com/hashicorp/hcl/v2/hclwrite
func WriteTerraformBackendConfigToFileAsHcl(
	filePath string,
	backendType string,
	backendConfig map[string]any,
) error {
	defer perf.Track(nil, "utils.WriteTerraformBackendConfigToFileAsHcl")()

	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()
	tfBlock := rootBody.AppendNewBlock("terraform", nil)
	tfBlockBody := tfBlock.Body()
	backendBlock := tfBlockBody.AppendNewBlock("backend", []string{backendType})
	backendBlockBody := backendBlock.Body()

	backendConfigSortedKeys := StringKeysFromMap(backendConfig)

	for _, name := range backendConfigSortedKeys {
		v := backendConfig[name]
		ctyVal := GoToCty(v)
		backendBlockBody.SetAttributeValue(name, ctyVal)
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Warn(err.Error())
		}
	}(f)

	_, err = f.Write(hclFile.Bytes())
	if err != nil {
		return err
	}

	return nil
}
