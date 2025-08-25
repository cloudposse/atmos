package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

type mockLogin struct{
	validateErr error
	loginErr error
	assumeErr error
	envErr error
	calls []string
}

func (m *mockLogin) Validate() error { m.calls = append(m.calls, "Validate"); return m.validateErr }
func (m *mockLogin) Login() error    { m.calls = append(m.calls, "Login"); return m.loginErr }
func (m *mockLogin) AssumeRole() error { m.calls = append(m.calls, "AssumeRole"); return m.assumeErr }
func (m *mockLogin) Logout() error { m.calls = append(m.calls, "Logout"); return nil }
func (m *mockLogin) SetEnvVars(info *schema.ConfigAndStacksInfo) error { m.calls = append(m.calls, "SetEnvVars"); return m.envErr }

func TestValidateLoginAssumeRole_Success(t *testing.T) {
	m := &mockLogin{}
	info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
	var cfg schema.AtmosConfiguration
	if err := ValidateLoginAssumeRole(m, cfg, info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantOrder := []string{"Validate","Login","AssumeRole","SetEnvVars"}
	if len(m.calls) != len(wantOrder) {
		t.Fatalf("unexpected calls: %v", m.calls)
	}
	for i, c := range wantOrder {
		if m.calls[i] != c {
			t.Fatalf("call %d expected %s got %s", i, c, m.calls[i])
		}
	}
}

func TestValidateLoginAssumeRole_ErrorsPropagate(t *testing.T) {
	cases := []struct{
		name string
		m *mockLogin
		wantSubstr string
	}{
		{"validate", &mockLogin{validateErr: errors.New("bad")}, "validation"},
		{"login", &mockLogin{loginErr: errors.New("bad")}, "login failed"},
		{"assume", &mockLogin{assumeErr: errors.New("bad")}, "assume role failed"},
		{"env", &mockLogin{envErr: errors.New("bad")}, "set env vars failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			info := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{}}
			var cfg schema.AtmosConfiguration
			err := ValidateLoginAssumeRole(tc.m, cfg, info)
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantSubstr, err)
			}
		})
	}
}
