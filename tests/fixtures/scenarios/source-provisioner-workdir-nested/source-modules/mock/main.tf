# Local, network-free "source" module. source-provisioner-workdir-nested's
# components vendor this directory via source.uri (a plain local relative
# path resolved against the process CWD -- see pkg/provisioner/source/vendor.go
# IsLocalPath/localDirectorySource, which copies directly without go-getter,
# so no network access is required to exercise the JIT source+workdir path).

variable "environment" {
  type        = string
  default     = "test"
  description = "Environment name"
}

output "component_type" {
  value       = "source-provisioner-workdir-nested-mock"
  description = "Marker output proving this state came from the vendored source module."
}

output "environment" {
  value       = var.environment
  description = "The configured environment"
}
