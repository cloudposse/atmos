# atmos-pro-setup fixture

Minimal multi-tenant stack hierarchy used to validate the `atmos-pro` skill's template
renderer. Three tenants × two stages produces 6 target accounts, one of which is the
root account (pinned to plan role in the apply profile).

## Layout

```
atmos-pro-setup/
  README.md          - this file
  fixture.json       - canonical RenderData used for golden-snapshot tests
  golden/            - expected rendered output, one file per template
  stacks/            - placeholder (not exercised in the renderer test)
```

## Usage

Consumed by `pkg/ai/skills/atmospro/render_test.go`. Running
`go test ./pkg/ai/skills/atmospro/...` renders the templates against `fixture.json`
and diffs every output against its twin in `golden/`. Drift fails the test.

## Regenerating the golden snapshot

After intentional template changes:

```bash
go test ./pkg/ai/skills/atmospro/... -regenerate-snapshots
git diff tests/fixtures/scenarios/atmos-pro-setup/golden/
```

Review the diff carefully — the generator encodes the contract between the skill and every
downstream consumer (AI agents, a future `atmos pro init` command, docs).
