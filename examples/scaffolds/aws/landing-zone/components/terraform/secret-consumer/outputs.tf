output "secret_lengths" {
  description = "Lengths of the resolved secrets (values themselves are never exposed)."
  sensitive   = true
  value = {
    ssm_instance_token    = length(var.ssm_instance_token)
    ssm_stack_token       = length(var.ssm_stack_token)
    asm_database_password = length(var.asm_database_password)
    global_shared_token   = length(var.global_shared_token)
  }
}
