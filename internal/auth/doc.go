// Package auth provides pluggable authentication methods used by Atmos to
// obtain AWS credentials and configure the runtime environment for commands
// like "atmos terraform". The package exposes a small interface, LoginMethod,
// implemented by several providers. Each provider knows how to:
//   - Validate its configuration (Validate)
//   - Perform an authentication/login flow (Login)
//   - Optionally exchange the login artifact for AWS role credentials (AssumeRole)
//   - Configure process environment and AWS config/profile for downstream tools (SetEnvVars)
//   - Remove any cached credentials (Logout)
//
// # Configuration model
//
// Providers and identities are configured via the Atmos config (e.g. atmos.yaml)
// under the top-level "auth" section, represented by schema.AuthConfig.
//
//	auth:
//	  default_region: "us-east-1"
//	  providers:
//	    idp-acme-sso:
//	      type: aws/iam-identity-center
//	      url: https://acme.awsapps.com/start/
//	      profile: acme-identity
//	    idp-acme-saml:
//	      type: aws/saml
//	      url: https://accounts.google.com/o/saml2/initsso?...  # IdP SAML entry URL
//	      profile: acme-identity
//	    oidc-gha:
//	      type: aws/oidc
//	      profile: gha
//	  identities:
//	    acme-sso:                      # logical identity name
//	      default: true
//	      idp: idp-acme-sso            # reference a provider by name
//	      role_name: IdentityManagersTeamAccess
//	      account_name: core-identity
//	    acme-saml:
//	      idp: idp-acme-saml
//	      role_arn: arn:aws:iam::111111111111:role/acme-core-gbl-identity-managers
//	    analytics-admin:
//	      role_arn: arn:aws:iam::222222222222:role/acme-core-analytics-admin  # assume-role (no idp)
//	    gha-oidc:
//	      idp: oidc-gha
//	      role_arn: arn:aws:iam::333333333333:role/gha-ci
//
// # Supported provider types
//
// The following provider types are registered (see identityRegistry in load.go):
//   - "aws/iam-identity-center" → AWS IAM Identity Center (SSO) interactive device auth
//   - "aws/saml"                → SAML via a browser flow (saml2aws Browser provider)
//   - "aws/oidc"                → OIDC web identity token exchange (e.g., GitHub Actions)
//   - "aws/user"                → Long-lived user access keys exchanged for session token (MFA supported)
//   - "" (empty)                → AssumeRole without an IdP provider (source creds must be present)
//
// # Usage
//
// Most callers obtain a LoginMethod for a given identity using GetIdentityInstance,
// then drive the lifecycle:
//
//	lm, err := auth.GetIdentityInstance("acme-sso", cfg.Auth, info)
//	if err != nil { /* handle */ }
//	if err := lm.Validate(); err != nil { /* handle */ }
//	if err := lm.Login(); err != nil { /* handle */ }
//	if err := lm.AssumeRole(); err != nil { /* handle */ }
//	if err := lm.SetEnvVars(info); err != nil { /* handle */ }
//
// See implementations:
//   - aws_identity_center.go (type: aws/iam-identity-center)
//   - aws_saml.go             (type: aws/saml)
//   - aws_oidc.go             (type: aws/oidc)
//   - aws_user.go             (type: aws/user)
//   - aws_assume_role.go      (assume-role without explicit provider)
package auth
