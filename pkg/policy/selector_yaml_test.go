package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/tags"
)

func TestTaggedSelectorAttachments(t *testing.T) {
	for _, tc := range []struct {
		selector string
		mode     tags.TagMode
		status   DecisionStatus
	}{
		{"tags: !any [pci, sensitive]", tags.TagModeAny, Deny},
		{"tags: !all [pci, sensitive]", tags.TagModeAll, NotApplicable},
		{"tags: !all [pci]", tags.TagModeAll, Deny},
		{"tags: [pci, sensitive]", "", Deny},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			directory := t.TempDir()
			document := strings.Replace(testPlanManifest, "      on: terraform.plan", "      on: operation\n      match: {labels: {stage: prod}, "+tc.selector+"}", 1)
			document = strings.Replace(document, `"has(input.resource_changes) && input.resource_changes.exists(r, 'delete' in r.change.actions)"`, `"true"`, 1)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(document), 0o600))
			bundle, err := LoadDirectory(directory)
			require.NoError(t, err)
			require.Equal(t, tc.mode, bundle.Rules[0].Match.TagMode)
			subject := &Subject{ObjectIdentity: ObjectIdentity{Name: "vpc", Stack: "prod", ComponentType: "terraform"}, Metadata: SubjectMetadata{Tags: []string{"pci"}, Labels: map[string]string{"stage": "prod"}}}
			decision, err := NewEngine().Evaluate(context.Background(), &Request{Kind: Operation, Subject: subject, Input: json.RawMessage(`{}`)}, bundle.Rules)
			require.NoError(t, err)
			require.Equal(t, tc.status, decision.Status)
			subject.Metadata.Labels["stage"] = "dev"
			decision, err = NewEngine().Evaluate(context.Background(), &Request{Kind: Operation, Subject: subject, Input: json.RawMessage(`{}`)}, bundle.Rules)
			require.NoError(t, err)
			require.Equal(t, NotApplicable, decision.Status)
		})
	}
}

func TestTaggedSelectorRejectsAmbiguousOrExecutableSyntax(t *testing.T) {
	for _, selector := range []string{
		"tags: [pci], tag_mode: any",
		"tags: !all [pci], tag_mode: all",
		"tags: !all [pci], tag_mode: any",
		"tags: !any [pci], tag_mode: all",
		"tags: !all pci",
		"tags: !exec [pci]",
		"tags: [!env PCI]",
		"labels: {stage: !env STAGE}",
		"tags: !any [pci], typo: true",
	} {
		t.Run(selector, func(t *testing.T) {
			directory := t.TempDir()
			document := strings.Replace(testPlanManifest, "      on: terraform.plan", "      on: operation\n      match: {"+selector+"}", 1)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(document), 0o600))
			_, err := LoadDirectory(directory)
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		})
	}
}

func TestSelectorUnmarshalYAMLShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		selector string
		wantErr  bool
		want     Selector
	}{
		"scalar is not a selector":                {selector: "pci", wantErr: true},
		"sequence is not a selector":              {selector: "[pci]", wantErr: true},
		"alias to a literal value is allowed":     {selector: "{components: &c [vpc], stacks: *c}", want: Selector{Components: []string{"vpc"}, Stacks: []string{"vpc"}}},
		"alias inside a list is rejected":         {selector: "{labels: {stage: &s prod}, tags: [*s]}", wantErr: true},
		"executable tag inside any list rejected": {selector: "{tags: !any [!env PCI]}", wantErr: true},
		"nested executable tag is rejected":       {selector: "{labels: {stage: [!exec date]}}", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			var got Selector
			err := yaml.Unmarshal([]byte(tc.selector), &got)
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
