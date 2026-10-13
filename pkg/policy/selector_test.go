package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/tags"
)

func TestSelectorMatching(t *testing.T) {
	subject := &Subject{
		ObjectIdentity: ObjectIdentity{Name: "vpc", Stack: "prod", ComponentType: "terraform"},
		Metadata:       SubjectMetadata{Tags: []string{"pci", "network"}, Labels: map[string]string{"stage": "prod", "team": "platform"}},
	}
	for _, tc := range []struct {
		name     string
		selector *Selector
		match    bool
	}{
		{"unscoped", nil, true},
		{"all instances", &Selector{}, true},
		{"label", &Selector{Labels: map[string]string{"stage": "prod"}}, true},
		{"all labels", &Selector{Labels: map[string]string{"stage": "prod", "team": "other"}}, false},
		{"missing label", &Selector{Labels: map[string]string{"missing": "value"}}, false},
		{"any tag", &Selector{Tags: []string{"pci", "other"}}, true},
		{"all tags", &Selector{Tags: []string{"pci", "other"}, TagMode: tags.TagModeAll}, false},
		{"no tag", &Selector{Tags: []string{"other"}}, false},
		{"combined", &Selector{Tags: []string{"pci"}, Labels: map[string]string{"stage": "prod"}, Components: []string{"vpc"}, Stacks: []string{"prod"}, ComponentTypes: []string{"terraform"}}, true},
		{"wrong component", &Selector{Components: []string{"app"}}, false},
		{"wrong stack", &Selector{Stacks: []string{"dev"}}, false},
		{"wrong type", &Selector{ComponentTypes: []string{"helm"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.selector.Matches(subject)
			require.NoError(t, err)
			require.Equal(t, tc.match, actual)
		})
	}
	_, err := (&Selector{}).Matches(nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	_, err = (&Selector{}).Matches(&Subject{})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	_, err = (&Selector{TagMode: "invalid"}).Matches(subject)
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	_, err = (&Selector{Tags: []string{"!env POLICY_TAG"}}).Matches(subject)
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	subject.TemplateLeft = "[["
	subject.Metadata.Labels["stage"] = "[[ .vars.stage ]]"
	_, err = (&Selector{}).Matches(subject)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestSubjectMetadataIsolationAndValidation(t *testing.T) {
	identity := ObjectIdentity{Name: "app", Stack: "prod", ComponentType: "helm"}
	metadata := map[string]any{"tags": []any{"pci"}, "labels": map[string]any{"stage": "prod"}}
	subject, err := NewSubject(identity, metadata)
	require.NoError(t, err)
	subject.Metadata.Labels["stage"] = "dev"
	require.Equal(t, "prod", metadata["labels"].(map[string]any)["stage"])
	metadata["tags"].([]any)[0] = "changed"
	require.Equal(t, []string{"pci"}, subject.Metadata.Tags)
	subject, err = NewSubject(identity, map[string]any{"tags": []string{"[[ .vars.tag ]]"}}, "[[", "]]")
	require.NoError(t, err)
	require.Equal(t, "[[", subject.TemplateLeft)
	_, err = (&Selector{Tags: []string{"pci"}}).Matches(subject)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	subject, err = NewSubject(identity, nil)
	require.NoError(t, err)
	require.Empty(t, subject.Metadata.Tags)
	require.NotNil(t, subject.Metadata.Labels)
	for _, invalid := range []any{map[string]any{"tags": "pci"}, map[string]any{"labels": map[string]any{"stage": 42}}, make(chan int)} {
		_, err = NewSubject(identity, invalid)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	}
	for _, metadata := range []any{
		map[string]any{"tags": []string{"!exec echo pci"}},
		map[string]any{"labels": map[string]string{"stage": "{{ .vars.stage }}"}},
		map[string]any{"labels": map[string]string{"{{ .key }}": "prod"}},
	} {
		subject, err = NewSubject(identity, metadata)
		require.NoError(t, err)
		_, err = (&Selector{Labels: map[string]string{"stage": "prod"}}).Matches(subject)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	}
}

func TestSelectorDirectoryAndEngine(t *testing.T) {
	directory := t.TempDir()
	document := strings.Replace(testPlanManifest, "      on: terraform.plan", "      match:\n        labels: {stage: prod}\n      on: operation", 1)
	document = strings.Replace(document, `"has(input.resource_changes) && input.resource_changes.exists(r, 'delete' in r.change.actions)"`, `"request.subject.metadata.labels.stage == 'prod'"`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(document), 0o600))
	bundle, err := LoadDirectory(directory)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"stage": "prod"}, bundle.Rules[0].Match.Labels)
	var engine Engine
	require.NoError(t, engine.Register(CELEngine, &CEL{}))
	request := &Request{Kind: Operation, Input: json.RawMessage(`{}`)}
	decision, err := engine.Evaluate(context.Background(), request, bundle.Rules)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Equal(t, EvaluationError, decision.Status)
	request.Subject = &Subject{ObjectIdentity: ObjectIdentity{Name: "app", Stack: "prod", ComponentType: "helm"}, Metadata: SubjectMetadata{Labels: map[string]string{"stage": "prod"}}}
	decision, err = engine.Evaluate(context.Background(), request, bundle.Rules)
	require.NoError(t, err)
	require.Equal(t, Deny, decision.Status)
	request.Subject.Metadata.Labels["stage"] = "dev"
	decision, err = engine.Evaluate(context.Background(), request, bundle.Rules)
	require.NoError(t, err)
	require.Equal(t, NotApplicable, decision.Status)
}

func TestSelectorDirectoryRejectsInvalidAttachments(t *testing.T) {
	for name, attachment := range map[string]string{
		"affected requires separate snapshots": "      on: atmos.affected\n      match: {labels: {owner: platform}}",
		"unresolved selector":                  "      on: operation\n      match: {tags: ['!env POLICY_TAG']}",
		"invalid tag mode":                     "      on: operation\n      match: {tags: [pci], tag_mode: none}",
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			document := strings.Replace(testPlanManifest, "      on: terraform.plan", attachment, 1)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(document), 0o600))
			bundle, err := LoadDirectory(directory)
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
			require.Nil(t, bundle)
		})
	}
}
