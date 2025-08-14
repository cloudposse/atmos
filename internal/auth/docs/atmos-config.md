# Atmos.yaml Configuration

```yaml
auth:
  default_region: "us-east-2"
  session_length: 15 minutes
  providers:
    idp-acme-saml:
      type: aws/saml
      url: https://accounts.google.com/o/saml2/initsso?idpid=Foo&spid=Bar&forceauthn=false
      idp_arn: arn:aws:iam::000000000000:saml-provider/acme-core-gbl-identity
    idp-acme-sso:
      type: aws/iam-identity-center
      url: https://acme.awsapps.com/start/
  identities:
    acme-sso:
      default: true
      idp: idp-acme-sso
      profile: "acme-identity"
      role_name: "IdentityManagersTeamAccess"
      account_name: core-identity
    acme-devops:
      enabled: false
      role_arn: "arn:aws:iam::000000000000:role/acme-core-gbl-analytics-admin"
      profile: "acme-identity"
    acme-saml:
      enabled: false
      idp: idp-acme-saml
      role_arn: "arn:aws:iam::000000000000:role/acme-core-gbl-identity-managers"
      profile: "acme-identity"

```
