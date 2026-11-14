---
title: Native CI on Gitea Actions
tags: [Automation, Emulators]
related_docs:
  - label: "Gitea Actions provider"
    url: /integrations/gitea-actions
  - label: "Native CI configuration"
    url: /cli/configuration/ci
  - label: "Emulator commands"
    url: /cli/commands/emulator/usage
---

# Example: Native CI on Gitea Actions

Run the recommended Atmos CI workflows on **Gitea Actions on your laptop**: no
GitHub account, no hosted runner, and no cloud credentials. A local Gitea server and
a Gitea runner run the same plan, apply, deploy-affected, and deploy-all workflows
that the [GitHub Actions guides](https://atmos.tools/integrations/github-actions)
describe, adapted for Gitea.

[//]: # (editorconfig-checker-disable: Preserve literal output alignment.)
```
git push ──▶ Gitea (emulator) ──job──▶ Gitea runner ──docker──▶ job container (Atmos)
                  ▲                                                     │
                  └────── commit statuses + pull request comments ◀─────┘
```
[//]: # (editorconfig-checker-enable)

The directory has two halves:

- **The harness** (`atmos.yaml`, `stacks/`, `components/container/`) runs on your
  machine. It starts a [Gitea](https://about.gitea.com/) server emulator with Actions
  enabled, and a `gitea/runner` container that registers with it.
- **The project** (`repo/`) is what you push to Gitea. It has its own `atmos.yaml`
  with native CI enabled, a provider-free Terraform component, `dev` and `prod`
  stacks, and the workflows in `repo/.gitea/workflows/`.

## Try It

Prerequisites: Docker. The runner mounts `/var/run/docker.sock` to start job
containers, which gives it root-equivalent access to your Docker daemon, so only run
this locally.

```shell
cd examples/gitea-actions

# Start Gitea (http://localhost:3300, login atmos / atmos) and the runner.
atmos gitea up
```

Create the repository, a token for Atmos, and the variables the workflows read:

```shell
API=http://atmos:atmos@localhost:3300/api/v1
curl -X POST "$API/user/repos" -H 'Content-Type: application/json' \
  -d '{"name":"infra","default_branch":"main"}'

# A scoped token Atmos uses for commit statuses and pull request comments.
TOKEN=$(curl -s -X POST "$API/users/atmos/tokens" -H 'Content-Type: application/json' \
  -d '{"name":"atmos-ci","scopes":["write:repository","write:issue","read:user"]}' | jq -r .sha1)
curl -X PUT "$API/repos/atmos/infra/actions/secrets/ATMOS_CI_GITEA_TOKEN" \
  -H 'Content-Type: application/json' -d "{\"data\":\"$TOKEN\"}"

# The Atmos image tag the jobs run. Use a release that includes the Gitea provider.
curl -X POST "$API/repos/atmos/infra/actions/variables/ATMOS_VERSION" \
  -H 'Content-Type: application/json' -d '{"value":"<atmos version>"}'
```

Push the project. The push to `main` runs `apply.yml`:

```shell
HARNESS_DIR=$PWD
cp -R repo /tmp/infra && cd /tmp/infra
git init -b main && git add -A && git commit -m "Add infrastructure"
git push http://atmos:atmos@localhost:3300/atmos/infra.git main
```

Open a pull request that changes only `dev` to run `plan.yml` and
`deploy-affected.yml`:

```shell
git checkout -b change-dev
sed -i.bak 's/Hello from dev/Hello again from dev/' stacks/deploy/dev.yaml && rm stacks/deploy/dev.yaml.bak
git commit -am "Change the dev message"
git push http://atmos:atmos@localhost:3300/atmos/infra.git change-dev
curl -X POST "$API/repos/atmos/infra/pulls" -H 'Content-Type: application/json' \
  -d '{"title":"Change the dev message","head":"change-dev","base":"main"}'
```

Watch the runs at http://localhost:3300/atmos/infra/actions. The pull request gets
`atmos/plan/prod/demo` and `atmos/plan/dev/demo` commit statuses and an Atmos plan
comment. `deploy-affected.yml` plans only `dev`, the instance the change affects.
Run `deploy-all.yml` from the Actions tab to deploy every instance.

Tear it down:

```shell
cd "$HARNESS_DIR"
atmos gitea down
```

## What Changes from GitHub Actions

| | GitHub Actions | Gitea Actions |
|---|---|---|
| Workflow directory | `.github/workflows/` | `.gitea/workflows/` |
| Checkout | `actions/checkout` | `atmos git clone --ci`: the Atmos image has no Node.js for JavaScript actions |
| Token | `GITHUB_TOKEN` | `ATMOS_CI_GITEA_TOKEN` secret; Atmos never sends `GITHUB_TOKEN` to Gitea |
| `permissions:` | `id-token`, `checks`, `statuses`, `pull-requests` | Restrict the job token where supported; Atmos reports through commit statuses |
| Checks | Check runs | Commit statuses |
| Apply on merge | Verifies the plan reviewed in the pull request | Plans the merged commit fresh |

## How Jobs Reach Gitea

The emulator and the runner join the stack's shared network, `atmos-local`, where
Gitea's alias is `local-gitea`. The runner starts each job container on the same
network (`container.network` in `runner.yaml`), and Gitea advertises
`http://local-gitea:3000` as its URL, so jobs clone and call the API through it. From
your machine, use the published port, `http://localhost:3300`.

The local server speaks plain HTTP, so `runner.yaml` sets
`ATMOS_CI_GITEA_ALLOW_INSECURE=true` for jobs. Never set it against a real server.

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | The harness project and the `atmos gitea up` / `atmos gitea down` commands |
| `stacks/deploy/local.yaml` | The Gitea emulator and the runner container |
| `components/container/gitea-runner/runner.yaml` | Runner configuration: job network, labels, and job environment |
| `repo/atmos.yaml` | The project's native CI configuration |
| `repo/.gitea/workflows/` | Plan, apply, deploy-affected, and deploy-all workflows |
