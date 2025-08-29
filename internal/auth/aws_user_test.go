package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/cloudposse/atmos/internal/auth/authstore"
	"github.com/cloudposse/atmos/pkg/schema"
	ini "gopkg.in/ini.v1"
)

func TestAwsUser_Validate(t *testing.T) {
	i := &awsUser{}
	if err := i.Validate(); err == nil {
		t.Fatalf("expected error when profile missing")
	}
	i.Common.Profile = "p"
	// Region should default if empty
	if err := i.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i.Common.Region != "us-east-1" {
		t.Fatalf("expected default region 'us-east-1', got %q", i.Common.Region)
	}
}

func TestAwsUser_SetEnvVars_WritesConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgFile := filepath.Join(tmp, ".aws", "config")
	t.Setenv("HOME", tmp)
	t.Setenv("ATMOS_AWS_CONFIG_FILE", cfgFile)
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	i := &awsUser{}
	i.Common.Region = "us-west-2"
	i.Common.Profile = "src-prof"
	i.Identity.Identity = "user-prof"
	i.Provider = "aws/user"

	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	if err := i.SetEnvVars(info); err != nil {
		t.Fatalf("SetEnvVars: %v", err)
	}

	// Env
	if info.ComponentEnvSection["AWS_PROFILE"] != i.Identity.Identity {
		t.Fatalf("expected AWS_PROFILE %q, got %v", i.Identity.Identity, info.ComponentEnvSection["AWS_PROFILE"])
	}
	if info.ComponentEnvSection["AWS_REGION"] != i.Common.Region {
		t.Fatalf("expected AWS_REGION %q, got %v", i.Common.Region, info.ComponentEnvSection["AWS_REGION"])
	}

	// Config
	f, err := ini.Load(cfgFile)
	if err != nil {
		t.Fatalf("ini load: %v", err)
	}
	sec := f.Section("profile " + i.Identity.Identity)
	if sec == nil {
		t.Fatalf("missing profile section")
	}
	if sec.Key("region").String() != i.Common.Region {
		t.Fatalf("expected region %q, got %q", i.Common.Region, sec.Key("region").String())
	}
	if !sec.HasKey("source_profile") || sec.Key("source_profile").String() != i.Common.Profile {
		t.Fatalf("expected source_profile %q to be set", i.Common.Profile)
	}
	// For user provider, role_arn should not be set by UpdateAwsAtmosConfig
	if sec.HasKey("role_arn") {
		t.Fatalf("did not expect role_arn in user config")
	}
}

func TestAwsUser_Logout_RemovesCredentials(t *testing.T) {
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	if err := os.MkdirAll(filepath.Dir(credFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)

	// Seed a credentials file with the target profile
	f := ini.Empty()
	sec := f.Section("my-user-prof")
	sec.Key("aws_access_key_id").SetValue("AKIA...")
	if err := f.SaveTo(credFile); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	i := &awsUser{}
	i.Common.Profile = "my-user-prof"

	if err := i.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Verify profile section removed
	f2, err := ini.Load(credFile)
	if err != nil {
		t.Fatalf("reload credentials: %v", err)
	}
	if f2.HasSection("my-user-prof") {
		t.Fatalf("expected profile to be removed from credentials")
	}
}

func TestAwsUser_AssumeRole_NoOp(t *testing.T) {
	i := &awsUser{}
	if err := i.AssumeRole(); err != nil {
		t.Fatalf("AssumeRole should be no-op for user: %v", err)
	}
}

func TestAwsUser_keyringAlias(t *testing.T) {
	i := &awsUser{}
	i.Provider = "aws/user"
	// No identity -> provider only
	if got := i.keyringAlias(); got != "aws/user" {
		t.Fatalf("expected alias 'aws/user', got %q", got)
	}
	// With identity -> provider/identity
	i.Identity.Identity = "dev"
	if got := i.keyringAlias(); got != "aws/user/dev" {
		t.Fatalf("expected alias 'aws/user/dev', got %q", got)
	}
}

// mockStore implements a minimal in-memory GenericStore for tests
type mockStore struct {
	data map[string][]byte
	getErr error
}

func (m *mockStore) GetInto(alias string, out any) error {
	if m.getErr != nil {
		return m.getErr
	}
	b, ok := m.data[alias]
	if !ok {
		return fmt.Errorf("not found")
	}
	return json.Unmarshal(b, out)
}

func (m *mockStore) SetAny(alias string, v any) error {
	b, err := json.Marshal(v)
	if err != nil { return err }
	if m.data == nil { m.data = map[string][]byte{} }
	m.data[alias] = b
	return nil
}

func (m *mockStore) Delete(alias string) error { delete(m.data, alias); return nil }

func TestAwsUser_Login_Error_NoStoredCredentials(t *testing.T) {
	// Override keyring store
	old := newKeyringStore
	newKeyringStore = func() authstore.GenericStore { return &mockStore{ data: map[string][]byte{} } }
	defer func(){ newKeyringStore = old }()

	i := &awsUser{}
	i.Provider = "aws/user"
	if err := i.Login(); err == nil {
		t.Fatalf("expected error when no stored credentials")
	}
}

func TestAwsUser_Login_UsesCachedToken_WritesCredentials(t *testing.T) {
	tmp := t.TempDir()
	credFile := filepath.Join(tmp, ".aws", "credentials")
	if err := os.MkdirAll(filepath.Dir(credFile), 0o755); err != nil { t.Fatalf("mkdir: %v", err) }
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credFile)

	i := &awsUser{}
	i.Common.Profile = "p"
	i.Common.Region = "us-west-1"
	i.Provider = "aws/user"

	// Seed mock keyring with a cached token that is valid >30m and has creds
	exp := time.Now().Add(2 * time.Hour)
	secret := awsUserSecret{
		AccessKeyID:     "AKIA_LONG",
		SecretAccessKey: "long_secret",
		LastUpdated:     time.Now().Add(-1 * time.Hour),
		ExpiresAt:       exp,
		TokenOutput: &sts.GetSessionTokenOutput{
			Credentials: &ststypes.Credentials{
				AccessKeyId:     aws.String("ASIA_TEMP"),
				SecretAccessKey: aws.String("temp_secret"),
				SessionToken:    aws.String("temp_token"),
				Expiration:      aws.Time(exp),
			},
		},
	}
	b, _ := json.Marshal(secret)
	alias := i.keyringAlias() // "aws/user"

	old := newKeyringStore
	newKeyringStore = func() authstore.GenericStore {
		return &mockStore{ data: map[string][]byte{ alias: b } }
	}
	defer func(){ newKeyringStore = old }()

	if err := i.Login(); err != nil {
		t.Fatalf("Login error: %v", err)
	}

	// Verify credentials file has profile p with expected values
	f, err := ini.Load(credFile)
	if err != nil { t.Fatalf("load creds: %v", err) }
	if !f.HasSection("p") { t.Fatalf("expected credentials section 'p'") }
	s := f.Section("p")
	if s.Key("aws_access_key_id").String() != "ASIA_TEMP" { t.Fatalf("unexpected access key: %s", s.Key("aws_access_key_id").String()) }
	if s.Key("aws_session_token").String() != "temp_token" { t.Fatalf("unexpected session token: %s", s.Key("aws_session_token").String()) }
}
