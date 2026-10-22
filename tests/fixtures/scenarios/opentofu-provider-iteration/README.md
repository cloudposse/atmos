# OpenTofu provider iteration

Reproduces issue #3483. The component selects an indexed provider alias (`provider = aws.by_account[each.value]`), which OpenTofu 1.9+ supports but terraform-config-inspect reports as `Invalid provider reference`. `atmos describe component test-component -s test` must succeed and record `component_info.validation_skipped_provider_iteration`.
