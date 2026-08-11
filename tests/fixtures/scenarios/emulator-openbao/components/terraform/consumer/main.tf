terraform {
  required_version = ">= 1.3.0"
}

variable "name" {
  type        = string
  description = "Component name."
  default     = "consumer"
}

variable "stage" {
  type        = string
  description = "Stage name (injected by Atmos)."
  default     = ""
}

variable "app_secret" {
  type        = string
  description = "Application secret resolved from the OpenBao emulator via !secret."
  sensitive   = true
}

# The length is non-sensitive and lets the E2E assert that the exact value round-tripped
# through OpenBao (set -> KV v2 store -> resolved via !secret -> TF_VAR_app_secret) without
# ever printing the secret itself. No cloud provider or resources are needed: resolving
# the !secret happens before Terraform runs, so a missing or unreachable secret fails the
# apply.
# nonsensitive() unwraps the length so `terraform output` can print it: the count of
# characters leaks negligible information, and the test needs to read it to confirm the
# value round-tripped. The secret value itself is never emitted.
output "app_secret_len" {
  value       = nonsensitive(length(var.app_secret))
  description = "Character length of the resolved secret (asserted by the E2E test)."
}
