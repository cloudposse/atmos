# Conditional import selector field-test fixture

Copied from `examples/conditional-imports` and extended to exercise the boundary between early import decisions and final component metadata.

Run from this directory with a freshly built Atmos binary. Set `FIELD_TEST_PCI=true`, `FIELD_TEST_TAG=other` for the computed cases. Commands only list or describe configuration; no infrastructure is created.

```shell
atmos list instances --labels pci:true
atmos list instances --tags pci
atmos describe component app -s payments --query .vars
atmos describe component app -s tag-only --query .vars
```

The other stacks cover quoted booleans, component overrides, late metadata, computed metadata, mixed literal/computed tags, negative membership, literal exclamation-prefixed tags, and skipped missing imports. The field-test report records observed outcomes and limitations.
