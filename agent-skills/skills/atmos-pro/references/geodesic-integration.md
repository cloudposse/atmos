# Running the Skill Inside Geodesic

Geodesic is a containerized shell that many Atmos users run as their primary dev environment.
The skill works inside a Geodesic shell with two adjustments:

1. The worktree must be bind-mounted so the container can read/write it.
2. `gh` and `git` authentication tokens must be passed through.

## Detecting Geodesic

Any of the following indicates Geodesic is in use:

- `Dockerfile` with `FROM cloudposse/geodesic`
- `geodesic.mk` in the repo root
- `.envrc` with a `geodesic` invocation
- `Makefile` with a `cloudposse/geodesic` target

See [`starting-conditions.md`](starting-conditions.md) for the full detection probe.

## Invocation inside a Geodesic shell

Once inside the Geodesic container (e.g., after running `./geodesic` or the user's equivalent
entry script):

```shell
# Inside the Geodesic shell
cd /path/to/repo
git worktree add .worktrees/atmos-pro-setup feat/atmos-pro
cd .worktrees/atmos-pro-setup
atmos ai ask "setup atmos pro" --skill atmos-pro
```

## Passing GitHub credentials through

`gh pr create` requires authentication. Inside Geodesic, the host's `gh` auth is not
automatically available. Two options:

### Option 1: `GITHUB_TOKEN` environment variable

Before entering the Geodesic shell:

```shell
export GITHUB_TOKEN=$(gh auth token)
./geodesic
```

Inside the shell, `gh` picks up `GITHUB_TOKEN` automatically.

### Option 2: Volume-mount `gh` config

The Geodesic shell already mounts `$HOME` for some users. If so, `gh` config is preserved. If
not, add to the user's Geodesic entry command:

```shell
docker run -v "$HOME/.config/gh:/root/.config/gh" ... cloudposse/geodesic
```

The skill detects the absence of `gh auth` on startup and tells the user which option to use.

## Skill detection of Geodesic context

The skill checks `/.dockerenv` to determine if it's running inside a container. If it is
**and** the repo detection flagged Geodesic, the skill adjusts the playbook:

1. Skip the "install `gh`" step — it's pre-installed in Geodesic.
2. Warn if `GITHUB_TOKEN` is unset.
3. Use container-friendly paths in the PR body's rollout checklist (e.g., `atmos` is in
   `/usr/local/bin`, already on PATH).

## The Geodesic section in the generated `docs/atmos-pro.md`

When Geodesic is detected, the generated `docs/atmos-pro.md` gets an additional section:

```markdown
## Running inside Geodesic

This repo uses Geodesic as its dev environment. The Atmos Pro workflows run in their own
container (`ghcr.io/cloudposse/atmos:${ATMOS_VERSION}`) and do not depend on Geodesic, but
local operators who want to replay the skill or debug manually should enter the Geodesic
shell first:

    export GITHUB_TOKEN=$(gh auth token)
    ./geodesic

Then run the skill from inside the shell:

    atmos ai ask "setup atmos pro" --skill atmos-pro
```

## Testing the skill inside Geodesic

Phase 3 of the PRD adds an integration test where the skill runs inside the `cloudposse/geodesic`
container against a fixture repo. The test asserts:

1. The skill produces the same generated files as the non-Geodesic run.
2. `gh pr create` succeeds (mocked with `GITHUB_TOKEN` pointing at a local fixture server).
3. `atmos validate stacks` passes on the generated output.

Failures inside Geodesic but not outside are bugs against this document.
