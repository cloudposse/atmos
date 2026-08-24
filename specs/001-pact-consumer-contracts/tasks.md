---

description: "Task list for Pact Consumer Contract Testing for Atmos Pro API"
---

# Tasks: Pact Consumer Contract Testing for Atmos Pro API

**Input**: Design documents from `specs/001-pact-consumer-contracts/`

**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/interactions.md ✅, quickstart.md ✅

**Tests**: Pact consumer tests are the feature itself — all [US1]/[US2] tasks produce test code.

**Organization**: Tasks are grouped by user story to enable independent implementation and verification.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no blocking dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Test files: `pkg/pro/` (same package as production code, `//go:build pact` tag)
- Pact output: `pacts/` at repo root
- Documentation: `README.md` at repo root

---

## Phase 1: Setup

**Purpose**: Add the pact-go dependency and create the pact output directory.

- [x] T001 Add `github.com/pact-foundation/pact-go/v2` as a dev dependency by running `go get github.com/pact-foundation/pact-go/v2` and committing the updated `go.mod` and `go.sum`
- [x] T002 [P] Create `pacts/.gitkeep` at repo root to track the pact output directory in git; add `pacts/*.json` to `.gitignore` only if JSON files should NOT be committed (per spec: they SHOULD be committed — do not gitignore them)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared mock provider helpers required by all 8 pact interactions.

**⚠️ CRITICAL**: No pact interaction tasks can be written until this phase is complete.

- [x] T003 Create `pkg/pro/pact_helpers_test.go` with `//go:build pact` tag and `package pro` — implement:
  - `newHTTPMockProvider(t *testing.T) *consumer.V2HTTPMockProvider` using `consumer.NewV2Pact(consumer.MockHTTPProviderConfig{Consumer: "atmos", Provider: "AtmosPro", PactDir: pactDir()})` where `pactDir()` resolves to `<repo_root>/pacts`
  - `newTLSMockProvider(t *testing.T) *consumer.V2HTTPMockProvider` using `consumer.NewV2Pact` with TLS enabled (for the GitHub OIDC GET interaction)
  - `pactDir() string` helper that returns the absolute path to `pacts/` at the repository root (use `runtime.Caller(0)` or `filepath.Join` relative to the test file location)

**Checkpoint**: Foundation ready — all pact interaction tasks can now be implemented.

---

## Phase 3: User Story 1 — Consumer Pact Interactions (Priority: P1) 🎯 MVP

**Goal**: All 8 Atmos Pro API endpoint interactions are defined as pact consumer contracts. Running `go test -tags pact ./pkg/pro/... -v` generates `pacts/atmos-AtmosPro.json` with all 8 interactions.

**Independent Test**: Run `go test -tags pact ./pkg/pro/... -v -run TestPact` — all 8 tests must pass and the JSON pact file must exist with 8 interactions.

### Implementation for User Story 1

- [x] T004 [US1] Create `pkg/pro/consumer_pact_test.go` with `//go:build pact` build tag, `package pro` declaration, and required imports (`consumer`, `matchers` from `github.com/pact-foundation/pact-go/v2`, plus existing `pkg/pro` deps)

- [x] T005 [US1] Add `TestPact_UploadAffectedStacks(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - Use `newHTTPMockProvider(t)`
  - State: `"workspace exists and accepts affected stacks"`
  - Request: POST `/api/v1/affected-stacks`, headers `Authorization: Like("Bearer test-token")` + `Content-Type: application/json`, body with `Like()` matchers for `head_sha`, `base_sha`, `repo_url`, `repo_name`, `repo_owner`, `repo_host`, and `EachLike` for `stacks` array
  - Response: 200, body `{"success": true}`
  - Test body: create `AtmosProAPIClient` pointing to mock server URL, call `UploadAffectedStacks` with a minimal `dtos.UploadAffectedStacksRequest`, assert no error

- [x] T006 [US1] Add `TestPact_LockStack(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - State: `"workspace exists and stack is unlocked"`
  - Request: POST `/api/v1/locks`, `Like()` matchers for `key` (string) and `ttl` (int32)
  - Response: 200, body `success: true`, `data.id: Like("lock-id")`, `data.key: Like("key")`, `data.expiresAt: Term(regex, generate)`
  - Test body: call `LockStack` with `dtos.LockStackRequest{Key: "test/key", TTL: 3600}`, assert no error and response `Data.ID` is non-empty

- [x] T007 [US1] Add `TestPact_UnlockStack(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - State: `"workspace exists and stack is locked"`
  - Request: DELETE `/api/v1/locks`, body `{"key": Like("test/key")}`
  - Response: 200, body `{"success": true, "data": {}}`
  - Test body: call `UnlockStack` with `dtos.UnlockStackRequest{Key: "test/key"}`, assert no error

- [x] T008 [US1] Add `TestPact_ExchangeOIDCToken(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - Use `newHTTPMockProvider(t)` (standard HTTP, not TLS — `exchangeOIDCTokenForAtmosToken` does not enforce https)
  - State: `"OIDC token is valid and workspace exists"`
  - Request: POST `/api/v1/auth/github-oidc`, `Content-Type: application/json`, `User-Agent: Term(regex "^atmos/", generate "atmos/1.0.0 (linux; amd64)")`, body `{"token": Like("oidc-token"), "workspaceId": Like("workspace-uuid")}`
  - Response: 200, body `{"success": true, "data": {"token": Like("jwt-token")}}`
  - Test body: call `exchangeOIDCTokenForAtmosToken(mockURL, "api/v1", "oidc-token", "workspace-uuid")` (package-private function, accessible within `package pro`), assert no error

- [x] T009 [US1] Add `TestPact_UploadInstances(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - State: `"workspace exists and accepts drift detection instances"`
  - Request: POST `/api/v1/instances`, body with `Like()` for `repo_url/name/owner/host`, `EachLike` for `instances` array with `{component, stack, component_type}`
  - Response: 200, body `{"success": true}`
  - Test body: call `UploadInstances` with a single-instance `dtos.InstancesUploadRequest`, assert no error

- [x] T010 [US1] Add `TestPact_UploadInstanceStatus(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - State: `"workspace exists and instance exists for owner/repo"`
  - Request: PATCH `/api/v1/repos/org/repo/instances` with query `stack=dev-us-east-1&component=vpc`, body `{"command": Like("terraform plan"), "exit_code": Like(0)}`
  - Response: 200, body `{"success": true}`
  - Test body: create `AtmosProAPIClient` pointing at mock server; call `UploadInstanceStatus` with `dtos.InstanceStatusUploadRequest{RepoOwner:"org", RepoName:"repo", Stack:"dev-us-east-1", Component:"vpc", Command:"terraform plan", ExitCode:0}`; assert no error
  - Note: pact V2 path matching is exact-string; use literal `"/api/v1/repos/org/repo/instances"` as the path and set `r.Query` for `stack` and `component` separately

- [x] T011 [US1] Add `TestPact_CreateCommit(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - State: `"workspace exists and GitHub App is authorized"`
  - Request: POST `/api/v1/git/commit`, body with `Like()` for `branch` (string), `commitMessage` (string), `changes.additions` (`EachLike({path: Like("file.txt"), contents: Like("base64...")})`), `changes.deletions` (`EachLike({path: Like("old.txt")})`)
  - Response: 200, body `{"success": true, "data": {"sha": Like("abc123")}}`
  - Test body: call `CreateCommit` with a minimal `dtos.CommitRequest{Branch:"main", CommitMessage:"test", Changes: dtos.CommitChanges{Additions: []dtos.CommitFileAddition{{Path:"f.txt", Contents:"dGVzdA=="}}}}`, assert no error and response `Data.SHA` non-empty

- [x] T012 [US1] Add `TestPact_GetGitHubOIDCToken(t *testing.T)` to `pkg/pro/consumer_pact_test.go`:
  - Use `newTLSMockProvider(t)` (TLS required — `buildOIDCRequestURL` enforces https scheme)
  - State: `"GitHub Actions OIDC endpoint is available"`
  - Request: GET `/token?audience=atmos-pro.com`, header `Authorization: Like("Bearer test-request-token")`
  - Response: 200, body `{"value": Like("oidc-token-string")}`
  - Test body:
    1. Set `oidcHTTPClientOverride` to the pact TLS mock's HTTP client (restore to nil via `t.Cleanup`)
    2. Construct `schema.GithubOIDCSettings{RequestURL: fmt.Sprintf("https://%s/token", mockServerHost), RequestToken: "test-request-token"}`
    3. Call `getGitHubOIDCToken(settings)` (package-private, accessible within `package pro`)
    4. Assert returned token equals mock response value

**Checkpoint**: At this point all 8 pact interactions are implemented. Running `go test -tags pact ./pkg/pro/... -v` must generate `pacts/atmos-AtmosPro.json`.

---

## Phase 4: User Story 2 — Contract Regression Validation (Priority: P2)

**Goal**: Confirm the pact tests catch intentional mismatches, proving they guard against accidental client regressions.

**Independent Test**: Temporarily rename a field in `dtos.UploadAffectedStacksRequest` (e.g., `head_sha` → `headSha`), run pact tests, confirm `TestPact_UploadAffectedStacks` fails, then revert.

- [x] T013 [US2] Run `go test -tags pact ./pkg/pro/... -v` from the repo root and verify:
  - All 8 `TestPact_*` functions pass
  - File `pacts/atmos-AtmosPro.json` exists and contains exactly 8 interactions
  - Consumer is `"atmos"` and provider is `"AtmosPro"` in the generated file
  - Commit `pacts/atmos-AtmosPro.json` to the repository

---

## Phase 5: User Story 3 — README Documentation (Priority: P3)

**Goal**: A contributor can install the pact toolchain and run consumer tests in under 10 minutes by following only the README.

**Independent Test**: Follow the README instructions on a fresh checkout; confirm all pact tests run and produce output.

- [x] T014 [US3] Add "Pact Contract Testing" section to `README.md` using the content from `specs/001-pact-consumer-contracts/quickstart.md`. Place it in the Contributing / Development section (or just before the Testing section if one exists). Preserve existing README formatting and do not reorder other sections.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verify no regressions, no lint violations, no binary size impact.

- [x] T015 [P] Run `go build .` and confirm the binary compiles cleanly — pact-go must be test-only and must not appear in the production binary
- [x] T016 [P] Run `go test ./pkg/pro/...` (without `-tags pact`) and confirm all existing unit tests still pass — pact build tag must fully isolate the new test files
- [x] T017 [P] Run `make lint` and fix any `golangci-lint` violations in `pkg/pro/pact_helpers_test.go` and `pkg/pro/consumer_pact_test.go` — pay attention to `godot` (comment periods), import grouping, and `funlen`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on T001 (pact-go in go.mod)
- **US1 (Phase 3)**: All tasks depend on T003 (helpers file). T005–T012 each depend on T004 (file stub). T005–T011 are independent of each other; T012 depends on the TLS helper in T003.
- **US2 (Phase 4)**: Depends on all of Phase 3 being complete
- **US3 (Phase 5)**: Independent of Phases 3–4 — can be done any time after Phase 1
- **Polish (Phase 6)**: Depends on Phases 3–5 being complete

### User Story Dependencies

- **US1 (P1)**: Requires Phase 1 + Phase 2. No dependency on US2 or US3.
- **US2 (P2)**: Requires US1 complete (needs the 8 interactions to validate).
- **US3 (P3)**: Requires only Phase 1. Can be developed in parallel with US1/US2.

### Within Phase 3

- T004 → T005, T006, T007, T008, T009, T010, T011, T012 (file must exist before adding functions)
- T005–T011 are independent of each other (separate test functions in the same file)
- T012 requires T003 TLS helper (same dependency as others, but must also use the TLS mock)

### Parallel Opportunities

- T001 and T002 can run in parallel (different files)
- T005 through T011 can be written in parallel by different contributors (separate functions, append-only to the same file — coordinate via separate branches and merge)
- T015, T016, T017 can run in parallel (different commands, no file changes)
- T014 (README) is fully independent of T013 (pact validation)

---

## Implementation Strategy

### MVP (User Story 1 Only)

1. Complete Phase 1: Add dependency + pacts dir
2. Complete Phase 2: Create pact helpers
3. Complete Phase 3: Implement all 8 interactions
4. **STOP and VALIDATE**: Run `go test -tags pact ./pkg/pro/... -v`, confirm `pacts/atmos-AtmosPro.json` generated
5. Commit pact file

### Incremental Delivery

1. Setup + Foundational → dependency ready
2. T004–T007 (4 interactions) → partial pact file, early feedback on mock setup
3. T008–T012 (remaining 4) → full pact file with all 8 interactions
4. T013 → validate regression detection
5. T014 → README updated
6. T015–T017 → no regressions, lint clean

---

## Notes

- `[P]` = different files or independent functions, no blocking dependency
- `[Story]` label maps each task to a user story for traceability
- All pact test files MUST have `//go:build pact` as the FIRST line (before `package`)
- Never run `go test` with `--no-verify` or skip the pre-commit hooks
- `pacts/atmos-AtmosPro.json` MUST be committed to git — it is the consumer contract artifact
- pact-go native libraries must be installed via `pact-go -l DEBUG install` before running tests
