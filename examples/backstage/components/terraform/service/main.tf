# No providers or credentials are needed. Backstage describes this component;
# it does not run Terraform or provision resources.
variable "stage" {
  type = string
}
variable "service_name" {
  type = string
}
variable "replicas" {
  type = number
}
variable "owner" {
  type = string
}

output "service" {
  value = {
    name     = var.service_name
    stage    = var.stage
    replicas = var.replicas
    owner    = var.owner
  }
}
