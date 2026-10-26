# Mock component for merge-type-override test fixture.
variable "allowed_accounts" {
  type    = list(any)
  default = []
}

variable "rbac_roles" {
  type    = list(any)
  default = []
}

variable "tags" {
  type    = map(string)
  default = {}
}

variable "node_groups" {
  type    = any
  default = {}
}

output "mock" {
  value = "eks-cluster"
}
