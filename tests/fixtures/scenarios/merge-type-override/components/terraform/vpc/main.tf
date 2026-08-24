# Mock component for merge-type-override test fixture.
variable "cidr_blocks" {
  type    = list(string)
  default = []
}

output "mock" {
  value = "vpc"
}
