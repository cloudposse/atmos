---
sidebar_position: 3
---

# Component Validation

Validation is critical to maintaining hygenic configurations in distributed team environments. 

## JSON Schema

Atmos has native support for [JSON Schema](https://json-schema.org/), which can validate the schema of configurations. JSON Schema is an industry standard and provides a vocabulary to annotate and validate JSON documents for correctness.

This is powerful stuff: because you can define many schemas, it's possible to validate components differently for different environments or teams.

## Open Policy Agent (OPA)

The [Open Policy Agent](https://www.openpolicyagent.org/docs/latest/) (OPA, pronounced “oh-pa”) is another open-source industry standard that provides a general-purpose policy engine to unify policy enforcement across your stacks. The OPA language (rego) is a high-level declarative language for specifying policy as code. Atmos has native support for the OPA decision-making engine to enforce policies across all the components in your stacks (e.g. for microservice configurations).
