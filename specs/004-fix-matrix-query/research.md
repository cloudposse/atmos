# Research: `--filter` for `describe affected`

All Technical Context unknowns are resolved. Each decision below includes the evidence in the repository.

## R1. Root cause of the reported bug

- **Decision**: Treat it as a design gap, not a one-line bug. `view()` returns from the matrix branch before `a.Query` is read (`internal/exec/describe_affected.go:761-771`). Rather than making a query reshape matrix entries, reject the combination and provide a filter.
- **Rationale**: A query can return anything (scalars, reshaped maps), so a matrix cannot be built from it reliably. A predicate filter cannot reshape, so its result always maps to matrix entries.
- **Alternatives considered**: apply `--query` before the matrix conversion and validate the shape (rejected: it needs a shape-validation error path and keeps a footgun); silently keep ignoring it (rejected: the original bug).

## R2. Filter semantics

- **Decision**: Reuse `pkg/list/filter.YQPredicateFilter` semantics. The expression is evaluated per item, and the item is kept when the result is truthy (`false`, `nil` and empty values are falsy). Add an exported `Matches(item map[string]any) (bool, error)` so the same truthiness rule can be applied to a single item. `Apply` calls it, so both paths stay identical.
- **Rationale**: `list instances --filter` documents exactly this behavior (`list-instances.mdx:64`). One syntax, one truthiness rule, no duplication.
- **Alternatives considered**: a new predicate type in `internal/exec` (rejected: duplicates the truthiness rules); reuse `--query` with `select()` (rejected: the whole motivation).

## R3. Where to apply the filter

- **Decision**: Apply it in the shared dependents-finalizing path, after dependents are resolved and before `--flatten` lifts them (`finalizeAffectedDependents`, `describe_affected.go:~750`). When `--include-dependents` is not set, apply it directly to the affected list. The call is a no-op when no predicate is set.
- **Rationale**: Spec FR-009a requires pruning before flattening so a dependent under a filtered-out ancestor never appears. `list affected` shares this path, so it keeps working untouched.
- **Alternatives considered**: filter at the end in `view()` (rejected: flattening has already lifted dependents out of their subtree, losing the ancestry needed for pruning).

## R4. Pruning rule differs from `--tags`/`--labels`

- **Decision**: A failing item is dropped together with its entire dependents subtree. Dependents that would pass are not promoted. This is the spec's clarified behavior.
- **Rationale**: User decision. The existing selectors do the opposite: `applySelectorsToAffectedForest` promotes a dropped item's matching dependents (`describe_affected_utils_2.go:~911`). The difference is intentional and must be called out in the docs so users are not surprised.
- **Alternatives considered**: reuse the promote logic (rejected by the user).

## R5. What the filter sees

- **Decision**: Convert each `schema.Affected` and `schema.Dependent` to a map with a JSON round trip (field names follow the existing json tags: `stack`, `component`, `settings`, ...) and evaluate against it.
- **Rationale**: `--query` already evaluates against the JSON-shaped data, so users write the same paths. `settings` is only populated with `--include-settings`, so a filter on `.settings...` needs that flag, exactly as `--query` does today.
- **Open check for implementation**: confirm that dependents carry `settings` when `--include-settings` is set (the field exists on `schema.Dependent`). If not, the docs must say settings-based filters apply to top-level items only, or settings must be populated for dependents.
- **Alternatives considered**: evaluate against raw component sections (rejected: a different shape than the output users see).

## R6. Flag and env var

- **Decision**: Register `filter` on the existing `newDescribeAffectedSelectorFlagsParser` with env var `ATMOS_DESCRIBE_AFFECTED_FILTER` under the existing `describe.affected` Viper prefix. Record whether the value came from the environment using the existing `flags.MarkFlagValueFromEnv`/`FlagValueFromEnv` mechanism, exactly like `--tags`.
- **Rationale**: This follows the established pattern and avoids a collision with the `ATMOS_LIST_FILTER` binding (the bare `filter` Viper key is used by the list family). No direct `viper.Bind*` calls are needed.
- **Alternatives considered**: `ATMOS_LIST_FILTER` (rejected: wrong command family); a bare `filter` Viper key (rejected: collision risk).

## R7. Upload and query guards

- **Decision**:
  - `--filter` flag + `--upload` → error (`ErrInvalidFlag` plus hints), mirroring `resolveAffectedSelectors`.
  - Filter only from the environment + `--upload` → `ui.Warningf` and the filter is dropped.
  - `--query` + `--format=matrix` → error (`ErrInvalidFlag`) with a hint to use `--filter`, raised during argument validation (`validateDescribeAffectedBasics`), before any affected computation or output.
- **Rationale**: Spec FR-006 and FR-009. Early validation also means no matrix is ever written.
- **Alternatives considered**: warn and ignore `--query` (rejected: still silent in CI logs).

## R8. Other commands with matrix output

- **Decision**: `list affected` exposes `--format` but has no `--query`/`--filter` (checked `cmd/list/affected.go`), so it has no bypass. `list instances` already rejects `--filter` and `--query` with `--format=matrix` (`list-instances.mdx`). No change is needed. Record this in the PR notes.
- **Rationale**: Spec FR-008 asks for a review. This is the result.
- **Alternatives considered**: add `--filter` to `list affected` (out of scope).

## R9. Release and documentation obligations

- **Decision**: Release label `minor`. Required: blog post, roadmap entry, updated `describe-affected.mdx`, regenerated help screengrab, fix record under `docs/fixes/`, and a website build. No JSON schema change, because this is a CLI flag and not an `atmos.yaml` option.
- **Rationale**: CLAUDE.md PR rules and the constitution.
