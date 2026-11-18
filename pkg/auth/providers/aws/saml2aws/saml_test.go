package saml2aws

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractAwsRoles(t *testing.T) {
	data, err := os.ReadFile("testdata/assertion.xml")
	assert.Nil(t, err)

	roles, err := ExtractAwsRoles(data)
	assert.Nil(t, err)
	assert.Len(t, roles, 2)
}

func TestExtractAwsRolesFail(t *testing.T) {
	data, err := os.ReadFile("testdata/notxml.xml")
	assert.Nil(t, err)

	_, err = ExtractAwsRoles(data)
	assert.Error(t, err)
}

func TestExtractSessionDuration(t *testing.T) {
	data, err := os.ReadFile("testdata/assertion.xml")
	assert.Nil(t, err)

	duration, err := ExtractSessionDuration(data)
	assert.Nil(t, err)
	assert.Equal(t, int64(28800), duration)
}

func TestExtractSessionDurationFail(t *testing.T) {
	data, err := os.ReadFile("testdata/notxml.xml")
	assert.Nil(t, err)

	_, err = ExtractSessionDuration(data)
	assert.Error(t, err)
}

func TestExtractDestinationURL(t *testing.T) {
	data, err := os.ReadFile("testdata/assertion.xml")
	assert.Nil(t, err)

	destination, err := ExtractDestinationURL(data)
	assert.Nil(t, err)
	assert.Equal(t, "https://signin.aws.amazon.com/saml", destination)
}

func TestExtractDestinationURLFail(t *testing.T) {
	data, err := os.ReadFile("testdata/notxml.xml")
	assert.Nil(t, err)

	_, err = ExtractDestinationURL(data)
	assert.Error(t, err)
}

func TestExtractDestinationURL2(t *testing.T) {
	data, err := os.ReadFile("testdata/assertion_no_destination.xml")
	assert.Nil(t, err)

	destination, err := ExtractDestinationURL(data)
	assert.Nil(t, err)
	assert.Equal(t, "https://signin.aws.amazon.com/saml", destination)
}

func TestExtractMFATokenDuration(t *testing.T) {
	data, err := os.ReadFile("testdata/assertion.xml")
	assert.Nil(t, err)

	timeObject, err := ExtractMFATokenExpiryTime(data)

	assert.Nil(t, err)
	assert.Equal(t, "2016-09-10T02:59:39Z", timeObject.Format(time.RFC3339))
}

func TestExtractMFATokenDurationFail(t *testing.T) {
	data, err := os.ReadFile("testdata/notxml.xml")
	assert.Nil(t, err)

	_, err = ExtractMFATokenExpiryTime(data)

	assert.Error(t, err)
}

func TestExtractMFATokenDuration2(t *testing.T) {
	data, err := os.ReadFile("testdata/assertion_invalid_date.xml")
	assert.Nil(t, err)

	_, err = ExtractMFATokenExpiryTime(data)
	t.Log(err)
	assert.NotNil(t, err)
}

// samlResponse wraps a namespaced SAML 2.0 assertion in a Response element.
func samlResponse(responseAttrs, assertionBody string) []byte {
	return []byte(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"` + responseAttrs + `>` +
		`<saml2:Assertion xmlns:saml2="urn:oasis:names:tc:SAML:2.0:assertion">` + assertionBody +
		`</saml2:Assertion></samlp:Response>`)
}

const (
	roleAttribute = `<saml2:Attribute Name="https://aws.amazon.com/SAML/Attributes/Role">` +
		`<saml2:AttributeValue>arn:aws:iam::123456789012:role/admin,arn:aws:iam::123456789012:saml-provider/idp</saml2:AttributeValue>` +
		`<saml2:AttributeValue>arn:aws:iam::210987654321:role/read,arn:aws:iam::210987654321:saml-provider/idp</saml2:AttributeValue>` +
		`</saml2:Attribute>`
	otherAttribute = `<saml2:Attribute Name="https://aws.amazon.com/SAML/Attributes/RoleSessionName">` +
		`<saml2:AttributeValue>alice</saml2:AttributeValue></saml2:Attribute>`
)

func sessionDurationAttribute(value string) string {
	return `<saml2:Attribute Name="https://aws.amazon.com/SAML/Attributes/SessionDuration">` +
		`<saml2:AttributeValue>` + value + `</saml2:AttributeValue></saml2:Attribute>`
}

func attributeStatement(attrs ...string) string {
	body := ""
	for _, a := range attrs {
		body += a
	}
	return `<saml2:AttributeStatement>` + body + `</saml2:AttributeStatement>`
}

func TestErrMissingElementError(t *testing.T) {
	assert.Equal(t, "missing Assertion element", ErrMissingAssertion.Error())
	assert.Equal(t, "missing Destination attribute on Response element",
		ErrMissingElement{Tag: responseTag, Attribute: "Destination"}.Error())
}

func TestExtractSessionDurationCases(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    int64
		wantErr error
		anyErr  bool
	}{
		{
			name: "duration present",
			data: samlResponse("", attributeStatement(otherAttribute, sessionDurationAttribute("43200"))),
			want: 43200,
		},
		{
			name: "duration absent defaults to zero",
			data: samlResponse("", attributeStatement(otherAttribute)),
			want: 0,
		},
		{
			name:   "duration not a number",
			data:   samlResponse("", attributeStatement(sessionDurationAttribute("eight hours"))),
			anyErr: true,
		},
		{
			name:    "no assertion",
			data:    []byte(`<Response/>`),
			wantErr: ErrMissingAssertion,
		},
		{
			name:    "no attribute statement",
			data:    samlResponse("", `<saml2:Subject/>`),
			wantErr: ErrMissingElement{Tag: attributeStatementTag},
		},
		{
			name: "non-namespaced assertion",
			data: []byte(`<Response><Assertion><AttributeStatement>` +
				`<Attribute Name="https://aws.amazon.com/SAML/Attributes/SessionDuration"><AttributeValue>900</AttributeValue></Attribute>` +
				`</AttributeStatement></Assertion></Response>`),
			want: 900,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractSessionDuration(tt.data)
			switch {
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			case tt.anyErr:
				assert.Error(t, err)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestExtractDestinationURLCases(t *testing.T) {
	missing := ErrMissingElement{Tag: responseTag}

	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr error
	}{
		{
			name: "destination on response",
			data: samlResponse(` Destination="https://signin.amazonaws-us-gov.com/saml"`, ""),
			want: "https://signin.amazonaws-us-gov.com/saml",
		},
		{
			name: "recipient fallback",
			data: samlResponse("", `<saml2:Subject><saml2:SubjectConfirmation><saml2:SubjectConfirmationData Recipient="https://signin.amazonaws.cn/saml"/></saml2:SubjectConfirmation></saml2:Subject>`),
			want: "https://signin.amazonaws.cn/saml",
		},
		{
			name:    "no root element",
			data:    []byte(`<?xml version="1.0"?>`),
			wantErr: missing,
		},
		{
			name:    "no destination and no subject confirmation data",
			data:    samlResponse("", ""),
			wantErr: missing,
		},
		{
			name:    "subject confirmation data without recipient",
			data:    samlResponse("", `<saml2:Subject><saml2:SubjectConfirmation><saml2:SubjectConfirmationData NotOnOrAfter="2030-01-01T00:00:00Z"/></saml2:SubjectConfirmation></saml2:Subject>`),
			wantErr: missing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractDestinationURL(tt.data)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractMFATokenExpiryTimeCases(t *testing.T) {
	missing := ErrMissingElement{Tag: responseTag}

	tests := []struct {
		name    string
		data    []byte
		want    time.Time
		wantErr error
	}{
		{
			name: "not on or after present",
			data: samlResponse("", `<saml2:Subject><saml2:SubjectConfirmation><saml2:SubjectConfirmationData NotOnOrAfter="2030-01-02T03:04:05Z"/></saml2:SubjectConfirmation></saml2:Subject>`),
			want: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		},
		{
			name:    "no root element",
			data:    []byte(`<?xml version="1.0"?>`),
			wantErr: missing,
		},
		{
			name:    "no subject confirmation data",
			data:    samlResponse("", ""),
			wantErr: missing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractMFATokenExpiryTime(tt.data)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(got), "got %s, want %s", got, tt.want)
		})
	}
}

func TestExtractAwsRolesCases(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []string
		wantErr error
	}{
		{
			name: "roles extracted in order, other attributes ignored",
			data: samlResponse("", attributeStatement(otherAttribute, roleAttribute)),
			want: []string{
				"arn:aws:iam::123456789012:role/admin,arn:aws:iam::123456789012:saml-provider/idp",
				"arn:aws:iam::210987654321:role/read,arn:aws:iam::210987654321:saml-provider/idp",
			},
		},
		{
			name: "no role attribute",
			data: samlResponse("", attributeStatement(otherAttribute)),
			want: []string{},
		},
		{
			name:    "no assertion",
			data:    []byte(`<Response/>`),
			wantErr: ErrMissingAssertion,
		},
		{
			name:    "no attribute statement",
			data:    samlResponse("", `<saml2:Subject/>`),
			wantErr: ErrMissingElement{Tag: attributeStatementTag},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractAwsRoles(tt.data)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractFromFixtureRolesContent(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "assertion.xml"))
	require.NoError(t, err)

	roles, err := ExtractAwsRoles(data)
	require.NoError(t, err)
	require.Len(t, roles, 2)

	parsed, err := ParseAWSRoles(roles)
	require.NoError(t, err)
	require.Len(t, parsed, 2)
	for _, role := range parsed {
		assert.Contains(t, role.RoleARN, ":role/")
		assert.Contains(t, role.PrincipalARN, ":saml-provider/")
	}
}

func TestExtractFunctionsRejectMalformedXML(t *testing.T) {
	malformed := []byte(`<Response><Assertion></Response>`)

	_, err := ExtractSessionDuration(malformed)
	assert.Error(t, err)

	_, err = ExtractDestinationURL(malformed)
	assert.Error(t, err)

	_, err = ExtractMFATokenExpiryTime(malformed)
	assert.Error(t, err)

	roles, err := ExtractAwsRoles(malformed)
	assert.Error(t, err)
	assert.Empty(t, roles)
}
