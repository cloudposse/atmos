## 0.8.0

- Convert infrastructure YAML stack configs into Spacelift stack configs
- Added `utils_spacelift_stack_config` data source
- Added `examples/data-sources/utils_spacelift_stack_config` example
- Use `goroutines` and `WaitGroup` to process a list of infrastructure YAML stack configs concurrently to speed up processing

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.7.0

- Calculate component dependencies from stack imports
- Added `deps` output for each component
- Reduced unnecessary stack runs in CI/CD systems like Spacelift

- The provider already calculates these parameters:
    - `imports` - a list of all imports (all levels) for the stack
    - `stacks` - a list of all stacks in the infrastructure where the component is defined

- Both `imports` and `stacks` are not 100% suitable to correctly determine the YAML config files that a component depends on:
    - `imports` is too broad. The provider returns all direct and indirect imports for the stack, even those that don't define any variables for the
      component. This will trigger the component's stack in Spacelift even if the unrelated imports are modified, resulting in unnecessary stack runs
      in Spacelift
    - `stacks` is too broad and too narrow at the same time. On the one hand, it detects all stacks in the infrastructure where the component is
      defined, but we don't need to trigger a particular stack in Spacelift if some other top-level YAML stack configs are modified. On the other
      hand, it misses the cases where a YAML stack config file specifies global variables, which are applied to the component as well

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.6.0

- Updates GitHub release action to use `go` version 1.16

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.4.2

- Added `process_stack_deps` input var to the stack_config data source (configurable, `false` by default). Not all provider invocations need to
  process all stack dependencies for the components (e.g. Spacelift module needs it, remote-state does not). Makes invocations without processing
  stack dependencies 2-3 times faster.

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.4.1

- Read and parse only YAML files in stack processor (non-YAML files in the stacks folder cause the YAML parser to panic)

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.4.0

- Added imports to stack output

- Added stacks to each component output

- `imports` attribute shows all imported stacks for the current stack - can be used in CI/CD pipelines to determine stack dependencies

- `stacks` attribute shows all the stacks the component (and its base component, if present) is declared in - can be used in CI/CD pipelines (e.g.
  Spacelift) to determine all stacks that the component depends on, and to provision triggers for all the dependencies (once any of the stack config
  files changes, the component's job will be triggered)

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.3.0

- Workaround for a deep-merge bug in `mergo.Merge()`. When deep-merging slice of maps in a `for` loop,
  `mergo` modifies the source of the previous loop iteration if it's a complex map and `mergo` gets a pointer to it, not only the destination of the
  current loop iteration.

- Added `settings` sections to `data_source_stack_config_yaml` data source to provide settings for Terraform and helmfile components

- Added `env` sections to `data_source_stack_config_yaml` data source to provide ENV vars for Terraform and helmfile components

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.2.0

- Added `data_source_stack_config_yaml` data source to process YAML stack configurations for Terraform and helmfile components

BACKWARDS INCOMPATIBILITIES / NOTES:

## 0.1.0

- Initial release
