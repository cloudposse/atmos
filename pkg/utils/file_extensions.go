package utils

import "strings"

const (
	DefaultStackConfigFileExtension = ".yaml"
	YamlFileExtension               = ".yaml"
	YmlFileExtension                = ".yml"
	YamlTemplateExtension           = ".yaml.tmpl"
	YmlTemplateExtension            = ".yml.tmpl"
	TemplateExtension               = ".tmpl"
)

// IsTemplateFile returns true if the file path has a .yaml.tmpl or .yml.tmpl extension.
func IsTemplateFile(filePath string) bool {
	return strings.HasSuffix(filePath, YamlTemplateExtension) || strings.HasSuffix(filePath, YmlTemplateExtension)
}
