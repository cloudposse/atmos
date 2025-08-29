package auth

import (
	"reflect"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewIdentityDefaults(t *testing.T) {
	id := NewIdentity()
	if !id.Enabled {
		t.Errorf("expected Enabled=true by default")
	}
}

func TestGetAllAndEnabledIdentities(t *testing.T) {
	in := map[string]any{
		"cloudposse": map[string]any{
			"enabled":  true,
			"provider": "aws/iam-identity-center",
			"default":  true,
		},
		"saml": map[string]any{
			"enabled":  true,
			"provider": "aws/saml",
		},
		"invalid": map[string]any{
			"enabled": true,
			// missing provider & role_arn -> filtered out by GetEnabledIdentitiesE
		},
	}

	all, err := GetAllIdentityConfigs(in)
	if err != nil {
		t.Fatalf("GetAllIdentityConfigs error: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(all))
	}

	enabled, err := GetEnabledIdentitiesE(in)
	if err != nil {
		t.Fatalf("GetEnabledIdentitiesE error: %v", err)
	}
	if _, ok := enabled["invalid"]; ok {
		t.Fatalf("expected 'invalid' identity to be filtered out")
	}
	if _, ok := enabled["cloudposse"]; !ok {
		t.Fatalf("expected 'cloudposse' enabled")
	}
}

func TestGetDefaultIdentity(t *testing.T) {
	in := map[string]any{
		"a": map[string]any{"enabled": true, "default": true, "provider": "aws/saml"},
	}
	def, err := GetDefaultIdentity(in)
	if err != nil || def != "a" {
		t.Fatalf("expected default 'a', got %q, err=%v", def, err)
	}

	inMulti := map[string]any{
		"a": map[string]any{"enabled": true, "default": true, "provider": "aws/saml"},
		"b": map[string]any{"enabled": true, "default": true, "provider": "aws/saml"},
	}
	_, err = GetDefaultIdentity(inMulti)
	if err == nil {
		t.Fatalf("expected error for multiple defaults")
	}

	inNone := map[string]any{
		"a": map[string]any{"enabled": true, "default": false, "provider": "aws/saml"},
	}
	_, err = GetDefaultIdentity(inNone)
	if err == nil {
		t.Fatalf("expected error for no default")
	}
}

func TestGetProviderHelpers(t *testing.T) {
	cfg := schema.AuthConfig{
		Providers: map[string]any{
			"okta": map[string]any{"type": "aws/saml", "url": "https://okta.example"},
		},
		Identities: map[string]any{
			"id1": map[string]any{"provider": "okta", "enabled": true},
		},
	}

	typeVal, err := GetType("okta", cfg)
	if err != nil || typeVal != "aws/saml" {
		t.Fatalf("GetType: expected aws/saml, got %q err=%v", typeVal, err)
	}
	idp, err := GetIdp("id1", cfg)
	if err != nil || idp != "okta" {
		t.Fatalf("GetIdp: expected okta, got %q err=%v", idp, err)
	}
}

func TestGetIdentityInstance_AssumeRole(t *testing.T) {
	cfg := schema.AuthConfig{
		DefaultRegion: "us-east-1",
		Providers: map[string]any{
			"": map[string]any{"type": ""}, // maps to NewAwsAssumeRoleFactory in identityRegistry
		},
		Identities: map[string]any{
			"assume": map[string]any{"role_arn": "arn:aws:iam::123456789012:role/Test", "enabled": true},
		},
	}
	lm, err := GetIdentityInstance("assume", cfg, nil)
	if err != nil {
		t.Fatalf("GetIdentityInstance error: %v", err)
	}
	// Ensure the concrete type is awsAssumeRole
	if _, ok := lm.(*awsAssumeRole); !ok {
		t.Fatalf("expected *awsAssumeRole, got %T", lm)
	}
}

func TestGetProviderConfigs(t *testing.T) {
	cfg := schema.AuthConfig{
		DefaultRegion: "us-west-2",
		Providers: map[string]any{
			"okta": map[string]any{"type": "aws/saml", "url": "https://okta.example"},
		},
	}
	out, err := GetProviderConfigs(cfg)
	if err != nil {
		t.Fatalf("GetProviderConfigs error: %v", err)
	}
	// GetProviderConfigs only unmarshals provider defaults; it does not apply DefaultRegion here.
	expected := schema.ProviderDefaultConfig{Type: "aws/saml", Url: "https://okta.example", Region: ""}
	if got := out["okta"]; !reflect.DeepEqual(got.Type, expected.Type) || got.Region != expected.Region || got.Url != expected.Url {
		t.Fatalf("unexpected provider defaults: %#v", got)
	}
}
