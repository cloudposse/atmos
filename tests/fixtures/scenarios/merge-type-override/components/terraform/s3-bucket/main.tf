# Mock component for merge-type-override test fixture.
variable "lifecycle_rules" {
  type    = list(any)
  default = []
}

output "mock" {
  value = "s3-bucket"
}
