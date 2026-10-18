# Cloud-free resources for the homepage UI demonstration. Each resource is
# actually created in local Terraform state. Short delays make dependency order
# and concurrent progress visible without external services or credentials.
resource "terraform_data" "cluster" {
  input = var.stage

  provisioner "local-exec" {
    command = "sleep 2"
  }
}

resource "terraform_data" "api" {
  input      = var.replicas
  depends_on = [terraform_data.cluster]

  provisioner "local-exec" {
    command = "sleep 3"
  }
}

resource "terraform_data" "worker" {
  input = terraform_data.cluster.output

  provisioner "local-exec" {
    command = "sleep 2"
  }
}

resource "terraform_data" "web" {
  input = terraform_data.api.output

  provisioner "local-exec" {
    command = "sleep 2"
  }
}
