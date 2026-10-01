variable "stage" {
  type        = string
  description = "Stage where the service is deployed"
}

# The `{{ ... }}` sequences below are part of the GCP resource name format documented by the
# provider. They are Terraform source code, not Atmos stack manifests, so Atmos must never
# render them as `Go` templates. See https://github.com/cloudposse/atmos/issues/2145.
variable "service_name_format" {
  type        = string
  description = "Name format: projects/{{project}}/locations/{{location}}/services/{{name}}"
  default     = "projects/{{project}}/locations/{{location}}/services/{{name}}"
}

locals {
  service_id = "${var.stage}-${var.service_name_format}"
}
