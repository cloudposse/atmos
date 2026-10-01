variable "foo" {
  type    = string
  default = "foo"
}

variable "bar" {
  type    = string
  default = "bar"
}

variable "baz" {
  type    = string
  default = "baz"
}

# Declared-type field-test vars: used to prove `stack set --type=auto` can infer
# a value's type from these declarations (via terraform-config-inspect) instead
# of only from an existing/merged value or the value's own shape.
variable "quota" {
  type    = number
  default = 1
}

variable "instance_count" {
  type    = number
  default = 1
}

variable "feature_flag" {
  type    = bool
  default = false
}

variable "allowed_cidrs" {
  type    = list(string)
  default = []
}

output "foo" {
  value = var.foo
}

output "bar" {
  value = var.bar
}

output "baz" {
  value = var.baz
}
