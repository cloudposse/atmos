# LOCAL (non-source) component. Unlike source-modules/mock/main.tf (which is
# vendored via source.uri -- see pkg/provisioner/source/vendor.go
# IsLocalPath/localDirectorySource), this directory IS the component itself:
# it is referenced directly via `metadata.component: mock` (e.g.
# "app/local-nested", consumer-flat-output/-state, consumer-nested-output/
# -state in stacks/deploy/dev.yaml) and is never vendored or copied.

variable "environment" {
  type        = string
  default     = "test"
  description = "Environment name"
}

output "component_type" {
  value       = "source-provisioner-workdir-nested-local"
  description = "Marker output proving this state came from the local (non-vendored) component, distinct from source-modules/mock's vendored marker."
}

output "environment" {
  value       = var.environment
  description = "The configured environment"
}
