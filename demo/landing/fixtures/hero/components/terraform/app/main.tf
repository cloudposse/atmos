# A cloud-free component makes the configuration change visible in a real plan.
resource "terraform_data" "app" {
  input = var.replicas
}
