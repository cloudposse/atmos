package validation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/version"
)

const (
	sarifSchemaURL = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion   = "2.1.0"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri,omitempty"`
	Rules          []sarifRule `json:"rules,omitempty"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name,omitempty"`
	ShortDescription sarifMessage `json:"shortDescription,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex,omitempty"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIF serializes the report as an indented SARIF 2.1.0 document.
func (r Report) SARIF() ([]byte, error) {
	diagnostics := r.sortedDiagnostics()
	rules, ruleIndex := sarifRules(diagnostics)
	results := make([]sarifResult, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result := sarifResult{
			RuleID:    diagnostic.RuleID,
			RuleIndex: ruleIndex[diagnostic.RuleID],
			Level:     sarifLevel(diagnostic.Severity),
			Message:   sarifMessage{Text: diagnostic.Message},
		}
		if diagnostic.File != "" {
			location := sarifLocation{PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: strings.ReplaceAll(diagnostic.File, "\\", "/")},
			}}
			if diagnostic.Line > 0 {
				location.PhysicalLocation.Region = &sarifRegion{
					StartLine:   diagnostic.Line,
					StartColumn: diagnostic.Column,
					EndLine:     diagnostic.EndLine,
					EndColumn:   diagnostic.EndColumn,
				}
			}
			result.Locations = []sarifLocation{location}
		}
		results = append(results, result)
	}

	log := sarifLog{
		Schema:  sarifSchemaURL,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "atmos",
				Version:        version.Version,
				InformationURI: "https://atmos.tools",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
	bytes, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal validation SARIF: %w", err)
	}
	return append(bytes, '\n'), nil
}

func sarifRules(diagnostics []Diagnostic) ([]sarifRule, map[string]int) {
	rules := make([]sarifRule, 0)
	indexes := make(map[string]int)
	for _, diagnostic := range diagnostics {
		if _, ok := indexes[diagnostic.RuleID]; ok {
			continue
		}
		indexes[diagnostic.RuleID] = len(rules)
		rules = append(rules, sarifRule{
			ID:               diagnostic.RuleID,
			Name:             diagnostic.Source,
			ShortDescription: sarifMessage{Text: diagnostic.RuleID},
		})
	}
	return rules, indexes
}

func sarifLevel(severity Severity) string {
	switch severity {
	case SeverityNotice:
		return "note"
	case SeverityWarning:
		return "warning"
	default:
		return "error"
	}
}
