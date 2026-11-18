package saml2aws

import (
	"fmt"
	"regexp"
	"strings"
)

// AWSRole aws role attributes
type AWSRole struct {
	RoleARN      string
	PrincipalARN string
	Name         string
}

// ParseAWSRoles parses and splits the roles while also validating the contents
func ParseAWSRoles(roles []string) ([]*AWSRole, error) {
	awsRoles := make([]*AWSRole, len(roles))

	for i, role := range roles {
		awsRole, err := parseRole(role)
		if err != nil {
			return nil, err
		}

		awsRoles[i] = awsRole
	}

	return awsRoles, nil
}

// arnPattern matches a complete ARN: arn:<partition>:<service>:<region>:<account>:<resource>.
var arnPattern = regexp.MustCompile(`^arn:[^:\n]*:[^:\n]*:[^:\n]*:[^:\n]*:[^\n]+$`)

// arnSeparator is the comma that separates two ARNs in a role attribute value. It only matches when a
// complete role or SAML provider ARN follows, because IAM role paths may themselves contain commas and colons.
var arnSeparator = regexp.MustCompile(`,\s*arn:aws[-a-z]*:iam::\d{12}:(?:role|saml-provider)/`)

// splitRoleARNs splits a "<arn>,<arn>" role attribute value into its ARNs.
//
// Atmos fork: upstream treated every comma as an ARN boundary, so a role name containing a comma
// (which IAM permits) was silently truncated. A comma only separates ARNs when a complete role or SAML provider ARN follows it,
// so paths such as "/team,arn:blue/" stay intact.
func splitRoleARNs(role string) []string {
	role = strings.TrimSpace(role)
	var parts []string
	start := 0
	for _, loc := range arnSeparator.FindAllStringIndex(role, -1) {
		parts = append(parts, role[start:loc[0]])
		// Skip the comma and any whitespace; the next ARN begins at "arn:".
		start = loc[0] + strings.Index(role[loc[0]:], "arn:")
	}
	parts = append(parts, role[start:])

	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); arnPattern.MatchString(part) {
			tokens = append(tokens, part)
		}
	}
	return tokens
}

// arnResourceHasPrefix reports whether the resource component of an ARN (everything after the fifth colon) starts with prefix.
func arnResourceHasPrefix(arn, prefix string) bool {
	const arnResourceField = 6 // arn:partition:service:region:account:resource.
	fields := strings.SplitN(strings.TrimSpace(arn), ":", arnResourceField)
	return len(fields) == arnResourceField && strings.HasPrefix(fields[arnResourceField-1], prefix)
}

func parseRole(role string) (*AWSRole, error) {
	tokens := splitRoleARNs(role)

	if len(tokens) != 2 {
		return nil, fmt.Errorf("invalid role string only %d tokens", len(tokens))
	}

	awsRole := &AWSRole{}

	for _, token := range tokens {
		// Atmos fork: classify by the ARN resource type, not by text anywhere in the ARN, so a role path
		// that contains "saml-provider/" is not mistaken for a SAML provider.
		switch {
		case arnResourceHasPrefix(token, "saml-provider/"):
			awsRole.PrincipalARN = strings.TrimSpace(token)
		case arnResourceHasPrefix(token, "role/"):
			awsRole.RoleARN = strings.TrimSpace(token)
		}
	}

	if awsRole.PrincipalARN == "" {
		return nil, fmt.Errorf("unable to locate PrincipalARN in: %s", role)
	}

	if awsRole.RoleARN == "" {
		return nil, fmt.Errorf("unable to locate RoleARN in: %s", role)
	}

	return awsRole, nil
}
