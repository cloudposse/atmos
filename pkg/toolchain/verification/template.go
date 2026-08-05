package verification

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

type templateData struct {
	Version         string
	SemVer          string
	OS              string
	Arch            string
	GOOS            string
	GOARCH          string
	RepoOwner       string
	RepoName        string
	Format          string
	Asset           string
	AssetWithoutExt string
}

func renderTemplateString(templateStr string, tool *registry.Tool, version, assetName string, extraReplacements map[string]string) (string, error) {
	data := buildTemplateData(tool, version, assetName, extraReplacements)
	result, err := render(templateStr, data)
	if err != nil {
		return "", err
	}
	if strings.Contains(templateStr, ".Asset") {
		data.Asset = assetName
		data.AssetWithoutExt = stripFileExtension(assetName)
		result, err = render(templateStr, data)
		if err != nil {
			return "", err
		}
	}
	return result, nil
}

//nolint:cyclop,revive // Template data mirrors Aqua fields and platform override behavior in one place.
func buildTemplateData(tool *registry.Tool, version, assetName string, extraReplacements map[string]string) *templateData {
	prefix := tool.VersionPrefix
	releaseVersion := version
	if prefix != "" && !strings.HasPrefix(releaseVersion, prefix) {
		releaseVersion = prefix + releaseVersion
	}
	semVer := version
	if prefix != "" {
		semVer = strings.TrimPrefix(releaseVersion, prefix)
	}

	osVal := runtime.GOOS
	archVal := runtime.GOARCH
	if tool.Rosetta2 && osVal == "darwin" && archVal == "arm64" {
		archVal = "amd64"
	}
	if tool.WindowsArmEmulation && osVal == "windows" && archVal == "arm64" {
		archVal = "amd64"
	}
	replacements := make(map[string]string)
	for k, v := range tool.Replacements {
		replacements[k] = v
	}
	for k, v := range extraReplacements {
		replacements[k] = v
	}
	if replacement, ok := replacements[osVal]; ok {
		osVal = replacement
	}
	if replacement, ok := replacements[archVal]; ok {
		archVal = replacement
	}

	format := tool.Format
	for _, fo := range tool.FormatOverrides {
		if fo.GOOS == runtime.GOOS {
			format = fo.Format
			break
		}
	}

	return &templateData{
		Version:         releaseVersion,
		SemVer:          semVer,
		OS:              osVal,
		Arch:            archVal,
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		RepoOwner:       tool.RepoOwner,
		RepoName:        tool.RepoName,
		Format:          format,
		Asset:           assetName,
		AssetWithoutExt: stripFileExtension(assetName),
	}
}

func render(templateStr string, data *templateData) (string, error) {
	tmpl, err := template.New("verification").Funcs(templateFuncs()).Parse(templateStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func templateFuncs() template.FuncMap {
	funcs := sprig.TxtFuncMap()
	delete(funcs, "env")
	delete(funcs, "expandenv")
	delete(funcs, "getHostByName")
	funcs["trimV"] = func(s string) string {
		return strings.TrimPrefix(s, "v")
	}
	funcs["trimPrefix"] = func(prefix, s string) string {
		return strings.TrimPrefix(s, prefix)
	}
	funcs["trimSuffix"] = func(suffix, s string) string {
		return strings.TrimSuffix(s, suffix)
	}
	funcs["replace"] = func(old, new, s string) string {
		return strings.ReplaceAll(s, old, new)
	}
	return funcs
}

func stripFileExtension(name string) string {
	compoundExts := []string{".tar.gz", ".tar.xz", ".tar.bz2"}
	lower := strings.ToLower(name)
	for _, ext := range compoundExts {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
}
