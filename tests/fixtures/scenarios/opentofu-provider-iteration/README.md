# OpenTofu provider iteration

Reproduces issue #3483. The component selects an indexed provider alias (`provider = aws.by_account[each.value]`), which OpenTofu 1.9+ supports but terraform-config-inspect reports as `Invalid provider reference`. `atmos describe component test-component -s test` must succeed and record `component_info.validation_skipped_provider_iteration`.

`mixed-component` adds a "Variables not allowed" diagnostic on top of provider iteration, so Atmos also records `validation_skipped_module_source_interpolation` and leaves `terraform_config` nil. It must record `component_info.sensitive_variables` so the `sensitive = true` input stays out of the generated varfile.
