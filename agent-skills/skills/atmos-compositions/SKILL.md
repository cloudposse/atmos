---
name: atmos-compositions
description: "Atmos compositions: named service groupings, compositions.<name>.services, components.container[*].composition, composition validate, and service contract checks"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Compositions

Use this skill for compositions: named groupings of component instances that form a system.
Compositions define the service contract; components fulfill services by referencing the
composition.

## Related Skills

| Need | Load |
|---|---|
| Container components that fulfill services | [atmos-container](../atmos-container/SKILL.md) |
| Stack manifest structure | [atmos-stacks](../atmos-stacks/SKILL.md) |
| Workflow orchestration across services | [atmos-workflows](../atmos-workflows/SKILL.md) |

## Configuration

Declare composition service contracts at stack scope:

```yaml
compositions:
  app:
    description: Application runtime
    services:
      - api
      - worker
      - db

components:
  container:
    api:
      composition: app
    worker:
      composition: app
```

The composition describes expected services. Components declare which composition they fulfill.

## Validation

Use `atmos composition validate` to inspect whether a stack provides all services for a composition.

```bash
atmos composition validate app -s plat-ue2-dev
```

Validation reports fulfilled and not-provided services. Use this before running workflows that
assume a complete multi-service system.

## Guidance

- Use compositions for systems made of multiple stack-scoped services.
- Keep service names stable and human-readable.
- Do not use composition names as a replacement for Atmos profiles or `ATMOS_PROFILE`.
- Put shared composition definitions in stack mixins/defaults when multiple stacks use the same
  service contract.
- Prefer composition validation over hand-written checks in shell steps.
