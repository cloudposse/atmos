package client

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
)

func marshalMCPExport(config mcpclient.MCPJSONConfig, output string) ([]byte, error) {
	if strings.EqualFold(filepath.Ext(output), ".toml") {
		return toml.Marshal(config)
	}
	return json.MarshalIndent(config, "", "  ")
}
