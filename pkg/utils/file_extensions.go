package utils

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	DefaultStackConfigFileExtension = ".yaml"
	YamlFileExtension               = ".yaml"
	YmlFileExtension                = ".yml"
	YamlTemplateExtension           = ".yaml.tmpl"
	YmlTemplateExtension            = ".yml.tmpl"
	TemplateExtension               = ".tmpl"
	JSONFileExtension               = ".json"
	HCLFileExtension                = ".hcl"
)

// StackConfigExtensions returns all supported stack configuration file extensions.
// The order determines the priority when searching for files without explicit extensions.
func StackConfigExtensions() []string {
	return []string{
		YamlFileExtension,
		YmlFileExtension,
		JSONFileExtension,
		HCLFileExtension,
		YamlTemplateExtension,
		YmlTemplateExtension,
	}
}

// IsStackConfigFile returns true if the file path has a supported stack configuration extension.
func IsStackConfigFile(filePath string) bool {
	for _, ext := range StackConfigExtensions() {
		if strings.HasSuffix(filePath, ext) {
			return true
		}
	}
	return false
}

// IsTemplateFile returns true if the file path has a .yaml.tmpl or .yml.tmpl extension.
func IsTemplateFile(filePath string) bool {
	defer perf.Track(nil, "utils.IsTemplateFile")()

	return strings.HasSuffix(filePath, YamlTemplateExtension) || strings.HasSuffix(filePath, YmlTemplateExtension)
}
