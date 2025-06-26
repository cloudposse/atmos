variable "stage" {
  type    = string
  default = "nonprod"
}

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

variable "tags" {
  type    = map(string)
  default = {}
}

output "stage" {
  value = var.stage
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

output "tags" {
  value = var.tags
}
