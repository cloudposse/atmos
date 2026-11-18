package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"

	"gopkg.in/yaml.v3"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

//go:embed plugin.yaml
var manifestData []byte

// fallbackVersion is used only if the embedded manifest cannot be read, which a test prevents.
const fallbackVersion = "0.0.0"

type manifestHeader struct {
	Metadata struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
}

// pluginVersion returns the version in the embedded manifest. The daemon records it, and the
// service compares it with the installed copy.
func pluginVersion() string {
	var m manifestHeader
	if err := yaml.Unmarshal(manifestData, &m); err != nil || m.Metadata.Version == "" {
		return fallbackVersion
	}
	return m.Metadata.Version
}

// description must agree with plugin.yaml: the host compares the digest and the exports.
func description() *pluginv1.DescribeResponse {
	var m manifestHeader
	if err := yaml.Unmarshal(manifestData, &m); err != nil {
		panic("invalid embedded manifest")
	}
	digest := sha256.Sum256(manifestData)
	return &pluginv1.DescribeResponse{
		PluginName: m.Metadata.Name, PluginVersion: m.Metadata.Version,
		ManifestDigest: "sha256:" + hex.EncodeToString(digest[:]),
		Services: []*pluginv1.ServiceVersionRange{
			{Service: "auth.provider", Min: 1, Max: 1},
			{Service: "auth.identity", Min: 1, Max: 1},
			{Service: "command", Min: 1, Max: 1},
		},
		Exports: []*pluginv1.Export{
			{Service: "auth.provider", Kind: "ssh/self"},
			{Service: "auth.identity", Kind: "ssh/key"},
			{Service: "command", Kind: "ssh"},
		},
	}
}
