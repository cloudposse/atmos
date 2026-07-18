//nolint:gocritic,lintroller,revive // Rendering must preserve one-to-one graph mappings across both standard formats.
package sbom

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	atmosversion "github.com/cloudposse/atmos/pkg/version"
)

const atmosRepositoryURL = "https://github.com/cloudposse/atmos"

var errUnsupportedFormat = errors.New("unsupported SBOM format")

// Render serializes graph to a supported JSON SBOM format.
func Render(graph *Graph, format string) ([]byte, error) {
	if graph == nil {
		graph = &Graph{}
	}
	switch format {
	case FormatCycloneDXJSON:
		return json.MarshalIndent(cycloneDX(graph), "", "  ")
	case FormatSPDXJSON:
		return json.MarshalIndent(spdx(graph), "", "  ")
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedFormat, format)
	}
}

func cycloneDX(graph *Graph) map[string]any {
	components := make([]map[string]any, 0, len(graph.Components))
	for _, component := range graph.Components {
		componentID := publicIdentifier(component.ID)
		entry := map[string]any{"bom-ref": componentID, "type": component.Type, "name": publicValue(component.Name), "version": publicValue(component.Version)}
		if component.PURL != "" && !isLocalFilesystemPath(component.PURL) {
			entry["purl"] = component.PURL
		}
		if component.Supplier != "" && !isLocalFilesystemPath(component.Supplier) {
			entry["supplier"] = map[string]string{"name": component.Supplier}
		}
		if component.SHA256 != "" {
			entry["hashes"] = []map[string]string{{"alg": "SHA-256", "content": component.SHA256}}
		}
		if isDistributionURL(component.Source) {
			entry["externalReferences"] = []map[string]string{{"type": "distribution", "url": component.Source}}
		}
		propertyNames := make([]string, 0, len(component.Properties))
		for name := range component.Properties {
			propertyNames = append(propertyNames, name)
		}
		sort.Strings(propertyNames)
		properties := make([]map[string]string, 0, len(propertyNames))
		for _, name := range propertyNames {
			if isLocalFilesystemPath(component.Properties[name]) {
				continue
			}
			properties = append(properties, map[string]string{"name": name, "value": component.Properties[name]})
		}
		entry["properties"] = properties
		components = append(components, entry)
	}
	dependencies := make([]map[string]any, 0, len(graph.Relationships))
	for _, relationship := range graph.Relationships {
		dependencies = append(dependencies, map[string]any{"ref": publicIdentifier(relationship.From), "dependsOn": []string{publicIdentifier(relationship.To)}})
	}
	metadata := map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339), "tools": atmosToolComponent(), "properties": coverageProperties(graph)}
	if graph.Subject.Name != "" {
		metadata["component"] = map[string]any{"bom-ref": "atmos:subject", "type": "application", "name": publicValue(graph.Subject.Name), "version": publicValue(graph.Subject.Version), "supplier": map[string]string{"name": publicValue(graph.Subject.Supplier)}}
	}
	return map[string]any{
		"bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1,
		"metadata":     metadata,
		"components":   components,
		"dependencies": dependencies,
	}
}

func spdx(graph *Graph) map[string]any {
	packages := make([]map[string]any, 0, len(graph.Components))
	for _, component := range graph.Components {
		downloadLocation := component.Source
		if !isDistributionURL(downloadLocation) {
			downloadLocation = ""
		}
		entry := map[string]any{"SPDXID": spdxID(publicIdentifier(component.ID)), "name": publicValue(component.Name), "versionInfo": publicValue(component.Version), "downloadLocation": downloadLocation, "filesAnalyzed": false}
		if downloadLocation == "" {
			entry["downloadLocation"] = "NOASSERTION"
		}
		if component.SHA256 != "" {
			entry["checksums"] = []map[string]string{{"algorithm": "SHA256", "checksumValue": component.SHA256}}
		}
		if component.PURL != "" {
			entry["externalRefs"] = []map[string]string{{"referenceCategory": "PACKAGE-MANAGER", "referenceType": "purl", "referenceLocator": component.PURL}}
		}
		if component.Supplier != "" && !isLocalFilesystemPath(component.Supplier) {
			entry["supplier"] = component.Supplier
		}
		packages = append(packages, entry)
	}
	relationships := make([]map[string]string, 0, len(graph.Relationships))
	for _, relationship := range graph.Relationships {
		relationships = append(relationships, map[string]string{"spdxElementId": spdxID(publicIdentifier(relationship.From)), "relationshipType": spdxRelationship(relationship.Type), "relatedSpdxElement": spdxID(publicIdentifier(relationship.To))})
	}
	if graph.Subject.Name != "" {
		relationships = append(relationships, map[string]string{"spdxElementId": "SPDXRef-DOCUMENT", "relationshipType": "DESCRIBES", "relatedSpdxElement": spdxID("atmos:subject")})
	}
	return map[string]any{
		"spdxVersion": "SPDX-2.3", "dataLicense": "CC0-1.0", "SPDXID": "SPDXRef-DOCUMENT", "name": documentName(graph), "documentNamespace": "https://atmos.tools/sbom/locks", "creationInfo": map[string]any{"created": time.Now().UTC().Format(time.RFC3339), "creators": []string{"Tool: atmos-" + atmosversion.Version}, "comment": coverageComment(graph) + "; generator repository: " + atmosRepositoryURL}, "packages": packages, "relationships": relationships,
	}
}

// atmosToolComponent uses CycloneDX 1.6's non-deprecated tooling component
// form. It describes the actual generator without claiming a binary checksum
// or a build commit that the running process cannot prove.
func atmosToolComponent() map[string]any {
	return map[string]any{"components": []map[string]any{{
		"type":     "application",
		"name":     "atmos",
		"version":  atmosversion.Version,
		"supplier": map[string]string{"name": "Cloud Posse, LLC"},
		"purl":     "pkg:golang/github.com/cloudposse/atmos@" + atmosversion.Version,
		"externalReferences": []map[string]string{{
			"type": "vcs",
			"url":  atmosRepositoryURL,
		}},
	}}}
}

// isDistributionURL prevents local component paths from leaking into SBOM
// distribution/download fields. Local paths identify the build workspace, not
// a package location consumers can retrieve or verify.
func isDistributionURL(value string) bool {
	if isLocalFilesystemPath(value) {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Scheme == "file" {
		return false
	}
	return true
}

// isLocalFilesystemPath identifies local paths without interpreting repository-relative
// paths as local-machine evidence. SBOMs may contain repository-relative provenance,
// but must never disclose an operator's workspace path.
func isLocalFilesystemPath(value string) bool {
	if value == "" {
		return false
	}
	if filepath.IsAbs(value) || strings.HasPrefix(strings.ToLower(value), "file://") {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

func publicValue(value string) string {
	if isLocalFilesystemPath(value) {
		return "REDACTED_LOCAL_PATH"
	}
	return value
}

func publicIdentifier(value string) string {
	if !isLocalFilesystemPath(value) {
		return value
	}
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("atmos:redacted-local-path:%x", digest[:8])
}

func coverageProperties(graph *Graph) []map[string]string {
	properties := make([]map[string]string, 0, len(graph.Coverage)+1)
	properties = append(properties, map[string]string{"name": "atmos:sbom-kind", "value": "provenance/build-input"})
	for _, coverage := range graph.Coverage {
		properties = append(properties, map[string]string{"name": "atmos:coverage:" + coverage.Adapter, "value": coverage.Status + ": " + publicValue(coverage.Detail)})
	}
	return properties
}

func coverageComment(graph *Graph) string {
	parts := make([]string, 0, len(graph.Coverage))
	for _, coverage := range graph.Coverage {
		parts = append(parts, publicValue(coverage.Adapter)+"="+coverage.Status)
	}
	return "Atmos provenance/build-input SBOM; coverage: " + strings.Join(parts, ", ")
}

func documentName(graph *Graph) string {
	if graph.Subject.Name != "" {
		return publicValue(graph.Subject.Name)
	}
	return "atmos-provenance"
}

func spdxRelationship(kind string) string {
	if kind == "contains" {
		return "CONTAINS"
	}
	return "DEPENDS_ON"
}

func spdxID(id string) string {
	var value strings.Builder
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '.' || character == '-' {
			value.WriteRune(character)
			continue
		}
		value.WriteByte('-')
	}
	return "SPDXRef-" + value.String()
}
