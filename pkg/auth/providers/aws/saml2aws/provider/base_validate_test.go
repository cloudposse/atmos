package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

func TestLoginDetails_Validate(t *testing.T) {
	type fields struct {
		Username string
		Password string
		URL      string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr error
	}{
		// Each failing case sets every other field, so only the field under test can cause the failure.
		{name: "hostname missing error", fields: fields{Username: "test", Password: "test"}, wantErr: ErrEmptyURL},
		{name: "username missing error", fields: fields{URL: "id.example.com", Password: "test"}, wantErr: ErrEmptyUsername},
		{name: "password missing error", fields: fields{URL: "id.example.com", Username: "test"}, wantErr: ErrEmptyPassword},
		{name: "all missing reports the URL first", fields: fields{}, wantErr: ErrEmptyURL},
		{name: "ok", fields: fields{URL: "id.example.com", Username: "test", Password: "test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ld := &creds.LoginDetails{
				Username: tt.fields.Username,
				Password: tt.fields.Password,
				URL:      tt.fields.URL,
			}
			providerstub := &ValidateBase{}
			err := providerstub.Validate(ld)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
