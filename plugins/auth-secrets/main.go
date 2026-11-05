package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/sdk/api"
	"github.com/cloudposse/atmos/sdk/plugin"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

//go:embed plugin.yaml
var manifestData []byte

func description() *pluginv1.DescribeResponse {
	var manifest struct {
		Metadata struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil {
		panic("invalid embedded manifest")
	}
	digest := sha256.Sum256(manifestData)
	return &pluginv1.DescribeResponse{
		PluginName: manifest.Metadata.Name, PluginVersion: manifest.Metadata.Version,
		ManifestDigest: "sha256:" + hex.EncodeToString(digest[:]),
		Services:       []*pluginv1.ServiceVersionRange{{Service: "auth.provider", Min: 1, Max: 1}, {Service: "auth.identity", Min: 1, Max: 1}},
		Exports:        []*pluginv1.Export{{Service: "auth.provider", Kind: "secrets/store"}, {Service: "auth.identity", Kind: "secrets/env"}},
	}
}

func main() {
	plugin.Main(plugin.Config{Description: description(), Build: func(client *api.Client) plugin.Services {
		return plugin.Services{Provider: &provider{secrets: client}, Identity: &identity{}}
	}})
}
