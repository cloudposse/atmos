---
name: atmos-aws-eks
description: "AWS EKS commands in Atmos: atmos aws eks update-kubeconfig, atmos aws eks token, kubeconfig generation, kubectl exec credentials, EKS auth integrations"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos AWS EKS

Use this skill for Atmos commands that connect AWS EKS clusters to local Kubernetes tooling.
It owns `atmos aws eks update-kubeconfig` and `atmos aws eks token`.

## Command Model

`atmos aws eks update-kubeconfig` writes or prints kubeconfig entries for an EKS cluster. It can
run from explicit CLI arguments, a component and stack, an `auth.integrations` entry, or an Atmos
identity with explicit cluster details.

```shell
atmos aws eks update-kubeconfig <component> -s <stack>
atmos aws eks update-kubeconfig --profile dev --name dev-cluster
atmos aws eks update-kubeconfig --integration dev/eks/primary
atmos aws eks update-kubeconfig --name dev-cluster --region us-east-2 --identity dev-admin
```

`atmos aws eks token` generates a Kubernetes `ExecCredential` token for kubectl. It is normally
called by kubectl from generated kubeconfig rather than run by humans.

```shell
atmos aws eks token --cluster-name dev-cluster --region us-east-2 --identity dev-admin
```

## Configuration

For integration mode, configure an `aws/eks` integration in `auth.integrations`. Route provider,
identity, AWS SSO, SAML, OIDC, assume role, and assume root details to `atmos-auth`.

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start/

  identities:
    dev-admin:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: AdministratorAccess
        account: dev

  integrations:
    dev/eks/primary:
      kind: aws/eks
      via:
        identity: dev-admin
      spec:
        cluster:
          name: dev-cluster
          region: us-east-2
          alias: dev-eks
```

## Agent Guidance

- Prefer `--integration` when a named EKS integration exists; it centralizes cluster name, region,
  alias, and identity selection.
- Use `--identity` with `--name` and `--region` for ad hoc kubeconfig generation through Atmos Auth.
- Use `--profile` or `--role-arn` only when the workflow intentionally relies on AWS CLI-style
  credentials rather than Atmos Auth.
- Use `--dry-run` when reviewing kubeconfig output or avoiding writes to the user's kubeconfig.
- Do not hard-code kubeconfig paths unless the repo already has a convention. Check
  `components.helmfile.kubeconfig_path` and Helmfile settings first when using component/stack mode.
- If the AWS CLI or kubectl must be installed for a scripted job, route tool installation to
  `atmos-toolchain`.

## Routing

| Need | Skill |
|------|-------|
| AWS identity/provider setup, SSO, SAML, OIDC, assume role/root | `atmos-auth` |
| Helmfile/EKS deployment behavior after kubeconfig exists | `atmos-helmfile` |
| Installing `aws`, `kubectl`, or other command-line tools | `atmos-toolchain` |
| Component/stack lookup for cluster settings | `atmos-components`, `atmos-stacks` |
