# YAML Functions YQ Expression Issues

**Date:** 2026-01-26

This document tracks issues and inconsistencies in Atmos YAML functions documentation and implementation.

## Issue 1: Incorrect `.outputs.` Prefix in `!terraform.state` Documentation

**File:** `website/docs/functions/yaml/terraform.state.mdx`

**Problem:** Lines 136, 143, 146, 149, 152 incorrectly use `.outputs.` prefix in YQ expressions for bracket notation
examples.

**Evidence from Test Fixtures:**
The test fixture `tests/fixtures/scenarios/atmos-terraform-state-yaml-function/stacks/deploy/nonprod.yaml` shows the
correct patterns:

```yaml
# Correct - no .outputs. prefix
bar: !terraform.state component-2 .bar
test_val: !terraform.state component-2 ".foo | ""jdbc:postgresql://"" + . + "":5432/events"""
```

**Current (Incorrect):**

```yaml
# Line 136
access_key_id: !terraform.state security '.outputs.users["github-dependabot"].access_key_id'

# Line 143
api_key: !terraform.state config '.outputs.api_keys["service-account-1"]'

# Line 146
endpoint: !terraform.state services '.outputs.endpoints["my-service"]["production"]'

# Line 149
token: !terraform.state identity {{ .stack }} '.outputs.tokens["github-actions"]'

# Line 152
app_name: !terraform.state config '.outputs.apps["app''s-name"].display_name'
```

**Should Be (Correct):**

```yaml
# Line 136
access_key_id: !terraform.state security '.users["github-dependabot"].access_key_id'

# Line 143
api_key: !terraform.state config '.api_keys["service-account-1"]'

# Line 146
endpoint: !terraform.state services '.endpoints["my-service"]["production"]'

# Line 149
token: !terraform.state identity {{ .stack }} '.tokens["github-actions"]'

# Line 152
app_name: !terraform.state config '.apps["app''s-name"].display_name'
```

**Why This Is Wrong:**

- `!terraform.state` and `!terraform.output` abstract away the internal Terraform state structure
- Users access outputs directly by name, not through `.outputs.` path
- Earlier examples in the same file (lines 114, 120) correctly omit `.outputs.`:
  ```yaml
  subnet_id1: !terraform.state vpc .private_subnet_ids[0]
  username: !terraform.state config .config_map.username
  ```

---

## Issue 2: Confusing Component/Output Name Collision in `!terraform.output` Documentation

**File:** `website/docs/functions/yaml/terraform.output.mdx`

**Problem:** Line 141 uses `.security.` prefix where the component is also named `security`, creating confusion.

**Current (Confusing):**

```yaml
# Line 141 - component name is "security", YQ starts with ".security."
access_key_id: !terraform.output security '.security.users["github-dependabot"].access.key.id'
```

**Issue:** This implies accessing an output named `security` from a component also named `security`. While technically
possible, it's confusing for documentation examples.

**Suggested Fix:**
Either use a different output name to avoid confusion:

```yaml
# Option A: Different output name (clearer)
access_key_id: !terraform.output security '.users["github-dependabot"].access.key.id'
```

Or use different component/output names in the example:

```yaml
# Option B: Different names (clearest)
access_key_id: !terraform.output iam-config '.users["github-dependabot"].access.key.id'
```

---

## Issue 3: Inconsistent YQ Expression Patterns Across Documentation

**Files Affected:**

- `terraform.state.mdx`
- `terraform.output.mdx`
- `include.mdx`

**Problem:** Documentation shows inconsistent patterns for accessing data:

| File                     | Example                  | Pattern Used             |
|--------------------------|--------------------------|--------------------------|
| terraform.state.mdx:114  | `.private_subnet_ids[0]` | Direct output access     |
| terraform.state.mdx:136  | `.outputs.users[...]`    | Wrong `.outputs.` prefix |
| terraform.output.mdx:119 | `.private_subnet_ids[0]` | Direct output access     |
| terraform.output.mdx:141 | `.security.users[...]`   | Nested output access     |

**Recommended Fix:**
Standardize all examples to use direct output access pattern matching the test fixtures.

---

## Issue 4: Missing Bracket Notation Examples Without Prefix

**Files:** `terraform.state.mdx`, `terraform.output.mdx`

**Problem:** The bracket notation section only shows examples with problematic prefixes. Should include simple bracket
notation examples.

**Missing Examples:**

```yaml
# Access map key with hyphen - simple case
user_key: !terraform.state iam '.users["my-user"].key'

# Access nested map with special characters
config_value: !terraform.output config '.settings["app-config"]["prod-env"]'
```

---

## User-Reported Issues

*(Add user-reported issues below as they come in)*

### Issue 5: [Title]

**Reported by:** [User/Issue#]

**Problem:** [Description]

**Expected Behavior:** [What should happen]

**Actual Behavior:** [What happens]

---

## Files to Update

When fixing these issues, the following files need updates:

| File                                               | Lines                   | Issue                           |
|----------------------------------------------------|-------------------------|---------------------------------|
| `website/docs/functions/yaml/terraform.state.mdx`  | 136, 143, 146, 149, 152 | Remove `.outputs.` prefix       |
| `website/docs/functions/yaml/terraform.output.mdx` | 141, 148, 151, 154, 157 | Clarify component/output naming |

---

## Verification

After fixes, verify against test fixtures:

- `tests/fixtures/scenarios/atmos-terraform-state-yaml-function/`
- `tests/fixtures/scenarios/atmos-terraform-output-yaml-function/`
- `tests/fixtures/scenarios/atmos-template-yaml-function/`

Run tests:

```bash
go test ./tests -run "TestYAML" -v
```

---

## Related Documentation

- [YQ Guide](https://mikefarah.gitbook.io/yq)
- [YQ Recipes](https://mikefarah.gitbook.io/yq/recipes)
- [YAML Functions Index](/functions/yaml/index)
