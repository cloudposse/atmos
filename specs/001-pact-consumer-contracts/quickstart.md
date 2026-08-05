# Pact Contract Testing — README Section Draft

> This is the draft content for the README.md "Pact Contract Testing" section.
> Add it to the **Contributing / Development** area of the README.

---

## Pact Contract Testing

Atmos uses [Pact](https://docs.pact.io/) to define consumer contracts between the Atmos
CLI and the [Atmos Pro](https://atmos-pro.com) backend API. Consumer pact tests run the
real `AtmosProAPIClient` against a pact mock server, generating JSON contract files that
document the exact request/response shapes the client expects.

Contract files are stored in `pacts/` and checked into version control so any drift in
the API client surface is visible in PR diffs.

### Prerequisites

Pact consumer tests require native pact libraries. Install them once per machine:

```bash
# Add pact-go as a dev dependency (already in go.mod after first checkout)
go install github.com/pact-foundation/pact-go/v2@latest

# Download and install the required native libraries (~15 MB, one-time setup)
pact-go -l DEBUG install
```

> **macOS note**: If `pact-go` is not on your `PATH` after `go install`, add
> `$(go env GOPATH)/bin` to your shell's `PATH`.

### Running the consumer tests

```bash
# Run all pact consumer tests and regenerate pacts/atmos-AtmosPro.json
go test -tags pact ./pkg/pro/... -v -run TestPact

# Run a single interaction
go test -tags pact ./pkg/pro/... -v -run TestPact/UploadAffectedStacks
```

Pact tests are **not** included in the standard `go test ./...` run. They require the
`pact` build tag and the native libraries installed above.

### Generated files

After a successful run, the following file is created or updated:

```text
pacts/
└── atmos-AtmosPro.json    # Consumer contract: all 8 Atmos Pro API interactions
```

This file is the source of truth for what the Atmos client expects from the Atmos Pro
API. Commit it alongside any change that intentionally alters the request or response
shape of an Atmos Pro API method.

### Covered interactions

| Interaction | Method | Path |
|-------------|--------|------|
| UploadAffectedStacks | POST | `/api/v1/affected-stacks` |
| LockStack | POST | `/api/v1/locks` |
| UnlockStack | DELETE | `/api/v1/locks` |
| GetGitHubOIDCToken | GET | `ACTIONS_ID_TOKEN_REQUEST_URL` (mocked) |
| ExchangeOIDCToken | POST | `/api/v1/auth/github-oidc` |
| UploadInstances | POST | `/api/v1/instances` |
| UploadInstanceStatus | PATCH | `/api/v1/repos/{owner}/{repo}/instances` |
| CreateCommit | POST | `/api/v1/git/commit` |

### What to do when a pact test fails

A failing pact test means the Atmos client code no longer matches the defined contract.
There are two cases:

1. **Intentional change** (you modified a request/response shape on purpose):
   - Run the tests to regenerate `pacts/atmos-AtmosPro.json`.
   - Review the diff in the pact file — confirm the change is what you intended.
   - Commit both the code change and the updated pact file.

2. **Accidental regression** (the test fails unexpectedly):
   - Read the pact failure output to find the mismatched field.
   - Fix the client code to match the contract, or update the contract if the API
     intentionally changed.

### Provider verification (future)

Consumer contracts are local-only today. A future effort will wire up provider
verification, allowing the Atmos Pro team to confirm that their API still satisfies the
contracts defined here.
