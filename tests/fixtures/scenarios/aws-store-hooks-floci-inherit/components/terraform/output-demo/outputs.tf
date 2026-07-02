output "demo_id" {
  value = "store-hooks-demo"
}

output "structured_config" {
  value = {
    endpoint = "https://store-hooks.example.com"
    ports    = [443, 8443]
    nested = {
      enabled = true
    }
  }
}

output "secret_like_value" {
  value     = "demo-sensitive-output"
  sensitive = true
}

output "received_values" {
  value = {
    same_stack_ssm        = var.same_stack_ssm
    same_stack_asm        = var.same_stack_asm
    ssm_query             = var.ssm_query
    asm_query             = var.asm_query
    ssm_raw_key           = var.ssm_raw_key
    asm_raw_key           = var.asm_raw_key
    template_read         = var.template_read
    cold_start_default    = var.cold_start_default
    cross_stack_ssm       = var.cross_stack_ssm
    cross_stack_asm       = var.cross_stack_asm
    cross_stack_ssm_query = var.cross_stack_ssm_query
    cross_stack_template  = var.cross_stack_template
  }
}
