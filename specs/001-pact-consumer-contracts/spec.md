# Feature Specification: Pact Consumer Contract Testing for Atmos Pro API

**Feature Branch**: `1198-pact-consumer-contracts`

**Created**: 2026-06-09

**Status**: Draft

**Input**: User description: "Introduce pacts — contracts testing for Atmos Pro API endpoints"

## Clarifications

### Session 2026-06-09

- Q: Should consumer pact tests cover only the 5 originally listed endpoints, or all 8 endpoints in `pkg/pro/`? → A: All 8 endpoints (`UploadAffectedStacks`, `LockStack`, `UnlockStack`, GitHub OIDC GET, OIDC exchange, `UploadInstances`, `UploadInstanceStatus`, `CreateCommit`).
- Q: Should generated pact contract files be published to a Pact Broker/PactFlow or kept as local/repo artifacts? → A: Local/repo artifacts only — no broker publishing in this feature.
- Q: Should pact consumer tests run in a dedicated CI job, be tag-gated within the existing test job, or run locally only? → A: Local only — pact tests are never run automatically in CI.

## User Scenarios & Testing

### User Story 1 - Verify API Client Contracts Locally (Priority: P1)

A contributor working on the Atmos Pro API client can run pact consumer tests to verify that
the client code correctly describes the contract it expects from the Atmos Pro API — without
requiring network access to the live service. All 5 Atmos Pro API interactions are covered:
UploadAffectedStacks, LockStack, UnlockStack, OIDC token retrieval, and OIDC token exchange.

**Why this priority**: The API client is critical integration code. Contract tests allow
contributors to verify client behavior in total isolation, reducing the risk of silent
breakage when the Atmos Pro service changes its API shape.

**Independent Test**: Can be fully tested by running pact consumer tests (via a dedicated
build tag) and observing that all 5 endpoint interactions pass against the pact mock server,
with pact contract files generated as output.

**Acceptance Scenarios**:

1. **Given** the Atmos codebase is checked out and pact-go libraries are installed,
   **When** a contributor runs the pact consumer tests,
   **Then** tests for all 5 endpoint interactions complete successfully and generate
   JSON pact files in the designated `pacts/` output directory.

2. **Given** an Atmos Pro API client method sends a request with an unexpected field name
   or missing required header,
   **When** the pact consumer tests run,
   **Then** the test fails with a clear message identifying which interaction and field
   caused the mismatch.

---

### User Story 2 - Developer Detects Breaking Client Changes Locally (Priority: P2)

When a developer modifies the Atmos Pro API client code (request DTOs, headers, HTTP
methods, response parsing), running the pact consumer tests locally reveals whether the
client's expected contract has changed. This prevents shipping code that silently alters
the API surface expected by the Atmos Pro backend.

**Why this priority**: Contract changes that go undetected can cause production failures
when the Atmos Pro service receives unexpected request formats or the client fails to
parse a valid response. Local pact tests catch this before code is reviewed or merged.

**Independent Test**: Can be verified by intentionally modifying a request DTO field name
(e.g., renaming `head_sha` to `headSha`) and confirming the pact consumer test fails with
a descriptive mismatch error before any code reaches the live API.

**Acceptance Scenarios**:

1. **Given** a developer modifies the shape of any request or response DTO in `pkg/pro/`,
   **When** they run the pact consumer tests locally,
   **Then** any deviation from the defined contract is surfaced as a test failure with
   a message identifying which interaction and field caused the mismatch.

2. **Given** a developer runs pact consumer tests on an unmodified codebase,
   **When** no client changes are present,
   **Then** all 8 tests pass and the generated pact files are consistent with previously
   checked-in versions.

---

### User Story 3 - README Guides Contributors Through the Pact Workflow (Priority: P3)

A contributor new to the project reads the README and can understand how to install the
pact toolchain, run existing consumer tests, and generate updated pact contract files —
without needing to search external documentation.

**Why this priority**: Contract testing infrastructure is only valuable if contributors
know it exists and how to maintain it. Clear documentation prevents the tests from being
ignored, skipped, or accidentally broken.

**Independent Test**: Can be verified by following only the README instructions on a fresh
clone and confirming all pact consumer tests run and produce output files without additional
guidance.

**Acceptance Scenarios**:

1. **Given** a contributor follows the README pact section step-by-step,
   **When** they run the documented commands,
   **Then** they can install the required native libraries, run consumer tests,
   and locate the generated pact JSON files.

2. **Given** a pact consumer test fails due to a client change,
   **When** the contributor reads the README,
   **Then** they understand whether they should update the contract (intentional change)
   or fix the client code (accidental regression).

---

### Edge Cases

- What happens when the GitHub Actions OIDC endpoint is unavailable? The consumer test
  uses a pact mock server — the real GitHub OIDC endpoint is never contacted.
- How are chunked payloads (UploadAffectedStacks, UploadInstances) handled? The contract
  covers the shape of a single request/response cycle; chunking logic is tested separately
  by existing unit tests.
- What if pact-go updates introduce breaking API changes? The version is pinned in `go.mod`;
  contributors must review the pact-go changelog when upgrading.
- What if the pact tests are accidentally run without the native pact libraries installed?
  pact-go returns a clear error message; the README documents the required `pact-go install`
  step to prevent this.

## Requirements

### Functional Requirements

- **FR-001**: The repository MUST contain consumer pact tests for all 8 Atmos Pro API
  interactions: `UploadAffectedStacks` (POST `/affected-stacks`), `LockStack`
  (POST `/locks`), `UnlockStack` (DELETE `/locks`), GitHub OIDC token retrieval
  (GET to external OIDC endpoint), OIDC token exchange (POST `/auth/github-oidc`),
  `UploadInstances` (POST `/instances`), `UploadInstanceStatus`
  (PATCH `/repos/{owner}/{repo}/instances`), and `CreateCommit` (POST `/git/commit`).

- **FR-002**: Each consumer pact test MUST use a pact mock HTTP server rather than
  calling the live Atmos Pro API or GitHub OIDC endpoint.

- **FR-003**: Running the pact consumer tests MUST generate pact contract files (JSON)
  that fully describe each interaction: HTTP method, path, required headers
  (`Authorization`, `Content-Type`), request body shape, expected response status code,
  and response body shape. Generated pact files MUST be stored as local repository or
  CI build artifacts only — no publishing to an external Pact Broker is required.

- **FR-004**: Pact consumer tests MUST be isolated from the standard test suite via a
  build tag so that `go test ./...` does not run them by default, preventing test suite
  slowdowns for contributors who have not installed the pact native libraries.

- **FR-005**: The README MUST document: how to install the pact-go toolchain and native
  libraries, how to run the consumer tests, and where the generated pact files are stored.

- **FR-006**: All 8 consumer pact tests MUST be runnable locally by any contributor who
  has installed the pact native libraries, without requiring CI or cloud access.

### Key Entities

- **Pact Interaction**: A named description of a single request/response pair between
  the Atmos client (consumer) and the Atmos Pro API (provider). Attributes: consumer
  name, provider name, state (preconditions), request (method, path, headers, body),
  response (status, headers, body).

- **Pact Contract File**: A JSON file generated per consumer–provider pair containing all
  defined interactions. Stored in `pacts/` at the repository root. Used for provider
  verification in a future effort.

- **Consumer** (`atmos`): The Atmos CLI and its `pkg/pro` API client, which makes HTTP
  requests to the Atmos Pro API.

- **Provider** (`AtmosPro`): The Atmos Pro backend API service that fulfills HTTP requests
  from the consumer.

## Success Criteria

### Measurable Outcomes

- **SC-001**: All 8 Atmos Pro API endpoint interactions have corresponding consumer pact
  tests that pass locally without any network access to the live Atmos Pro API.

- **SC-002**: Running the pact consumer tests produces at least 1 pact contract JSON file
  containing definitions for all 8 interactions.

- **SC-003**: A new contributor can install the pact toolchain and run all consumer tests
  successfully by following only the README instructions, with the total setup time
  under 10 minutes on a standard development machine.

- **SC-004**: Introducing a deliberate mismatch in any of the 5 request or response shapes
  (field name, HTTP method, status code) causes the corresponding pact consumer test to
  fail with a descriptive error message identifying the mismatched field.

## Assumptions

- The Atmos CLI is the **consumer** and the Atmos Pro API is the **provider** in pact
  terminology. Only consumer contract generation is in scope for this feature; provider
  verification against a live or staged Atmos Pro API is a separate future effort.
- All 8 endpoints in `pkg/pro/` are in scope: `UploadAffectedStacks`, `LockStack`,
  `UnlockStack`, GitHub OIDC token retrieval, OIDC token exchange, `UploadInstances`,
  `UploadInstanceStatus`, and `CreateCommit`.
- Pact contract files will be stored in a `pacts/` directory at the repository root or
  within `pkg/pro/pacts/`; the exact location is decided during implementation.
- Pact consumer tests will use a build tag (e.g., `//go:build pact`) to remain opt-in
  and separate from the default `go test ./...` run. They are never run automatically
  in CI; contributors run them locally after installing the pact native libraries.
- Authentication in pact tests uses a mock Bearer token (`test-token`); no real Atmos Pro
  credentials or GitHub Actions OIDC tokens are needed.
- The GitHub OIDC token retrieval interaction (`GET` to `ACTIONS_ID_TOKEN_REQUEST_URL`)
  defines a contract describing what the Atmos client expects from that endpoint, not a
  binding contract with GitHub's infrastructure.
- The `pact-go` v2 package will be added as a dev dependency; it will not be included in
  the production binary.
- Pact contract files are local/repo artifacts only. Publishing to a Pact Broker or
  PactFlow is out of scope; that is a future effort tied to provider verification setup.
