# Ambiguous component for testing ambiguous path detection
# This terraform folder is referenced by multiple Atmos components:
# - ambiguous-alpha (metadata.component: ambiguous-component)
# - ambiguous-beta (metadata.component: ambiguous-component)
# When using path resolution (e.g., "atmos terraform plan ."), Atmos should
# detect the ambiguity and either show a selector (interactive) or error (non-interactive)

variable "environment" {
  type        = string
  description = "Environment name"
}

variable "variant" {
  type        = string
  description = "Component variant (alpha or beta)"
}

output "component_info" {
  value = {
    component   = "ambiguous-component"
    environment = var.environment
    variant     = var.variant
  }
}
